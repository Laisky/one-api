package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
)

// TestZaiRegistration verifies the stable channel ID and backend mappings for the
// Z.AI channel. Zai is currently the highest channel type, so it also pins the
// Dummy sentinel that relay/channeltype/url.go's init() asserts against.
func TestZaiRegistration(t *testing.T) {
	t.Parallel()

	require.Equal(t, 58, Zai)
	require.Equal(t, 59, Dummy)
	require.Equal(t, apitype.Zai, ToAPIType(Zai))
	require.Equal(t, "zai", IdToName(Zai))
	require.Equal(t, "zai", apitype.String(apitype.Zai))

	config := GetChannelBaseURLConfig(Zai)
	require.Equal(t, "https://api.z.ai", config.URL)
	require.False(t, config.Editable)
}

// TestZaiIsDistinctFromZhipu pins that the two brands stay independently routable:
// same company, same wire protocol, but separate channel types, API types, and
// therefore separate credentials and price tables.
func TestZaiIsDistinctFromZhipu(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, Zai, Zhipu)
	require.NotEqual(t, apitype.Zai, apitype.Zhipu)
	require.NotEqual(t, ToAPIType(Zai), ToAPIType(Zhipu))
	require.NotEqual(t, GetChannelBaseURLConfig(Zai).URL, GetChannelBaseURLConfig(Zhipu).URL)
}

// TestZaiDefaultEndpoints verifies the advertised relay capability matrix.
// Z.AI serves a smaller surface than open.bigmodel.cn: no embeddings, no rerank,
// no text-to-speech, and no realtime.
func TestZaiDefaultEndpoints(t *testing.T) {
	t.Parallel()

	endpoints := DefaultEndpointsForChannelType(Zai)

	for _, want := range []Endpoint{
		EndpointChatCompletions,
		EndpointImagesGenerations,
		EndpointVideos,
		EndpointAudioTranscription,
		EndpointResponseAPI,
		EndpointClaudeMessages,
		EndpointOCR,
	} {
		require.Contains(t, endpoints, want)
	}

	for _, unsupported := range []Endpoint{
		EndpointEmbeddings,
		EndpointRerank,
		EndpointAudioSpeech,
		EndpointAudioTranslation,
		EndpointRealtime,
	} {
		require.NotContains(t, endpoints, unsupported)
	}
}
