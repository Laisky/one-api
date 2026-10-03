package ali_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	relay "github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// TestQwenCachedInputBillsAtDiscount verifies that, with the Qwen CachedInputRatio set,
// quota.Compute bills the cached portion at ~0.2x the standard input price (Alibaba
// implicit cache: cached input billed at 20% of standard input token price), not 1.0x.
func TestQwenCachedInputBillsAtDiscount(t *testing.T) {
	t.Parallel()

	const modelName = "qwen-plus"
	adaptor := relay.GetAdaptor(apitype.Ali)
	require.NotNil(t, adaptor, "nil adaptor for api type %d", apitype.Ali)

	modelRatio := adaptor.GetModelRatio(modelName)
	require.Greater(t, modelRatio, 0.0, "unexpected model ratio: %v", modelRatio)
	groupRatio := 1.0

	const promptTokens = 100_000
	const completionTokens = 1_000
	const cachedPrompt = 60_000

	// CachedInputRatio must be present and distinctly below the input ratio.
	eff := pricing.ResolveEffectivePricing(modelName, promptTokens, adaptor)
	require.Greater(t, eff.CachedInputRatio, 0.0, "qwen model must define a positive cached input ratio")
	require.InDelta(t, 0.2*modelRatio, eff.CachedInputRatio, modelRatio*1e-9,
		"cached input ratio should be 0.2x the input ratio")

	baseUsage := &model.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}
	base := quotautil.Compute(quotautil.ComputeInput{
		Usage:          baseUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	cachedUsage := &model.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: &model.UsagePromptTokensDetails{
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

	normalInputPrice := base.UsedModelRatio * groupRatio
	cachedInputPrice := eff.CachedInputRatio * groupRatio

	// Cached request must cost less: the cached portion is repriced from 1.0x to 0.2x.
	require.Less(t, cached.TotalQuota, base.TotalQuota,
		"cache hits must reduce the quota (cached portion billed below input price)")
	require.Equal(t, cachedPrompt, cached.CachedPromptTokens)

	expectedDelta := int64(math.Ceil(float64(cachedPrompt) * (cachedInputPrice - normalInputPrice)))
	actualDelta := cached.TotalQuota - base.TotalQuota
	require.InDelta(t, expectedDelta, actualDelta, 2,
		"unexpected quota delta: got %d, want ~%d (base=%d cached=%d)", actualDelta, expectedDelta, base.TotalQuota, cached.TotalQuota)
}
