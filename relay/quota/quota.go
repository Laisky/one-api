package quota

import (
	"math"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// ComputeInput describes all parameters required to calculate quota consumption
// for a particular usage snapshot.
type ComputeInput struct {
	Usage                  *relaymodel.Usage
	ModelName              string
	ModelRatio             float64
	ChannelModelRatio      map[string]float64
	GroupRatio             float64
	ChannelModelConfigs    map[string]modelcfg.ModelConfigLocal
	ChannelCompletionRatio map[string]float64
	PricingAdaptor         adaptor.Adaptor
}

// ComputeResult captures the outcome of a quota calculation, including
// normalized ratios used and cached token details.
type ComputeResult struct {
	TotalQuota          int64
	PromptTokens        int
	CompletionTokens    int
	CachedPromptTokens  int
	UsedModelRatio      float64
	UsedCompletionRatio float64
}

// Compute calculates the quota required for the provided usage snapshot.
func Compute(input ComputeInput) ComputeResult {
	usage := input.Usage
	if usage == nil {
		return ComputeResult{}
	}

	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	pricingAdaptor := input.PricingAdaptor
	resolvedModelCfg, hasResolvedModelCfg := pricing.ResolveModelConfigRatioOnly(input.ModelName, input.ChannelModelConfigs, pricingAdaptor)

	// Resolve the completion ratio using a priority-ordered check to avoid redundant lookups.
	var completionRatioResolved float64
	if override, ok := input.ChannelCompletionRatio[input.ModelName]; ok {
		completionRatioResolved = override
	} else if hasResolvedModelCfg && resolvedModelCfg.CompletionRatio != 0 {
		completionRatioResolved = resolvedModelCfg.CompletionRatio
	} else {
		completionRatioResolved = pricing.GetCompletionRatioWithThreeLayers(input.ModelName, input.ChannelCompletionRatio, pricingAdaptor)
	}

	hasChannelModelRatioOverride := hasOverrideForModel(input.ModelName, input.ChannelModelRatio)
	baseRatio := input.ModelRatio

	if hasResolvedModelCfg {
		// Preserve legacy fallback behavior: when channel config omits base ratio/completion
		// (keeps zero values), continue using the resolved three-layer ratios as base values.
		if resolvedModelCfg.Ratio == 0 {
			resolvedModelCfg.Ratio = baseRatio
		}
		if resolvedModelCfg.CompletionRatio == 0 {
			resolvedModelCfg.CompletionRatio = completionRatioResolved
		}
	} else {
		// Build a minimal config from resolved base ratios if no config was found.
		resolvedModelCfg = adaptor.ModelConfig{
			Ratio:           baseRatio,
			CompletionRatio: completionRatioResolved,
		}
	}

	eff := pricing.ResolveEffectivePricingFromConfig(promptTokens, resolvedModelCfg)

	usedModelRatio := baseRatio
	usedCompletionRatio := completionRatioResolved

	if hasResolvedModelCfg {
		if !hasChannelModelRatioOverride {
			usedModelRatio = eff.InputRatio
		}

		// Optimization: Deriving the tiered completion ratio from eff.OutputRatio / eff.InputRatio
		// avoids a redundant loop over tiers. Since eff.OutputRatio = eff.InputRatio * tierComp,
		// the division recovers the effective completion ratio for the current tier.
		if eff.InputRatio != 0 {
			usedCompletionRatio = eff.OutputRatio / eff.InputRatio
		} else {
			usedCompletionRatio = 1.0
		}
	} else if pricingAdaptor != nil {
		// Optimized check: only use effective pricing if the input model ratio matches the adaptor base.
		// This avoids extra GetDefaultModelPricing() map lookups when not needed.
		adaptorBase := pricingAdaptor.GetModelRatio(input.ModelName)
		if math.Abs(baseRatio-adaptorBase) < 1e-12 {
			usedModelRatio = eff.InputRatio
			if eff.InputRatio != 0 {
				usedCompletionRatio = eff.OutputRatio / eff.InputRatio
			} else {
				usedCompletionRatio = 1.0
			}
		}
	}

	cachedPrompt := 0
	cachedPromptRaw := 0
	if usage.PromptTokensDetails != nil {
		cachedPromptRaw = max(usage.PromptTokensDetails.CachedTokens, 0)
	}
	nonCachedPrompt := max(promptTokens, 0)
	nonCachedCompletion := completionTokens

	normalInputPrice := usedModelRatio * input.GroupRatio
	normalOutputPrice := usedModelRatio * usedCompletionRatio * input.GroupRatio

	cachedInputPrice := normalInputPrice
	if eff.CachedInputRatio < 0 {
		cachedInputPrice = 0
	} else if eff.CachedInputRatio > 0 {
		cachedInputPrice = eff.CachedInputRatio * input.GroupRatio
	}

	write5m := usage.CacheWrite5mTokens
	write1h := usage.CacheWrite1hTokens
	if write5m < 0 {
		write5m = 0
	}
	if write1h < 0 {
		write1h = 0
	}

	isClaudeModel := isClaudeModelName(input.ModelName)

	// Some providers (e.g. Anthropic Claude prompt caching) report prompt tokens as
	// post-breakpoint input only, while cache-read/write tokens are separate buckets.
	// In that case, we should not clamp or subtract cache buckets from prompt tokens.
	promptExcludesCacheBuckets := isClaudeModel || cachedPromptRaw > promptTokens || write5m+write1h > promptTokens
	if promptExcludesCacheBuckets {
		cachedPrompt = cachedPromptRaw
	} else {
		cachedPrompt = min(cachedPromptRaw, promptTokens)
		nonCachedPrompt = promptTokens - cachedPrompt
		if write5m+write1h > nonCachedPrompt {
			writeExcess := write5m + write1h - nonCachedPrompt
			if write1h >= writeExcess {
				write1h -= writeExcess
			} else {
				writeExcess -= write1h
				write1h = 0
				if write5m >= writeExcess {
					write5m -= writeExcess
				} else {
					write5m = 0
				}
			}
			nonCachedPrompt = 0
		} else {
			nonCachedPrompt -= write5m + write1h
		}
	}

	write5mPrice := normalInputPrice
	if eff.CacheWrite5mRatio < 0 {
		write5mPrice = 0
	} else if eff.CacheWrite5mRatio > 0 {
		write5mPrice = eff.CacheWrite5mRatio * input.GroupRatio
	}

	write1hPrice := normalInputPrice
	if eff.CacheWrite1hRatio < 0 {
		write1hPrice = 0
	} else if eff.CacheWrite1hRatio > 0 {
		write1hPrice = eff.CacheWrite1hRatio * input.GroupRatio
	}

	promptCost := float64(nonCachedPrompt) * normalInputPrice
	if hasResolvedModelCfg {
		if multimodalPromptCost, ok := computeEmbeddingPromptCost(nonCachedPrompt, usage.PromptTokensDetails, resolvedModelCfg.Embedding, input.GroupRatio, normalInputPrice); ok {
			promptCost = multimodalPromptCost
		}
	}

	cost := promptCost + float64(cachedPrompt)*cachedInputPrice +
		float64(nonCachedCompletion)*normalOutputPrice +
		float64(write5m)*write5mPrice + float64(write1h)*write1hPrice

	totalQuota := int64(math.Ceil(cost)) + usage.ToolsCost
	if (usedModelRatio*input.GroupRatio) != 0 && totalQuota <= 0 {
		logger.Logger.Debug("quota calculation clamped to minimum charge",
			zap.String("model_name", input.ModelName),
			zap.Int("prompt_tokens", promptTokens),
			zap.Int("completion_tokens", completionTokens),
			zap.Float64("raw_cost", cost),
			zap.Float64("model_ratio", usedModelRatio),
			zap.Float64("group_ratio", input.GroupRatio),
			zap.Float64("completion_ratio", usedCompletionRatio),
		)
		totalQuota = 1
	}

	return ComputeResult{
		TotalQuota:          totalQuota,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		CachedPromptTokens:  cachedPrompt,
		UsedModelRatio:      usedModelRatio,
		UsedCompletionRatio: usedCompletionRatio,
	}
}

// hasOverrideForModel reports whether overrides contains modelName, preserving explicit zero values.
func hasOverrideForModel(modelName string, overrides map[string]float64) bool {
	if overrides == nil {
		return false
	}
	_, ok := overrides[modelName]
	return ok
}

// resolveCompletionRatio returns the effective completion ratio for modelName.
// Parameters: resolvedModelCfg is the already-loaded model config and hasResolvedModelCfg reports whether it exists.
// Returns: the completion ratio after applying channel overrides, config values, and fallback pricing lookup.
func resolveCompletionRatio(
	modelName string,
	resolvedModelCfg adaptor.ModelConfig,
	hasResolvedModelCfg bool,
	channelOverrides map[string]float64,
	provider adaptor.Adaptor,
) float64 {
	if override, ok := channelOverrides[modelName]; ok {
		return override
	}
	if hasResolvedModelCfg && resolvedModelCfg.CompletionRatio != 0 {
		return resolvedModelCfg.CompletionRatio
	}
	return pricing.GetCompletionRatioWithThreeLayers(modelName, channelOverrides, provider)
}

// isClaudeModelName reports whether modelName contains the ASCII token "claude" regardless of case.
func isClaudeModelName(modelName string) bool {
	return containsASCIIFold(modelName, "claude")
}

// containsASCIIFold reports whether s contains substr using ASCII-only case folding.
// Parameters: s is the model name and substr is the lowercase ASCII token to match.
// Returns: true when substr appears in s without allocating a normalized copy.
func containsASCIIFold(s string, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	// substr is already expected to be lowercase from the caller (isClaudeModelName).
	// We pre-calculate the uppercase variant of the first byte to allow a fast search
	// that avoids calling asciiLower on every character in the model name string.
	firstLower := substr[0]
	var firstUpper byte
	if firstLower >= 'a' && firstLower <= 'z' {
		firstUpper = firstLower - ('a' - 'A')
	} else {
		firstUpper = firstLower
	}

	last := len(s) - len(substr)
	for i := 0; i <= last; i++ {
		// Fast path: match the first byte against both possible cases.
		if s[i] != firstLower && s[i] != firstUpper {
			continue
		}

		matched := true
		for j := 1; j < len(substr); j++ {
			if asciiLower(s[i+j]) != substr[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
// asciiLower converts ASCII uppercase bytes to lowercase.
// Parameter: b is the byte to normalize.
// Returns: the lowercase byte when b is an ASCII uppercase letter, otherwise b unchanged.
func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// computeEmbeddingPromptCost calculates modality-aware prompt billing for embedding models.
// It returns false when the usage snapshot does not contain multimodal embedding details.
func computeEmbeddingPromptCost(promptTokens int, details *relaymodel.UsagePromptTokensDetails, cfg *adaptor.EmbeddingPricingConfig, groupRatio float64, fallbackTokenPrice float64) (float64, bool) {
	if cfg == nil || !cfg.HasData() || details == nil {
		return 0, false
	}

	hasDetailedUsage := details.TextTokens > 0 || details.ImageTokens > 0 || details.AudioTokens > 0 ||
		details.VideoTokens > 0 || details.DocumentTokens > 0 || details.ImageCount > 0 || details.AudioSeconds > 0 ||
		details.VideoFrames > 0 || details.DocumentPages > 0
	if !hasDetailedUsage {
		return 0, false
	}

	textRatio := cfg.TextTokenRatio
	if textRatio == 0 {
		if groupRatio == 0 {
			textRatio = 0
		} else {
			textRatio = fallbackTokenPrice / groupRatio
		}
	}
	imageRatio := resolveEmbeddingTokenRatio(cfg.ImageTokenRatio, textRatio)
	audioRatio := resolveEmbeddingTokenRatio(cfg.AudioTokenRatio, textRatio)
	videoRatio := resolveEmbeddingTokenRatio(cfg.VideoTokenRatio, textRatio)
	documentRatio := resolveEmbeddingTokenRatio(cfg.DocumentTokenRatio, textRatio)

	countedPromptTokens := max(details.TextTokens, 0) + max(details.ImageTokens, 0) +
		max(details.AudioTokens, 0) + max(details.VideoTokens, 0) + max(details.DocumentTokens, 0)
	remainingPromptTokens := max(promptTokens-countedPromptTokens, 0)

	cost := float64(max(details.TextTokens, 0))*textRatio*groupRatio +
		float64(max(details.ImageTokens, 0))*imageRatio*groupRatio +
		float64(max(details.AudioTokens, 0))*audioRatio*groupRatio +
		float64(max(details.VideoTokens, 0))*videoRatio*groupRatio +
		float64(max(details.DocumentTokens, 0))*documentRatio*groupRatio +
		float64(remainingPromptTokens)*documentRatio*groupRatio

	if details.ImageTokens == 0 && details.ImageCount > 0 && cfg.UsdPerImage > 0 {
		cost += float64(details.ImageCount) * cfg.UsdPerImage * billingratio.QuotaPerUsd * groupRatio
	}
	if details.AudioTokens == 0 && details.AudioSeconds > 0 && cfg.UsdPerAudioSecond > 0 {
		cost += details.AudioSeconds * cfg.UsdPerAudioSecond * billingratio.QuotaPerUsd * groupRatio
	}
	if details.VideoTokens == 0 && details.VideoFrames > 0 && cfg.UsdPerVideoFrame > 0 {
		cost += float64(details.VideoFrames) * cfg.UsdPerVideoFrame * billingratio.QuotaPerUsd * groupRatio
	}
	if details.DocumentPages > 0 && cfg.UsdPerDocumentPage > 0 {
		cost += float64(details.DocumentPages) * cfg.UsdPerDocumentPage * billingratio.QuotaPerUsd * groupRatio
	}

	return cost, true
}

// resolveEmbeddingTokenRatio returns fallback when the modality-specific embedding token ratio is unset.
func resolveEmbeddingTokenRatio(value float64, fallback float64) float64 {
	if value != 0 {
		return value
	}
	return fallback
}
