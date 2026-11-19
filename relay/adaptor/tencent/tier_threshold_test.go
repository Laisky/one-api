package tencent_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/tencent"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestHy3PreviewTierThresholds pins the TokenHub hy3-preview input-length tiers.
// TokenHub published 输入长度（0, 16k） / [16k, 32k） / [32k+）, so each surcharge
// applies from the boundary itself; k is read as 1000, the smaller and never
// under-billing reading. It takes t and returns nothing.
func TestHy3PreviewTierThresholds(t *testing.T) {
	t.Parallel()
	cfg, ok := (&tencent.Adaptor{}).GetDefaultModelPricing()["hy3-preview"]
	require.True(t, ok)
	require.Len(t, cfg.Tiers, 2)
	require.Equal(t, 16000, cfg.Tiers[0].InputTokenThreshold)
	require.Equal(t, 32000, cfg.Tiers[1].InputTokenThreshold)

	for _, tc := range []struct {
		inputTokens             int
		input, output, cachedIn float64
	}{
		{inputTokens: 15999, input: 1.2, output: 4, cachedIn: 0.4},
		{inputTokens: 16000, input: 1.6, output: 6.4, cachedIn: 0.6},
		{inputTokens: 31999, input: 1.6, output: 6.4, cachedIn: 0.6},
		{inputTokens: 32000, input: 2, output: 8, cachedIn: 0.8},
	} {
		resolved := pricing.ResolveEffectivePricingFromConfig(tc.inputTokens, cfg)
		require.InDelta(t, tc.input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-9, "input at %d tokens", tc.inputTokens)
		require.InDelta(t, tc.output, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-9, "output at %d tokens", tc.inputTokens)
		require.InDelta(t, tc.cachedIn, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-9, "cached input at %d tokens", tc.inputTokens)
	}
}
