package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestVertexDoesNotAdvertiseLiveOnlyModels pins the advertised Vertex catalog.
// Parameters: t is the test handle. Returns: none. Vertex has no Live transport
// here, and Google serves these IDs only over bidiGenerateContent, so a channel
// filled from this list would publish models that answer 503 on /v1/realtime
// (the channel type does not expose that endpoint) and 400 on every REST route.
func TestVertexDoesNotAdvertiseLiveOnlyModels(t *testing.T) {
	t.Parallel()
	models := (&Adaptor{}).GetModelList()
	require.NotEmpty(t, models)
	for _, name := range []string{
		"gemini-3.8-live",
		"gemini-3.8-live-extended-thinking",
		"gemini-3.1-flash-live-preview",
		"gemini-3.5-live-translate-preview",
		"gemini-3.5-transcribe-live",
	} {
		require.True(t, adaptor.IsLiveOnlyGoogleModel(name))
		require.NotContains(t, models, name)
	}
	// Ordinary Gemini models on Vertex are unaffected.
	require.Contains(t, models, "gemini-3.8-flash")
	require.Contains(t, models, "gemini-3.5-flash")
}
