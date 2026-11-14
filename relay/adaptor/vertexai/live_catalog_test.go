package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestVertexAdvertisesLiveModelsWithoutAssumingPrices verifies corroborated
// Live suggestions without importing Developer API realtime prices.
// Parameters: t owns the test assertions. Returns: none.
func TestVertexAdvertisesLiveModelsWithoutAssumingPrices(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	models := a.GetModelList()
	require.NotEmpty(t, models)
	for _, name := range []string{
		"gemini-3.8-live",
	} {
		require.True(t, adaptor.IsLiveOnlyGoogleModel(name))
		require.Contains(t, models, name)
		require.NotContains(t, a.GetDefaultModelPricing(), name, "Developer API prices are not verified Vertex defaults")
	}
	require.Contains(t, models, "gemini-live-2.5-flash-native-audio")
	for _, id := range []string{
		"gemini-3.8-live-extended-thinking", "gemini-3.1-flash-live-preview",
		"gemini-3.5-live-translate-preview", "gemini-3.5-transcribe-live",
		"gemini-3.5-transcribe-live-preview",
	} {
		require.NotContains(t, models, id, "specialized or uncorroborated Live modes remain unqualified")
	}
	require.Contains(t, models, "gemini-3.8-flash")
	require.Contains(t, models, "gemini-3.5-flash")
}
