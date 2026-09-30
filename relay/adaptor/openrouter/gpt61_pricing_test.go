package openrouter_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGPT61SolProviderPricing verifies OpenRouter's published min_prompt_tokens
// override through the production resolver without overwriting native defaults.
func TestGPT61SolProviderPricing(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	for _, name := range []string{"openai/gpt-6.1-sol", "openai/gpt-6.1-sol-pro"} {
		for _, inputTokens := range []int{0, 1, 271_999, 272_000, 272_001, 500_000} {
			t.Run(name+"/"+strconv.Itoa(inputTokens), func(t *testing.T) {
				effective := pricing.ResolveEffectivePricingForUsage(name, inputTokens, 128_000, a)
				input, cached, write, output, threshold := 2.0, 0.1, 2.5, 10.0, 0
				// The provider's machine-readable minimum is inclusive; do not
				// replace it with the separately documented native OpenAI threshold.
				if inputTokens >= 272_000 {
					input, cached, write, output, threshold = 4.0, 0.2, 5.0, 15.0, 272_000
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
	native := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	require.NotContains(t, native.GetModelList(), "openai/gpt-6.1-sol-pro")
	require.NotContains(t, native.GetModelList(), "gpt-6.1-sol-pro")
	effective := pricing.ResolveEffectivePricingForUsage("gpt-6.1-sol", 272_000, 128_000, native)
	require.Zero(t, effective.AppliedTierThreshold)
	require.InDelta(t, 2.0, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
}
