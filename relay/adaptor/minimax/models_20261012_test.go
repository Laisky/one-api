package minimax

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestMiniMaxOctoberCatalogue checks model discovery, published control metadata,
// and preserved Standard tariffs through the public adaptor using t.
func TestMiniMaxOctoberCatalogue(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for id, prices := range map[string][3]float64{
		"MiniMax-M3":              {2.1, 0.42, 8.4},
		"MiniMax-M2.7":            {2.1, 0.42, 8.4},
		"MiniMax-M2.7-highspeed":  {4.2, 0.42, 16.8},
		"MiniMax-M2.5":            {2.1, 0.21, 8.4},
		"MiniMax-M2.5-highspeed":  {4.2, 0.21, 16.8},
		"MiniMax-M2.1":            {2.1, 0.21, 8.4},
		"MiniMax-M2.1-highspeed":  {4.2, 0.21, 16.8},
		"MiniMax-M2":              {2.1, 0.21, 8.4},
	} {
		cfg, ok := a.GetDefaultModelPricing()[id]
		require.True(t, ok)
		require.Contains(t, a.GetModelList(), id)
		require.Contains(t, ModelList, id)
		require.InDelta(t, prices[0], cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, prices[1], cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, prices[2], cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
		require.Contains(t, cfg.SupportedSamplingParameters, "max_completion_tokens")
		require.NotContains(t, cfg.SupportedSamplingParameters, "presence_penalty")
		require.NotContains(t, cfg.SupportedSamplingParameters, "frequency_penalty")
		require.Empty(t, cfg.SupportedReasoningEfforts, "M3.1 controls must not leak into other models")
		if id != "MiniMax-M3" {
			require.InDelta(t, 2.625, cfg.CacheWrite5mRatio/ratio.MilliTokensRmb, 1e-12)
			require.Contains(t, cfg.Description, "Thinking cannot be disabled")
		}
	}
	for _, id := range []string{"MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed"} {
		cfg := a.GetDefaultModelPricing()[id]
		require.Contains(t, cfg.Description, "2026-10-19")
		require.Contains(t, cfg.Description, "2026-11-18")
		require.Contains(t, cfg.Description, "not ordinary API keys")
	}
	require.NotContains(t, a.GetDefaultModelPricing(), "MiniMax-M3.1-Flash-Preview", "subscription availability does not establish a pay-as-you-go tariff")
	m3 := a.GetDefaultModelPricing()["MiniMax-M3"]
	require.Len(t, m3.Tiers, 1)
	require.EqualValues(t, 512001, m3.Tiers[0].InputTokenThreshold)
	require.InDelta(t, 4.2, m3.Tiers[0].Ratio/ratio.MilliTokensRmb, 1e-12)
	require.Contains(t, m3.Description, "permanent")
}

// TestMiniMaxCurrentCatalogueOwnership verifies constructors do not share mutable
// capability slices across models or calls. It takes t and returns nothing.
func TestMiniMaxCurrentCatalogueOwnership(t *testing.T) {
	t.Parallel()
	first, second := minimaxCurrentModels(), minimaxCurrentModels()
	cfg := first["MiniMax-M3"]
	cfg.SupportedSamplingParameters[0] = "changed"
	cfg.Tiers[0].Ratio = -1
	require.Equal(t, "temperature", second["MiniMax-M3"].SupportedSamplingParameters[0])
	require.Positive(t, second["MiniMax-M3"].Tiers[0].Ratio)
	require.Equal(t, "temperature", first["MiniMax-M2.7"].SupportedSamplingParameters[0])
}
