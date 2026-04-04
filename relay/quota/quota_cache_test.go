package quota_test

import (
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	model "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/apitype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

type stubQuotaAdaptor struct {
	pricing             map[string]adaptor.ModelConfig
	defaultPricingCalls int
}

// Init initializes the adaptor.
func (s *stubQuotaAdaptor) Init(meta *metalib.Meta) {}

// GetRequestURL returns an empty URL for tests.
func (s *stubQuotaAdaptor) GetRequestURL(meta *metalib.Meta) (string, error) { return "", nil }

// SetupRequestHeader is a no-op for tests.
func (s *stubQuotaAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *metalib.Meta) error {
	return nil
}

// ConvertRequest is unused in quota tests.
func (s *stubQuotaAdaptor) ConvertRequest(c *gin.Context, relayMode int, request *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}

// ConvertImageRequest is unused in quota tests.
func (s *stubQuotaAdaptor) ConvertImageRequest(c *gin.Context, request *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}

// ConvertClaudeRequest is unused in quota tests.
func (s *stubQuotaAdaptor) ConvertClaudeRequest(c *gin.Context, request *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}

// DoRequest is unused in quota tests.
func (s *stubQuotaAdaptor) DoRequest(c *gin.Context, meta *metalib.Meta, requestBody io.Reader) (*http.Response, error) {
	return nil, nil
}

// DoResponse is unused in quota tests.
func (s *stubQuotaAdaptor) DoResponse(c *gin.Context, resp *http.Response, meta *metalib.Meta) (usage *relaymodel.Usage, err *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}

// GetModelList returns the models defined in the test pricing map.
func (s *stubQuotaAdaptor) GetModelList() []string { return nil }

// GetChannelName returns a stable test name.
func (s *stubQuotaAdaptor) GetChannelName() string { return "stub" }

// GetDefaultModelPricing returns the test pricing map.
func (s *stubQuotaAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	s.defaultPricingCalls++
	return s.pricing
}

// GetModelRatio returns the configured base model ratio.
func (s *stubQuotaAdaptor) GetModelRatio(modelName string) float64 { return s.pricing[modelName].Ratio }

// GetCompletionRatio returns the configured base completion ratio.
func (s *stubQuotaAdaptor) GetCompletionRatio(modelName string) float64 {
	return s.pricing[modelName].CompletionRatio
}

// TestComputeEmbeddingPromptCostUsesModalityTokenRatios verifies modality-specific token ratios override the base input ratio for multimodal embedding billing.
func TestComputeEmbeddingPromptCostUsesModalityTokenRatios(t *testing.T) {
	t.Parallel()

	usage := &relaymodel.Usage{
		PromptTokens: 100,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			TextTokens:  60,
			ImageTokens: 30,
			AudioTokens: 10,
		},
	}

	pricingAdaptor := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		"gemini-embedding-2-preview": {
			Ratio: 0.20 * 0.5,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio:  0.20 * 0.5,
				ImageTokenRatio: 0.45 * 0.5,
				AudioTokenRatio: 6.50 * 0.5,
			},
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      "gemini-embedding-2-preview",
		ModelRatio:     0.20 * 0.5,
		GroupRatio:     1,
		PricingAdaptor: pricingAdaptor,
	})

	expected := int64(math.Ceil((60 * 0.20 * 0.5) + (30 * 0.45 * 0.5) + (10 * 6.50 * 0.5)))
	require.Equal(t, expected, result.TotalQuota)
	require.Equal(t, 100, result.PromptTokens)
}

// TestComputeUnknownModelFallbackUsesQuotaUnits verifies that unknown-model fallback pricing
// is expressed in internal quota units, preventing large requests from being underbilled.
func TestComputeUnknownModelFallbackUsesQuotaUnits(t *testing.T) {
	t.Parallel()

	modelRatio := pricing.GetModelRatioWithThreeLayers("unknown-billing-model", nil, nil)
	require.Equal(t, 2.5*0.5, modelRatio)

	usage := &relaymodel.Usage{
		PromptTokens:     12000,
		CompletionTokens: 8000,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:      usage,
		ModelName:  "unknown-billing-model",
		ModelRatio: modelRatio,
		GroupRatio: 1,
	})

	require.Equal(t, int64(25000), result.TotalQuota)
	require.Greater(t, result.TotalQuota, int64(1), "unknown-model fallback should not collapse large requests to the minimum charge")
}

// TestComputeEmbeddingPromptCostUsesPerUnitFallbacks verifies direct per-unit embedding prices are used when token breakdowns are unavailable.
func TestComputeEmbeddingPromptCostUsesPerUnitFallbacks(t *testing.T) {
	t.Parallel()

	usage := &relaymodel.Usage{
		PromptTokens: 0,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			ImageCount:   2,
			AudioSeconds: 5,
			VideoFrames:  3,
		},
	}

	pricingAdaptor := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		"gemini-embedding-2-preview": {
			Ratio: 0.20 * 0.5,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio:    0.20 * 0.5,
				UsdPerImage:       0.00012,
				UsdPerAudioSecond: 0.00016,
				UsdPerVideoFrame:  0.00079,
			},
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      "gemini-embedding-2-preview",
		ModelRatio:     0.20 * 0.5,
		GroupRatio:     1,
		PricingAdaptor: pricingAdaptor,
	})

	expected := int64(math.Ceil((2*0.00012 + 5*0.00016 + 3*0.00079) * 500000))
	require.Equal(t, expected, result.TotalQuota)
}

// TestComputeEmbeddingPromptCostFallsBackWithoutDetails verifies legacy usage payloads remain billed with the base ratio.
func TestComputeEmbeddingPromptCostFallsBackWithoutDetails(t *testing.T) {
	t.Parallel()

	usage := &relaymodel.Usage{
		PromptTokens: 100,
	}

	pricingAdaptor := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		"gemini-embedding-2-preview": {
			Ratio: 0.20 * 0.5,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio:  0.20 * 0.5,
				ImageTokenRatio: 0.45 * 0.5,
				AudioTokenRatio: 6.50 * 0.5,
			},
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      "gemini-embedding-2-preview",
		ModelRatio:     0.20 * 0.5,
		GroupRatio:     1,
		PricingAdaptor: pricingAdaptor,
	})

	expected := int64(math.Ceil(100 * 0.20 * 0.5))
	require.Equal(t, expected, result.TotalQuota)
}

// TestComputeCachedInputPricing verifies that cached input tokens are billed using CachedInputRatio
// while completion tokens always use Ratio * CompletionRatio irrespective of cache hits.
func TestComputeCachedInputPricing(t *testing.T) {
	t.Parallel()
	modelName := "gpt-4o"
	adaptor := relay.GetAdaptor(apitype.OpenAI)
	require.NotNil(t, adaptor, "nil adaptor for api type %d", apitype.OpenAI)

	modelRatio := adaptor.GetModelRatio(modelName)
	require.Greater(t, modelRatio, 0.0, "unexpected model ratio: %v", modelRatio)
	groupRatio := 0.75

	promptTokens := 480_000
	completionTokens := 220_000
	cachedPrompt := int(float64(promptTokens) * 0.55)

	baseUsage := &relaymodel.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}
	base := quotautil.Compute(quotautil.ComputeInput{
		Usage:          baseUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	eff := pricing.ResolveEffectivePricing(modelName, promptTokens, adaptor)
	normalInputPrice := base.UsedModelRatio * groupRatio
	cachedInputPrice := normalInputPrice
	if eff.CachedInputRatio < 0 {
		cachedInputPrice = 0
	} else if eff.CachedInputRatio > 0 {
		cachedInputPrice = eff.CachedInputRatio * groupRatio
	}
	if math.Abs(cachedInputPrice-normalInputPrice) < 1e-12 {
		t.Skipf("model %s lacks distinct cached input pricing", modelName)
	}

	cachedUsage := &relaymodel.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: cachedPrompt,
		},
	}
	cached := quotautil.Compute(quotautil.ComputeInput{
		Usage:          cachedUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	expectedDelta := int64(math.Ceil(float64(cachedPrompt) * (cachedInputPrice - normalInputPrice)))
	actualDelta := cached.TotalQuota - base.TotalQuota
	require.InDelta(t, expectedDelta, actualDelta, 2, "unexpected quota delta: got %d, want ~%d (+/-2). base=%d cached=%d", actualDelta, expectedDelta, base.TotalQuota, cached.TotalQuota)

	require.InDelta(t, base.UsedCompletionRatio, cached.UsedCompletionRatio, 1e-12, "completion ratio changed due to cached prompt tokens: base=%.6f cached=%.6f", base.UsedCompletionRatio, cached.UsedCompletionRatio)
}

// TestComputeClaudePromptExcludesCacheBuckets verifies Claude-style usage accounting where
// input_tokens excludes cache-read/write buckets and those buckets must be billed independently.
func TestComputeClaudePromptExcludesCacheBuckets(t *testing.T) {
	t.Parallel()

	modelName := "claude-4.6-sonnet"
	adaptor := relay.GetAdaptor(apitype.Anthropic)
	require.NotNil(t, adaptor, "nil adaptor for api type %d", apitype.Anthropic)

	modelRatio := adaptor.GetModelRatio(modelName)
	require.Greater(t, modelRatio, 0.0, "unexpected model ratio: %v", modelRatio)

	groupRatio := 1.0
	usage := &relaymodel.Usage{
		PromptTokens:     1,
		CompletionTokens: 8,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 1,
		},
		CacheWrite5mTokens: 63277,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	eff := pricing.ResolveEffectivePricing(modelName, usage.PromptTokens, adaptor)
	normalInputPrice := result.UsedModelRatio * groupRatio
	normalOutputPrice := result.UsedModelRatio * result.UsedCompletionRatio * groupRatio

	cachedInputPrice := normalInputPrice
	if eff.CachedInputRatio < 0 {
		cachedInputPrice = 0
	} else if eff.CachedInputRatio > 0 {
		cachedInputPrice = eff.CachedInputRatio * groupRatio
	}

	write5mPrice := normalInputPrice
	if eff.CacheWrite5mRatio < 0 {
		write5mPrice = 0
	} else if eff.CacheWrite5mRatio > 0 {
		write5mPrice = eff.CacheWrite5mRatio * groupRatio
	}

	expected := int64(math.Ceil(
		float64(usage.PromptTokens)*normalInputPrice +
			float64(usage.CompletionTokens)*normalOutputPrice +
			float64(usage.PromptTokensDetails.CachedTokens)*cachedInputPrice +
			float64(usage.CacheWrite5mTokens)*write5mPrice,
	))

	require.Equal(t, expected, result.TotalQuota)
}

// TestComputeClaudePromptExcludesCacheBucketsCaseInsensitive verifies Claude model detection remains case-insensitive without lowercasing allocations.
func TestComputeClaudePromptExcludesCacheBucketsCaseInsensitive(t *testing.T) {
	t.Parallel()

	const modelName = "ClAuDe-hot-path-test"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:             3,
			CompletionRatio:   5,
			CachedInputRatio:  1,
			CacheWrite5mRatio: 2,
		},
	}}

	usage := &relaymodel.Usage{
		PromptTokens:     2,
		CompletionTokens: 4,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 3,
		},
		CacheWrite5mTokens: 7,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     3,
		GroupRatio:     1,
		PricingAdaptor: provider,
	})

	expected := int64(math.Ceil(
		float64(usage.PromptTokens)*3 +
			float64(usage.CompletionTokens)*3*5 +
			float64(usage.PromptTokensDetails.CachedTokens)*1 +
			float64(usage.CacheWrite5mTokens)*2,
	))

	require.Equal(t, expected, result.TotalQuota)
	require.Equal(t, usage.PromptTokensDetails.CachedTokens, result.CachedPromptTokens)
}

// TestComputeLegacyPromptIncludesCacheBuckets verifies overlap clamping remains intact for
// providers whose prompt_tokens already include cache-read/write buckets.
func TestComputeLegacyPromptIncludesCacheBuckets(t *testing.T) {
	t.Parallel()

	modelName := "gpt-4o"
	adaptor := relay.GetAdaptor(apitype.OpenAI)
	require.NotNil(t, adaptor, "nil adaptor for api type %d", apitype.OpenAI)

	modelRatio := adaptor.GetModelRatio(modelName)
	require.Greater(t, modelRatio, 0.0, "unexpected model ratio: %v", modelRatio)

	usage := &relaymodel.Usage{
		PromptTokens:     100,
		CompletionTokens: 10,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 20,
		},
		CacheWrite5mTokens: 90,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     1.0,
		PricingAdaptor: adaptor,
	})

	eff := pricing.ResolveEffectivePricing(modelName, usage.PromptTokens, adaptor)
	normalInputPrice := result.UsedModelRatio
	normalOutputPrice := result.UsedModelRatio * result.UsedCompletionRatio

	cachedInputPrice := normalInputPrice
	if eff.CachedInputRatio < 0 {
		cachedInputPrice = 0
	} else if eff.CachedInputRatio > 0 {
		cachedInputPrice = eff.CachedInputRatio
	}

	write5mPrice := normalInputPrice
	if eff.CacheWrite5mRatio < 0 {
		write5mPrice = 0
	} else if eff.CacheWrite5mRatio > 0 {
		write5mPrice = eff.CacheWrite5mRatio
	}

	// legacy overlap logic:
	// nonCachedPrompt = 100 - 20 = 80
	// write5m 90 exceeds nonCachedPrompt by 10 => effective write5m 80, nonCachedPrompt 0
	effectiveWrite5m := 80
	expected := int64(math.Ceil(
		float64(20)*cachedInputPrice +
			float64(10)*normalOutputPrice +
			float64(effectiveWrite5m)*write5mPrice,
	))

	require.Equal(t, expected, result.TotalQuota)
}

// TestComputeChannelTierPricingWithInheritedBase verifies channel tier overrides
// are effective even when channel base ratio/completion are omitted (zero values).
func TestComputeChannelTierPricingWithInheritedBase(t *testing.T) {
	t.Parallel()

	modelName := "gpt-4o"
	adaptor := relay.GetAdaptor(apitype.OpenAI)
	require.NotNil(t, adaptor, "nil adaptor for api type %d", apitype.OpenAI)

	baseModelRatio := adaptor.GetModelRatio(modelName)
	baseCompletionRatio := adaptor.GetCompletionRatio(modelName)
	require.Greater(t, baseModelRatio, 0.0, "unexpected model ratio: %v", baseModelRatio)
	require.Greater(t, baseCompletionRatio, 0.0, "unexpected completion ratio: %v", baseCompletionRatio)

	usage := &relaymodel.Usage{
		PromptTokens:     120,
		CompletionTokens: 50,
	}

	channelModelConfigs := map[string]model.ModelConfigLocal{
		modelName: {
			// Keep base values zero to mimic inherited legacy behavior.
			Tiers: []model.ModelRatioTierLocal{
				{
					InputTokenThreshold: 100,
					Ratio:               baseModelRatio * 2,
					CompletionRatio:     3,
				},
			},
		},
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:               usage,
		ModelName:           modelName,
		ModelRatio:          baseModelRatio,
		GroupRatio:          1.0,
		ChannelModelConfigs: channelModelConfigs,
		PricingAdaptor:      adaptor,
	})

	expectedInputRatio := baseModelRatio * 2
	expectedCompletionRatio := 3.0
	expectedQuota := int64(math.Ceil(
		float64(usage.PromptTokens)*expectedInputRatio +
			float64(usage.CompletionTokens)*expectedInputRatio*expectedCompletionRatio,
	))

	require.InDelta(t, expectedInputRatio, result.UsedModelRatio, 1e-12)
	require.InDelta(t, expectedCompletionRatio, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, expectedQuota, result.TotalQuota)
}

// TestComputeChannelZeroConfigInheritsProviderCompletionRatio verifies zero-valued channel config completion falls back to provider defaults.
func TestComputeChannelZeroConfigInheritsProviderCompletionRatio(t *testing.T) {
	t.Parallel()

	const modelName = "provider-fallback-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:           2,
			CompletionRatio: 4,
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
		},
		ModelName:  modelName,
		ModelRatio: 2,
		GroupRatio: 1,
		ChannelModelConfigs: map[string]model.ModelConfigLocal{
			modelName: {
				Ratio:           0,
				CompletionRatio: 0,
			},
		},
		PricingAdaptor: provider,
	})

	require.InDelta(t, 2.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 4.0, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil(10*2+5*2*4)), result.TotalQuota)
}

// TestComputeUsesSinglePricingLookupWhenConfigProvidesCompletionRatio verifies Compute reuses the resolved config instead of repeating pricing-table traversal.
func TestComputeUsesSinglePricingLookupWhenConfigProvidesCompletionRatio(t *testing.T) {
	t.Parallel()

	const modelName = "single-lookup-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:           2,
			CompletionRatio: 3,
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{
			PromptTokens:     8,
			CompletionTokens: 2,
		},
		ModelName:      modelName,
		ModelRatio:     2,
		GroupRatio:     1,
		PricingAdaptor: provider,
	})

	require.InDelta(t, 2.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 3.0, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, 1, provider.defaultPricingCalls)
}

// TestComputeRetainsChannelModelRatioOverride verifies that a higher-priority channel ratio override
// is preserved even when provider pricing publishes tiered defaults for the same model.
func TestComputeRetainsChannelModelRatioOverride(t *testing.T) {
	t.Parallel()

	const modelName = "override-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:           2,
			CompletionRatio: 4,
			Tiers: []adaptor.ModelRatioTier{{
				InputTokenThreshold: 100,
				Ratio:               5,
				CompletionRatio:     6,
			}},
		},
	}}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{
			PromptTokens:     120,
			CompletionTokens: 10,
		},
		ModelName:         modelName,
		ModelRatio:        3,
		ChannelModelRatio: map[string]float64{modelName: 3},
		GroupRatio:        1,
		PricingAdaptor:    provider,
	})

	require.InDelta(t, 3.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 6.0, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil(120*3+10*3*6)), result.TotalQuota)
}

// TestComputeChannelZeroConfigFallsBackToResolvedOverrides verifies that explicit zero base values
// in channel model config preserve legacy fallback behavior while still applying tier cache ratios.
func TestComputeChannelZeroConfigFallsBackToResolvedOverrides(t *testing.T) {
	t.Parallel()

	const modelName = "zero-config-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:             2,
			CompletionRatio:   3,
			CachedInputRatio:  1,
			CacheWrite5mRatio: 7,
		},
	}}

	usage := &relaymodel.Usage{
		PromptTokens:     120,
		CompletionTokens: 20,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 10,
		},
		CacheWrite5mTokens: 5,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:             usage,
		ModelName:         modelName,
		ModelRatio:        11,
		ChannelModelRatio: map[string]float64{modelName: 11},
		GroupRatio:        1,
		ChannelModelConfigs: map[string]model.ModelConfigLocal{
			modelName: {
				Ratio:           0,
				CompletionRatio: 0,
			},
		},
		ChannelCompletionRatio: map[string]float64{modelName: 13},
		PricingAdaptor:         provider,
	})

	require.InDelta(t, 11.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 13.0, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil(105*11+10*11+20*11*13+5*11)), result.TotalQuota)
}

// TestComputeRatioOnlyResolutionAppliesTieredCacheRatios verifies that the ratio-only resolver still
// preserves tiered cached and cache-write pricing when the tier threshold is met.
func TestComputeRatioOnlyResolutionAppliesTieredCacheRatios(t *testing.T) {
	t.Parallel()

	const modelName = "tier-cache-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{
		modelName: {
			Ratio:             2,
			CompletionRatio:   3,
			CachedInputRatio:  1,
			CacheWrite5mRatio: 8,
			CacheWrite1hRatio: 9,
			Tiers: []adaptor.ModelRatioTier{{
				InputTokenThreshold: 100,
				CachedInputRatio:    4,
				CacheWrite5mRatio:   5,
				CacheWrite1hRatio:   -1,
			}},
		},
	}}

	usage := &relaymodel.Usage{
		PromptTokens:     120,
		CompletionTokens: 10,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 20,
		},
		CacheWrite5mTokens: 5,
		CacheWrite1hTokens: 7,
	}

	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     2,
		GroupRatio:     1,
		PricingAdaptor: provider,
	})

	require.InDelta(t, 2.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 3.0, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil(88*2+20*4+10*2*3+5*5)), result.TotalQuota)
}

// TestComputeResultCachedPromptTokensField verifies that ComputeResult.CachedPromptTokens
// correctly reflects the cached tokens from usage across different model types.
func TestComputeResultCachedPromptTokensField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		modelName      string
		promptTokens   int
		cachedTokens   int
		expectedCached int
		isClaudeStyle  bool // Claude: cache buckets are separate from prompt tokens
		cacheWrite5m   int
		cacheWrite1h   int
	}{
		{
			name:           "openai_with_cache_hit",
			modelName:      "gpt-4o",
			promptTokens:   1000,
			cachedTokens:   600,
			expectedCached: 600,
		},
		{
			name:           "openai_cache_exceeds_prompt_treated_as_separate_buckets",
			modelName:      "gpt-4o",
			promptTokens:   100,
			cachedTokens:   200, // more cached than prompt triggers promptExcludesCacheBuckets
			expectedCached: 200,
		},
		{
			name:           "openai_no_cache",
			modelName:      "gpt-4o",
			promptTokens:   500,
			cachedTokens:   0,
			expectedCached: 0,
		},
		{
			name:           "claude_with_cache_hit",
			modelName:      "claude-sonnet-4-20250514",
			promptTokens:   800,
			cachedTokens:   300,
			expectedCached: 300,
			isClaudeStyle:  true,
		},
		{
			name:           "claude_cache_exceeds_prompt_separate_buckets",
			modelName:      "claude-sonnet-4-20250514",
			promptTokens:   100,
			cachedTokens:   500,
			expectedCached: 500, // Claude: cache buckets are independent
			isClaudeStyle:  true,
		},
		{
			name:           "openai_with_cache_and_write_tokens",
			modelName:      "gpt-4o",
			promptTokens:   1000,
			cachedTokens:   400,
			expectedCached: 400,
			cacheWrite5m:   100,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			usage := &relaymodel.Usage{
				PromptTokens:       tc.promptTokens,
				CompletionTokens:   50,
				TotalTokens:        tc.promptTokens + 50,
				CacheWrite5mTokens: tc.cacheWrite5m,
				CacheWrite1hTokens: tc.cacheWrite1h,
			}
			if tc.cachedTokens > 0 {
				usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{
					CachedTokens: tc.cachedTokens,
				}
			}

			result := quotautil.Compute(quotautil.ComputeInput{
				Usage:      usage,
				ModelName:  tc.modelName,
				ModelRatio: 1.0,
				GroupRatio: 1.0,
			})

			require.Equal(t, tc.expectedCached, result.CachedPromptTokens,
				"CachedPromptTokens mismatch for %s", tc.name)
		})
	}
}
