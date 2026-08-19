package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
)

// TestDeepInfraRegistration verifies the stable channel ID and backend mappings.
func TestDeepInfraRegistration(t *testing.T) {
	t.Parallel()

	require.Equal(t, 57, DeepInfra)
	require.Equal(t, 58, Dummy)
	require.Equal(t, apitype.DeepInfra, ToAPIType(DeepInfra))
	require.Equal(t, "deepinfra", IdToName(DeepInfra))
	require.Equal(t, "deepinfra", apitype.String(apitype.DeepInfra))

	config := GetChannelBaseURLConfig(DeepInfra)
	require.Equal(t, "https://api.deepinfra.com", config.URL)
	require.False(t, config.Editable)
}

// TestDeepInfraDefaultEndpoints verifies the advertised relay capability matrix.
func TestDeepInfraDefaultEndpoints(t *testing.T) {
	t.Parallel()

	endpoints := DefaultEndpointsForChannelType(DeepInfra)
	require.Contains(t, endpoints, EndpointChatCompletions)
	require.Contains(t, endpoints, EndpointCompletions)
	require.Contains(t, endpoints, EndpointEmbeddings)
	require.Contains(t, endpoints, EndpointImagesGenerations)
	require.Contains(t, endpoints, EndpointImagesEdits)
	require.Contains(t, endpoints, EndpointAudioSpeech)
	require.Contains(t, endpoints, EndpointAudioTranscription)
	require.Contains(t, endpoints, EndpointAudioTranslation)
	require.Contains(t, endpoints, EndpointRerank)
	require.Contains(t, endpoints, EndpointResponseAPI)
	require.Contains(t, endpoints, EndpointClaudeMessages)
	require.NotContains(t, endpoints, EndpointModerations)
	require.NotContains(t, endpoints, EndpointRealtime)
	require.NotContains(t, endpoints, EndpointVideos)
}
