package xai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestXAIGatewayVideoRoute checks the actual adaptor URL for both creation
// entrypoints, task polling, versioned bases and retained query parameters.
func TestXAIGatewayVideoRoute(t *testing.T) {
	for _, tc := range []struct{ base, path, want string }{
		{"https://api.x.ai", "/v1/videos", "https://api.x.ai/v1/videos/generations"},
		{"https://example.com/proxy/v1", "/v1/videos?trace=1", "https://example.com/proxy/v1/videos/generations?trace=1"},
		{"https://api.x.ai", "/v1/videos/generations", "https://api.x.ai/v1/videos/generations"},
		{"https://api.x.ai/v1", "/v1/videos/job-123?trace=1", "https://api.x.ai/v1/videos/job-123?trace=1"},
	} {
		t.Run(tc.path+tc.base, func(t *testing.T) {
			a := &Adaptor{}
			got, err := a.GetRequestURL(&meta.Meta{BaseURL: tc.base, RequestURLPath: tc.path, Mode: relaymode.Videos, ChannelType: channeltype.XAI})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
