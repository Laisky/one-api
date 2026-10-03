package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiSpeechEndpointDiscovery adds speech without confusing it with transcription or Live.
func TestGeminiSpeechEndpointDiscovery(t *testing.T) {
	t.Parallel()
	for _, channel := range []int{Gemini, GeminiOpenAICompatible, VertextAI} {
		endpoints := DefaultEndpointsForChannelType(channel)
		require.Contains(t, endpoints, EndpointAudioSpeech)
		require.Contains(t, endpoints, EndpointRealtime)
		require.NotContains(t, endpoints, EndpointAudioTranscription)
		require.NotContains(t, endpoints, EndpointAudioTranslation)
	}
}
