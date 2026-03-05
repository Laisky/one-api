package quota

import (
	"math"
	"strings"

	"github.com/songquanpeng/one-api/relay/adaptor"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// ComputeInput describes all parameters required to calculate quota consumption
// for a particular usage snapshot.
type ComputeInput struct {
	Usage                  *relaymodel.Usage
	ModelName              string
	ModelRatio             float64
	GroupRatio             float64
	ChannelCompletionRatio map[string]float64
	PricingAdaptor         adaptor.Adaptor
}

// ComputeResult captures the outcome of a quota calculation, including
// normalized ratios used and cached token details.
type ComputeResult struct {
	TotalQuota             int64
	PromptTokens           int
	CompletionTokens       int
	CachedPromptTokens     int
	CachedCompletionTokens int
	UsedModelRatio         float64
	UsedCompletionRatio    float64
}

// Compute calculates the quota required for the provided usage snapshot.
// It mirrors the logic used in controller helper functions so streaming
// billing and final reconciliation share the same pricing semantics.
func Compute(input ComputeInput) ComputeResult {
	usage := input.Usage
	if usage == nil {
		return ComputeResult{}
	}

	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	pricingAdaptor := input.PricingAdaptor
	completionRatioResolved := pricing.GetCompletionRatioWithThreeLayers(input.ModelName, input.ChannelCompletionRatio, pricingAdaptor)

	eff := pricing.ResolveEffectivePricing(input.ModelName, promptTokens, pricingAdaptor)

	usedModelRatio := input.ModelRatio
	usedCompletionRatio := completionRatioResolved
	if pricingAdaptor != nil {
		defaultPricing := pricingAdaptor.GetDefaultModelPricing()
		if _, ok := defaultPricing[input.ModelName]; ok {
			adaptorBase := pricingAdaptor.GetModelRatio(input.ModelName)
			if math.Abs(input.ModelRatio-adaptorBase) < 1e-12 {
				usedModelRatio = eff.InputRatio
				baseComp := eff.OutputRatio
				if eff.InputRatio != 0 {
					baseComp = eff.OutputRatio / eff.InputRatio
				} else {
					baseComp = 1.0
				}
				usedCompletionRatio = baseComp
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

	isClaudeModel := strings.Contains(strings.ToLower(input.ModelName), "claude")

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

	cost := float64(nonCachedPrompt)*normalInputPrice + float64(cachedPrompt)*cachedInputPrice +
		float64(nonCachedCompletion)*normalOutputPrice +
		float64(write5m)*write5mPrice + float64(write1h)*write1hPrice

	totalQuota := int64(math.Ceil(cost)) + usage.ToolsCost
	if (usedModelRatio*input.GroupRatio) != 0 && totalQuota <= 0 {
		totalQuota = 1
	}

	return ComputeResult{
		TotalQuota:             totalQuota,
		PromptTokens:           promptTokens,
		CompletionTokens:       completionTokens,
		CachedPromptTokens:     cachedPrompt,
		CachedCompletionTokens: 0,
		UsedModelRatio:         usedModelRatio,
		UsedCompletionRatio:    usedCompletionRatio,
	}
}
