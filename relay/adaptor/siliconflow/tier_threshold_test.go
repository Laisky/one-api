package siliconflow_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestSiliconFlowInputLengthTiers pins the CNY input-length tiers published on
// https://www.siliconflow.cn/pricing as 输入 [0, Nk) / 输入 [Nk, +∞). The long
// tier applies from N itself, with k read as 1000 (the never under-billing
// reading), so N-1 tokens bill at the base rate and N tokens at the long rate.
// It takes t and returns nothing.
func TestSiliconFlowInputLengthTiers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                               string
		threshold                        int
		baseIn, baseOut, longIn, longOut float64
	}{
		{id: "Qwen/Qwen3.5-9B", threshold: 128000, baseIn: 0.5, baseOut: 4, longIn: 1.5, longOut: 12},
		{id: "Qwen/Qwen3.5-27B", threshold: 128000, baseIn: 0.6, baseOut: 4.8, longIn: 1.8, longOut: 14.4},
		{id: "Qwen/Qwen3.5-35B-A3B", threshold: 128000, baseIn: 0.4, baseOut: 3.2, longIn: 1.6, longOut: 12.8},
		{id: "Qwen/Qwen3.5-122B-A10B", threshold: 128000, baseIn: 0.8, baseOut: 6.4, longIn: 2, longOut: 16},
		{id: "Pro/zai-org/GLM-5.1", threshold: 32000, baseIn: 6, baseOut: 24, longIn: 8, longOut: 28},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := siliconflow.ModelRatios[tc.id]
			require.True(t, ok)
			require.Len(t, cfg.Tiers, 1)
			require.Equal(t, tc.threshold, cfg.Tiers[0].InputTokenThreshold)

			base := pricing.ResolveEffectivePricingFromConfig(tc.threshold-1, cfg)
			require.InDelta(t, tc.baseIn, base.InputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.baseOut, base.OutputRatio/ratio.MilliTokensRmb, 1e-9)

			long := pricing.ResolveEffectivePricingFromConfig(tc.threshold, cfg)
			require.InDelta(t, tc.longIn, long.InputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.longOut, long.OutputRatio/ratio.MilliTokensRmb, 1e-9)
		})
	}
}
