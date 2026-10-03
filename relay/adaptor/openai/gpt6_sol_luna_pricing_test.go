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

// TestGPT6SolLunaEffectivePricing verifies the production resolver selects the
// Standard or long-context rate for the entire request, not just excess tokens.
func TestGPT6SolLunaEffectivePricing(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                        string
		input, cached, write, output float64
	}{
		{"gpt-6-sol", 2, 0.2, 2.5, 10},
		{"gpt-6-luna", 0.1, 0.01, 0.125, 0.5},
	} {
		for _, inputTokens := range []int{0, 1, 271_999, 272_000, 272_001, 500_000} {
			t.Run(tc.name+"/"+strconv.Itoa(inputTokens), func(t *testing.T) {
				a := &openai.Adaptor{ChannelType: channeltype.OpenAI}
				// A large output must not trigger the input-only long-context tier.
				effective := pricing.ResolveEffectivePricingForUsage(tc.name, inputTokens, 128_000, a)
				inputMultiplier, outputMultiplier, threshold := 1.0, 1.0, 0
				if inputTokens > 272_000 {
					inputMultiplier, outputMultiplier, threshold = 2.0, 1.5, 272_001
				}
				require.Equal(t, threshold, effective.AppliedTierThreshold)
				require.Zero(t, effective.AppliedOutputTierThreshold)
				require.InDelta(t, tc.input*inputMultiplier, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, tc.cached*inputMultiplier, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, tc.write*inputMultiplier, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, tc.output*outputMultiplier, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
				require.Zero(t, effective.CacheWrite1hRatio)
			})
		}
	}
}
