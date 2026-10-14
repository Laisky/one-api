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

// TestOpenAIOctoberSnapshotEffectivePricing checks the production pricing
// resolver at the input-only long-context boundary. Parameter t owns assertions;
// the test returns no values and never contacts a paid upstream service.
func TestOpenAIOctoberSnapshotEffectivePricing(t *testing.T) {
	a := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	for _, name := range []string{"gpt-5.4-pro-2026-03-05", "gpt-5.5-pro-2026-04-23"} {
		for _, input := range []int{1, 271999, 272000, 272001, 300000} {
			for _, output := range []int{0, 128000} {
				t.Run(name+"/"+strconv.Itoa(input)+"/"+strconv.Itoa(output), func(t *testing.T) {
					effective := pricing.ResolveEffectivePricingForUsage(name, input, output, a)
					inUSD, outUSD, threshold := 30.0, 180.0, 0
					if input > 272000 {
						inUSD, outUSD, threshold = 60, 270, 272001
					}
					require.InDelta(t, inUSD, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
					require.InDelta(t, outUSD, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
					require.Equal(t, threshold, effective.AppliedTierThreshold)
					require.Zero(t, effective.AppliedOutputTierThreshold)
					// Zero is the existing no-discount sentinel, not free cache reads.
					require.Zero(t, a.GetDefaultModelPricing()[name].CachedInputRatio)
				})
			}
		}
	}
	chat := pricing.ResolveEffectivePricingForUsage("chat-latest", 300000, 100, a)
	require.InDelta(t, 5.0, chat.InputRatio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 30.0, chat.OutputRatio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 0.5, chat.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.Zero(t, chat.AppliedTierThreshold, "do not infer a GPT-5.5 surcharge from the alias name")
	for _, name := range []string{"gpt-4o-mini-transcribe-2025-03-20", "gpt-4o-mini-transcribe-2025-12-15"} {
		cfg := a.GetDefaultModelPricing()[name]
		require.InDelta(t, 1.25, cfg.Ratio/ratio.MilliTokensUsd, 1e-9)
		require.InDelta(t, 5.0, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-9)
		require.NotNil(t, cfg.Audio)
		require.InDelta(t, 3.0, cfg.Ratio*cfg.Audio.PromptRatio/ratio.MilliTokensUsd, 1e-9)
	}
}
