package openai

import (
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestReviewGitHubProxyURLDoesNotNormalize preserves proxy paths while the
// controller independently rejects paid channels before dispatch.
func TestReviewGitHubProxyURLDoesNotNormalize(t *testing.T) {
	for _, path := range []string{"/v1/oneapi/proxy/1/chat/completions", "/v1/oneapi/proxy/1/embeddings?x=1"} {
		a := &Adaptor{}
		got, err := a.GetRequestURL(&meta.Meta{Mode: relaymode.Proxy, ChannelType: channeltype.OpenAICompatible,
			BaseURL: "https://models.github.ai", RequestURLPath: path})
		require.NoError(t, err)
		require.Equal(t, "https://models.github.ai"+path, got)
	}
}
