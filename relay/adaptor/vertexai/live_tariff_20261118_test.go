package vertexai_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestVertexLiveModelsNeverBorrowGlobalTariff verifies that Vertex Live-only
// Google models resolve no tariff below the channel override. The Vertex
// adaptor ships no Live price (operators configure Live rates; see
// RequiresExplicitRealtimePricing), so before this policy the resolver fell
// through to the global layer and returned the Gemini Developer API price for
// gemini-3.8-live. That is another host's tariff and omits Vertex's separate
// $1.00/1M image/video input rate (Vertex pricing page
// https://cloud.google.com/vertex-ai/generative-ai/pricing, fetched
// 2026-11-18T14:34Z: Gemini 3.8 Live API, Non-global, text $0.75 in / $4.50 out,
// audio $3.00 in / $12.00 out, video/image $1.00 in). It accepts t and returns
// no values.
func TestVertexLiveModelsNeverBorrowGlobalTariff(t *testing.T) {
	relay.InitializeGlobalPricing()
	at := time.Date(2026, 11, 18, 4, 0, 0, 0, time.UTC)
	vertex := &vertexai.Adaptor{}
	for _, name := range []string{"gemini-3.8-live", "gemini-live-2.5-flash-native-audio"} {
		require.True(t, adaptor.IsLiveOnlyGoogleModel(name), name)
		require.Contains(t, vertex.GetModelList(), name)
		require.NotContains(t, vertex.GetDefaultModelPricing(), name)
		require.True(t, adaptor.RequiresOperatorTariff(vertex, name), name)
		_, found := pricing.ResolveModelConfig(name, nil, vertex, at)
		require.False(t, found, "%s must not resolve another host's price on Vertex", name)
	}
	// The Developer API keeps its own documented Live tariff.
	_, developer := pricing.GetGlobalModelConfig("gemini-3.8-live")
	require.True(t, developer)
	// Ordinary Vertex Gemini models keep their bundled defaults.
	require.False(t, adaptor.RequiresOperatorTariff(vertex, "gemini-3.8-flash"))
	_, found := pricing.ResolveModelConfig("gemini-3.8-flash", nil, vertex, at)
	require.True(t, found)
}
