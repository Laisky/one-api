package baichuan_test

import (
	"github.com/Laisky/one-api/relay/adaptor/baichuan"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberBaichuanMedicalLimits checks published native limits and stable
// token tariffs through public catalogue methods using t. It returns nothing.
func TestOctoberBaichuanMedicalLimits(t *testing.T) {
	t.Parallel()
	a := &baichuan.Adaptor{}
	for id, rates := range map[string][2]float64{"Baichuan-M2": {2, 20}, "Baichuan-M2-Plus": {10, 30}, "Baichuan-M3": {10, 30}, "Baichuan-M3-Plus": {5, 9}} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), id)
			require.EqualValues(t, 32000, cfg.MaxOutputTokens)
			require.EqualValues(t, 32000, cfg.ContextLength)
			require.Zero(t, cfg.MaxTokens)
			require.Empty(t, cfg.Tiers)
			require.Empty(t, cfg.TimeWindows)
			require.Equal(t, []string{"temperature", "top_p", "top_k", "max_tokens"}, cfg.SupportedSamplingParameters)
			require.InDelta(t, rates[0], a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, rates[1], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
		})
	}
	require.EqualValues(t, 2048, a.GetDefaultModelPricing()["Baichuan4-Turbo"].MaxOutputTokens)
	require.InDelta(t, .5, a.GetModelRatio("Baichuan-Text-Embedding")/ratio.MilliTokensRmb, 1e-12)
	require.Len(t, a.GetDefaultModelPricing()["Baichuan2-53B"].TimeWindows, 2)
}
