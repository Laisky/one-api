package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	geminiOpenaiCompatible "github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestVertexCarbonUsesVertexOwnedTariff pins Gemini 4 Carbon on Vertex to the
// Vertex-owned gross Standard tariff ($4 input, $20 output, $0.20 cached per
// 1M tokens) without the Developer API's time-of-day windows. The Vertex-owned
// catalog is the single source; the Developer API entry must not leak through.
func TestVertexCarbonUsesVertexOwnedTariff(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	cfg, ok := a.GetDefaultModelPricing()[geminiOpenaiCompatible.Gemini4CarbonModel]
	require.True(t, ok)
	require.InDelta(t, 4*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 5, cfg.CompletionRatio, 1e-12)
	require.InDelta(t, 0.20*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
	require.Empty(t, cfg.TimeWindows, "Vertex has no Developer API time-of-day pricing")
	require.Equal(t, VertexGoogleModelConfigs()[geminiOpenaiCompatible.Gemini4CarbonModel].Ratio, cfg.Ratio)
}
