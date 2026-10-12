package xunfeiv2_test

import (
	"github.com/Laisky/one-api/relay/adaptor/xunfeiv2"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberSparkTariffsAndTransport verifies explicit free pricing, paid-price
// isolation and transport-specific capabilities using t. It returns nothing.
func TestOctoberSparkTariffsAndTransport(t *testing.T) {
	t.Parallel()
	a := &xunfeiv2.Adaptor{}
	for id, input := range map[string]float64{
		"lite":        0,
		"generalv3":   2,
		"pro-128k":    5,
		"generalv3.5": 2,
		"max-32k":     5,
		"4.0Ultra":    0.8,
		"spark-x":     2,
	} {
		cfg, exists := a.GetDefaultModelPricing()[id]
		require.True(t, exists)
		require.Contains(t, a.GetModelList(), id)
		require.InDelta(t, input, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, input, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
		require.EqualValues(t, 1, cfg.CompletionRatio)
		require.Zero(t, cfg.CachedInputRatio)
		require.Empty(t, cfg.TimeWindows)
	}
	require.Zero(t, a.GetModelRatio("lite"), "free is a known tariff, never an unknown-model fallback")

	for _, id := range []string{"lite", "generalv3", "pro-128k", "generalv3.5", "max-32k", "4.0Ultra"} {
		cfg := a.GetDefaultModelPricing()[id]
		require.Contains(t, cfg.SupportedFeatures, "json_mode")
		require.Contains(t, cfg.SupportedSamplingParameters, "presence_penalty")
		require.Contains(t, cfg.SupportedSamplingParameters, "frequency_penalty")
		require.NotContains(t, cfg.SupportedSamplingParameters, "stop")
	}
	for _, id := range []string{"lite", "generalv3", "pro-128k"} {
		require.NotContains(t, a.GetDefaultModelPricing()[id].SupportedFeatures, "tools")
	}
	require.EqualValues(t, 32768, a.GetDefaultModelPricing()["pro-128k"].MaxOutputTokens, "HTTP and WS ceilings are distinct")

}
