package quota_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/apitype"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

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
