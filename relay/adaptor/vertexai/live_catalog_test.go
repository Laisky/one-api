package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestVertexAdvertisesLiveModelsWithoutAssumingPrices separates discoverability
// from upstream entitlement and backend pricing. Parameters: t owns the test.
// Returns: none. The prior exclusion assertion is replaced by the requested
// administrator-owned catalog policy, not removed without a positive contract.
func TestVertexAdvertisesLiveModelsWithoutAssumingPrices(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	models := a.GetModelList()
	require.NotEmpty(t, models)
	for _, name := range []string{
		"gemini-3.8-live",
		"gemini-3.8-live-extended-thinking",
		"gemini-3.1-flash-live-preview",
		"gemini-3.5-live-translate-preview",
		"gemini-3.5-transcribe-live",
	} {
		require.True(t, adaptor.IsLiveOnlyGoogleModel(name))
		require.Contains(t, models, name)
		require.NotContains(t, a.GetDefaultModelPricing(), name, "Developer API prices are not verified Vertex defaults")
	}
	require.Contains(t, models, "gemini-live-2.5-flash-native-audio")
	require.Contains(t, models, "gemini-3.5-transcribe-live-preview")
	require.Contains(t, models, "gemini-3.8-flash")
	require.Contains(t, models, "gemini-3.5-flash")
}
