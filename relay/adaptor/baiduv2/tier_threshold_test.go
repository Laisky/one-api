package baiduv2_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/baiduv2"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestQianfanAbove32KTierThresholds pins the Qianfan tiers whose long rate starts
// strictly above 32k input tokens ([0,32k] / (32k,128k] for DeepSeek-V3.2 and
// 输入<=32k / 32k<输入<=128k for ERNIE 5.x). With k normalized to 1000, a prompt
// of exactly 32000 tokens bills at the base rate and 32001 at the long rate.
// It takes t and returns nothing.
func TestQianfanAbove32KTierThresholds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                               string
		baseIn, baseOut, longIn, longOut float64
		baseCached, longCached           float64
	}{
		{id: "deepseek-v3.2", baseIn: 2, baseOut: 3, longIn: 4, longOut: 6, baseCached: 0.4, longCached: 0.4},
		{id: "ernie-5.0", baseIn: 6, baseOut: 24, longIn: 10, longOut: 40},
		{id: "ERNIE-5.0", baseIn: 6, baseOut: 24, longIn: 10, longOut: 40},
		{id: "ernie-5.1", baseIn: 4, baseOut: 18, longIn: 6, longOut: 22},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := baiduv2.ModelRatios[tc.id]
			require.True(t, ok)
			require.Len(t, cfg.Tiers, 1)
			require.Equal(t, 32001, cfg.Tiers[0].InputTokenThreshold)

			base := pricing.ResolveEffectivePricingFromConfig(32000, cfg)
			require.InDelta(t, tc.baseIn, base.InputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.baseOut, base.OutputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.baseCached, base.CachedInputRatio/ratio.MilliTokensRmb, 1e-9)

			long := pricing.ResolveEffectivePricingFromConfig(32001, cfg)
			require.InDelta(t, tc.longIn, long.InputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.longOut, long.OutputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.longCached, long.CachedInputRatio/ratio.MilliTokensRmb, 1e-9)
		})
	}
}

// TestQianfanERNIE50VariantsShareThreshold verifies every hosted ERNIE 5.0 ID,
// whatever its casing or thinking suffix, uses the single published boundary.
// It takes t and returns nothing.
func TestQianfanERNIE50VariantsShareThreshold(t *testing.T) {
	t.Parallel()
	seen := 0
	for id, cfg := range baiduv2.ModelRatios {
		if !strings.HasPrefix(strings.ToLower(id), "ernie-5.0") {
			continue
		}
		seen++
		require.Len(t, cfg.Tiers, 1, id)
		require.Equal(t, 32001, cfg.Tiers[0].InputTokenThreshold, id)
	}
	require.GreaterOrEqual(t, seen, 2)
}
