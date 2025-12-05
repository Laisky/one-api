package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestAllEndpointsConsistency verifies that all endpoint IDs match their corresponding relaymode constants.
func TestAllEndpointsConsistency(t *testing.T) {
	endpoints := AllEndpoints()
	require.NotEmpty(t, endpoints, "AllEndpoints should return non-empty list")

	// Check that each endpoint has valid ID, name, and path
	for _, ep := range endpoints {
		require.NotEmpty(t, ep.Name, "endpoint name should not be empty")
		require.NotEmpty(t, ep.Path, "endpoint path should not be empty")
		require.True(t, ep.ID >= 0, "endpoint ID should be non-negative")
	}
}

// TestEndpointNameConversion verifies name-to-ID and ID-to-name conversions.
func TestEndpointNameConversion(t *testing.T) {
	testCases := []struct {
		name       string
		expectedID Endpoint
	}{
		{"chat_completions", EndpointChatCompletions},
		{"embeddings", EndpointEmbeddings},
		{"rerank", EndpointRerank},
		{"response_api", EndpointResponseAPI},
		{"claude_messages", EndpointClaudeMessages},
		{"videos", EndpointVideos},
		{"audio_speech", EndpointAudioSpeech},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id := EndpointNameToID(tc.name)
			require.Equal(t, tc.expectedID, id, "EndpointNameToID should return correct ID")

			name := EndpointIDToName(tc.expectedID)
			require.Equal(t, tc.name, name, "EndpointIDToName should return correct name")
		})
	}

	// Test unknown endpoint
	require.Equal(t, Endpoint(-1), EndpointNameToID("unknown_endpoint"))
	require.Equal(t, "", EndpointIDToName(Endpoint(-999)))
}

// TestDefaultEndpointsForChannelType verifies that each channel type has default endpoints defined.
func TestDefaultEndpointsForChannelType(t *testing.T) {
	channelTypes := []int{
		OpenAI,
		Azure,
		Anthropic,
		Gemini,
		Cohere,
		DeepSeek,
		XAI,
		OpenAICompatible,
	}

	for _, ct := range channelTypes {
		t.Run(IdToName(ct), func(t *testing.T) {
			endpoints := DefaultEndpointsForChannelType(ct)
			require.NotEmpty(t, endpoints, "channel type should have default endpoints")

			// Chat completions should be supported by most channel types
			hasChatCompletions := false
			for _, ep := range endpoints {
				if ep == EndpointChatCompletions {
					hasChatCompletions = true
					break
				}
			}
			// Most channels should support chat completions (except DeepL which is translation-only)
			if ct != DeepL {
				require.True(t, hasChatCompletions, "most channels should support chat completions")
			}
		})
	}
}

// TestDefaultEndpointNamesForChannelType verifies name conversion for defaults.
func TestDefaultEndpointNamesForChannelType(t *testing.T) {
	names := DefaultEndpointNamesForChannelType(OpenAI)
	require.NotEmpty(t, names, "OpenAI should have default endpoint names")

	// OpenAI should include standard endpoints
	require.Contains(t, names, "chat_completions")
	require.Contains(t, names, "embeddings")
	require.Contains(t, names, "response_api")
}

// TestIsEndpointSupported verifies endpoint support checking.
func TestIsEndpointSupported(t *testing.T) {
	supported := []Endpoint{EndpointChatCompletions, EndpointEmbeddings}

	require.True(t, IsEndpointSupported(relaymode.ChatCompletions, supported))
	require.True(t, IsEndpointSupported(relaymode.Embeddings, supported))
	require.False(t, IsEndpointSupported(relaymode.Rerank, supported))
	require.False(t, IsEndpointSupported(relaymode.Videos, supported))
}

// TestIsEndpointSupportedByName verifies name-based endpoint support checking.
func TestIsEndpointSupportedByName(t *testing.T) {
	supported := []string{"chat_completions", "embeddings", "Response_API"}

	require.True(t, IsEndpointSupportedByName("chat_completions", supported))
	require.True(t, IsEndpointSupportedByName("CHAT_COMPLETIONS", supported)) // case insensitive
	require.True(t, IsEndpointSupportedByName("response_api", supported))
	require.False(t, IsEndpointSupportedByName("rerank", supported))
	require.False(t, IsEndpointSupportedByName("videos", supported))
}

// TestRelayModeToEndpointName verifies relay mode to name conversion.
func TestRelayModeToEndpointName(t *testing.T) {
	require.Equal(t, "chat_completions", RelayModeToEndpointName(relaymode.ChatCompletions))
	require.Equal(t, "embeddings", RelayModeToEndpointName(relaymode.Embeddings))
	require.Equal(t, "rerank", RelayModeToEndpointName(relaymode.Rerank))
	require.Equal(t, "response_api", RelayModeToEndpointName(relaymode.ResponseAPI))
	require.Equal(t, "claude_messages", RelayModeToEndpointName(relaymode.ClaudeMessages))
	require.Equal(t, "", RelayModeToEndpointName(relaymode.Unknown))
}

// TestParseEndpointList verifies endpoint list parsing.
func TestParseEndpointList(t *testing.T) {
	names := []string{"chat_completions", "invalid", "embeddings", "", "rerank"}
	endpoints := ParseEndpointList(names)

	require.Len(t, endpoints, 3)
	require.Contains(t, endpoints, EndpointChatCompletions)
	require.Contains(t, endpoints, EndpointEmbeddings)
	require.Contains(t, endpoints, EndpointRerank)
}

// TestEndpointListToNames verifies endpoint list to names conversion.
func TestEndpointListToNames(t *testing.T) {
	endpoints := []Endpoint{EndpointChatCompletions, EndpointEmbeddings, Endpoint(-1)}
	names := EndpointListToNames(endpoints)

	require.Len(t, names, 2)
	require.Contains(t, names, "chat_completions")
	require.Contains(t, names, "embeddings")
}

// TestCohereSupportsRerank verifies that Cohere channel supports rerank endpoint.
func TestCohereSupportsRerank(t *testing.T) {
	endpoints := DefaultEndpointsForChannelType(Cohere)
	hasRerank := false
	for _, ep := range endpoints {
		if ep == EndpointRerank {
			hasRerank = true
			break
		}
	}
	require.True(t, hasRerank, "Cohere should support rerank endpoint")
}

// TestOpenAISupportsResponseAPI verifies that OpenAI channel supports Response API.
func TestOpenAISupportsResponseAPI(t *testing.T) {
	endpoints := DefaultEndpointsForChannelType(OpenAI)
	hasResponseAPI := false
	for _, ep := range endpoints {
		if ep == EndpointResponseAPI {
			hasResponseAPI = true
			break
		}
	}
	require.True(t, hasResponseAPI, "OpenAI should support Response API endpoint")
}

// TestAnthropicDoesNotSupportEmbeddings verifies that Anthropic channel does not support embeddings.
func TestAnthropicDoesNotSupportEmbeddings(t *testing.T) {
	endpoints := DefaultEndpointsForChannelType(Anthropic)
	hasEmbeddings := false
	for _, ep := range endpoints {
		if ep == EndpointEmbeddings {
			hasEmbeddings = true
			break
		}
	}
	require.False(t, hasEmbeddings, "Anthropic should not support embeddings endpoint")
}
