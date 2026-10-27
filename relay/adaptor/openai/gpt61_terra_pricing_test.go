package openai_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGPT61TerraEffectivePricing verifies Standard and long-context billing
// through the production resolver. Input alone selects the whole-request tier.
func TestGPT61TerraEffectivePricing(t *testing.T) {
	t.Parallel()

	const name = "gpt-6.1-terra"
	a := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	for _, inputTokens := range []int{0, 1, 271_999, 272_000, 272_001, 1_050_000} {
		for _, outputTokens := range []int{0, 128_000} {
			t.Run(strconv.Itoa(inputTokens)+"/"+strconv.Itoa(outputTokens), func(t *testing.T) {
				effective := pricing.ResolveEffectivePricingForUsage(name, inputTokens, outputTokens, a)
				input, cached, write, output, threshold := 1.0, 0.05, 1.25, 5.0, 0
				if inputTokens > 272_000 {
					input, cached, write, output, threshold = 2.0, 0.10, 2.5, 7.5, 272_001
				}
				require.Equal(t, threshold, effective.AppliedTierThreshold)
				require.Zero(t, effective.AppliedOutputTierThreshold)
				require.InDelta(t, input, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, cached, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, write, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, output, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
				require.Zero(t, effective.CacheWrite1hRatio)
			})
		}
	}

	// The resolver must not mutate the catalogue when the expensive tier wins.
	cfg := a.GetDefaultModelPricing()[name]
	require.InDelta(t, 1.0, cfg.Ratio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 0.05, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.Len(t, cfg.Tiers, 1)
	require.Equal(t, 272_001, cfg.Tiers[0].InputTokenThreshold)
}
