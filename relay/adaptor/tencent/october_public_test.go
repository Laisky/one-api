package tencent_test

import (
	"github.com/Laisky/one-api/relay/adaptor/tencent"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberTencentLifecycle verifies scoped retirements without changing
// historical rates or claiming a new API transport. It takes t and returns nothing.
func TestOctoberTencentLifecycle(t *testing.T) {
	t.Parallel()
	a := &tencent.Adaptor{}
	for id, rates := range map[string][2]float64{
		"hunyuan-lite": {0, 0}, "hunyuan-standard": {4.5, 4.5}, "hunyuan-standard-256K": {15, 15},
		"hunyuan-pro": {30, 30}, "hunyuan-turbo": {2.4, 9.6}, "hunyuan-turbos": {.8, 2}, "hunyuan-t1": {1, 4}, "hunyuan-large-role": {2.4, 9.6},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), id)
			require.Contains(t, cfg.Description, "Retired on the Hunyuan platform and TokenHub on 2026-06-22")
			require.Contains(t, cfg.Description, "Historical tariff only")
			require.InDelta(t, rates[0], a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, rates[1], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
		})
	}
	preview := a.GetDefaultModelPricing()["hy3-preview"]
	require.Contains(t, preview.Description, "2026-08-31 at 00:00 Beijing")
	require.Contains(t, preview.Description, "historical preview pricing only")
	require.Contains(t, preview.Description, "separate Hy Token Plan")
	require.InDelta(t, 1.2, preview.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.Len(t, preview.Tiers, 2)
	current := a.GetDefaultModelPricing()["hy3"]
	require.InDelta(t, 1, current.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, .25, current.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 4, current.Ratio*current.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
	require.Empty(t, current.Tiers)
	require.NotContains(t, a.GetModelList(), "hy4-preview", "TokenHub catalogue presence is not legacy Tencent API support")
}
