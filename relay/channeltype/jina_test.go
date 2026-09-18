package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
)

// TestJinaRegistration verifies stable IDs, native defaults and bridged chat endpoints.
func TestJinaRegistration(t *testing.T) {
	t.Parallel()
	require.Equal(t, 58, Zai)
	require.Equal(t, 59, Jina)
	require.Equal(t, apitype.Jina, ToAPIType(Jina))
	require.Equal(t, "jina", IdToName(Jina))
	require.Len(t, ChannelBaseURLConfigs, Dummy)
	require.Len(t, ChannelBaseURLs, Dummy)
	require.Equal(t, "https://api.jina.ai", GetChannelBaseURLConfig(Jina).URL)
	require.True(t, GetChannelBaseURLConfig(Jina).Editable)
	require.ElementsMatch(t, []Endpoint{EndpointChatCompletions, EndpointResponseAPI, EndpointClaudeMessages, EndpointEmbeddings, EndpointRerank}, DefaultEndpointsForChannelType(Jina))
	require.NotContains(t, DefaultEndpointsForChannelType(Jina), EndpointImagesGenerations)
	require.NotContains(t, DefaultEndpointsForChannelType(Jina), EndpointOCR)
}
