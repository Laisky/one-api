package zhipu

import (
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestProtocolAuditVideoLookup verifies both creation aliases and exact native
// polling paths across configured API prefixes, without paid API calls.
func TestProtocolAuditVideoLookup(t *testing.T) {
	for _, base := range []string{"https://example.com", "https://example.com/api", "https://example.com/api/paas/v4"} {
		for _, tc := range []struct{ path, want string }{
			{"", "/videos/generations"},
			{"/v1/videos", "/videos/generations"},
			{"/v1/videos/generations", "/videos/generations"},
			{"/v1/videos/job_123?trace=1", "/async-result/job_123?trace=1"},
		} {
			a := &Adaptor{}
			got, err := a.GetRequestURL(&meta.Meta{BaseURL: base, Mode: relaymode.Videos, RequestURLPath: tc.path})
			require.NoError(t, err)
			require.Equal(t, "https://example.com/api/paas/v4"+tc.want, got)
		}
	}
	for _, path := range []string{"/v1/videos/job/content", "/v1/videos/../secret", "/v1/videos/%2fsecret", "/v1/videos/"} {
		_, err := videoRequestURL(&meta.Meta{BaseURL: "https://example.com", RequestURLPath: path})
		require.Error(t, err)
	}
}
