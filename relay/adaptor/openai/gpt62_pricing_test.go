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

// TestGPT62EffectivePricing checks actual tariff resolution across the input-only
// 272K boundary, including output and cache writes. Parameter t reports failures.
func TestGPT62EffectivePricing(t *testing.T) {
	t.Parallel()
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	for _, tc := range []struct {
		name                         string
		input, cached, write, output float64
	}{
		{"gpt-6.2-astra", 8, .4, 10, 40},
		{"gpt-6.2-sol", 2, .1, 2.5, 10},
	} {
		for _, inputTokens := range []int{0, 1, 271_999, 272_000, 272_001, 922_000} {
			for _, outputTokens := range []int{0, 128_000} {
				t.Run(tc.name+"/"+strconv.Itoa(inputTokens)+"/"+strconv.Itoa(outputTokens), func(t *testing.T) {
					effective := pricing.ResolveEffectivePricingForUsage(tc.name, inputTokens, outputTokens, provider)
					input, cached, write, output, threshold := tc.input, tc.cached, tc.write, tc.output, 0
					if inputTokens > 272_000 {
						input, cached, write, output, threshold = input*2, cached*2, write*2, output*1.5, 272_001
					}
					require.Equal(t, threshold, effective.AppliedTierThreshold)
					require.Zero(t, effective.AppliedOutputTierThreshold)
					require.InDelta(t, input, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
					require.InDelta(t, cached, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
					require.InDelta(t, write, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
					require.InDelta(t, output, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
					require.Zero(t, effective.CacheWrite1hRatio, "12h writes must not be mislabeled as Anthropic 1h")
				})
			}
		}
		cfg := provider.GetDefaultModelPricing()[tc.name]
		require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-9)
		require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
		require.Len(t, cfg.Tiers, 1)
		require.Equal(t, 272_001, cfg.Tiers[0].InputTokenThreshold)
	}
}
