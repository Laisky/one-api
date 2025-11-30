package quota_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/channeltype"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

// TestComputeCachedInputPricing verifies that cached input tokens are billed using CachedInputRatio
// while completion tokens always use Ratio * CompletionRatio irrespective of cache hits.
func TestComputeCachedInputPricing(t *testing.T) {
	modelName := "gpt-4o"
	adaptor := relay.GetAdaptor(channeltype.OpenAI)
	require.NotNil(t, adaptor, "nil adaptor for channel %d", channeltype.OpenAI)

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
