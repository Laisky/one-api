package xunfei_test

import (
	"github.com/Laisky/one-api/relay/adaptor/xunfei"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberSparkTariffsAndTransport verifies explicit free pricing, paid-price
// isolation and transport-specific capabilities using t. It returns nothing.
func TestOctoberSparkTariffsAndTransport(t *testing.T) {
	t.Parallel()
	a := &xunfei.Adaptor{}
	for id, input := range map[string]float64{
		"Spark-Lite":      0,
		"Spark-Pro":       1.26,
		"Spark-Pro-128K":  5,
		"Spark-Max":       2.1,
		"Spark-Max-32K":   2.1,
		"Spark-4.0-Ultra": 0.8,
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
	require.Zero(t, a.GetModelRatio("Spark-Lite"), "free is a known tariff, never an unknown-model fallback")

	for _, id := range a.GetModelList() {
		cfg := a.GetDefaultModelPricing()[id]
		require.NotContains(t, cfg.SupportedFeatures, "tools", "HTTP function calling is not WebSocket support")
		require.NotContains(t, cfg.SupportedFeatures, "json_mode")
		require.Equal(t, []string{"temperature", "max_tokens"}, cfg.SupportedSamplingParameters)
	}
	require.EqualValues(t, 131072, a.GetDefaultModelPricing()["Spark-Pro-128K"].MaxOutputTokens)

}
