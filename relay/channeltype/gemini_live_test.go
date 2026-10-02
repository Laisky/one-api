package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiLiveEndpointAdmission exercises the endpoint policy used by channel
// distribution. Parameters: t is the test handle. Returns: none.
func TestGeminiLiveEndpointAdmission(t *testing.T) {
	t.Parallel()
	for _, channel := range []int{Gemini, GeminiOpenAICompatible} {
		require.Contains(t, DefaultEndpointsForChannelType(channel), EndpointRealtime)
		require.Contains(t, DefaultEndpointsForChannelType(channel), EndpointChatCompletions)
		require.Contains(t, DefaultEndpointsForChannelType(channel), EndpointEmbeddings)
	}
	// Enabling the Gemini Live transport must not enable unrelated providers.
	require.NotContains(t, DefaultEndpointsForChannelType(Zai), EndpointRealtime)
}
