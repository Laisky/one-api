package mcp

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestConfiguredHeadersAreCanonicalized pins that a header configured in a
// non-canonical spelling cannot collide with one this client manages.
//
// The header map is replayed through http.Header.Set, which canonicalizes, so two
// entries differing only in case collapse onto one field with the winner decided
// by Go's randomized map iteration. The guards that strip Mcp-Session-Id and
// default Accept were exact-case, so "accept" or "mcp-session-id" configured on
// an MCP server slipped past them and corrupted a random subset of requests:
// an Accept without text/event-stream draws 406 from the MCP TypeScript SDK
// (and 406 is not in the fallback set), a stale session id draws 400/404.
func TestConfiguredHeadersAreCanonicalized(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured map[string]string
		assert     func(t *testing.T, snapshot map[string]string)
	}{
		{
			name:       "lowercase accept does not shadow the default",
			configured: map[string]string{"accept": "application/json"},
			assert: func(t *testing.T, snapshot map[string]string) {
				require.Len(t, keysMatching(snapshot, "Accept"), 1,
					"exactly one Accept entry may survive: %v", snapshot)
				require.Equal(t, "application/json", snapshot["Accept"])
			},
		},
		{
			name:       "lowercase session header is still stripped",
			configured: map[string]string{"mcp-session-id": "attacker-supplied"},
			assert: func(t *testing.T, snapshot map[string]string) {
				require.Empty(t, keysMatching(snapshot, mcpSessionIDHeader),
					"a configured session id must never survive: %v", snapshot)
			},
		},
		{
			name:       "lowercase protocol version header is still stripped",
			configured: map[string]string{"mcp-protocol-version": "1999-01-01"},
			assert: func(t *testing.T, snapshot map[string]string) {
				require.Empty(t, keysMatching(snapshot, mcpProtocolVersionHeader),
					"a configured protocol version must never survive: %v", snapshot)
			},
		},
		{
			name:       "mixed-case custom header keeps its value once",
			configured: map[string]string{"x-CUSTOM-header": "value"},
			assert: func(t *testing.T, snapshot map[string]string) {
				require.Len(t, keysMatching(snapshot, "X-Custom-Header"), 1)
				require.Equal(t, "value", snapshot["X-Custom-Header"])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewStreamableHTTPClient(
				&model.MCPServer{BaseURL: "https://mcp.example.com/mcp", Headers: tc.configured},
				nil, 5*time.Second)

			snapshot := client.headerSnapshot()
			for key := range snapshot {
				require.Equal(t, http.CanonicalHeaderKey(key), key,
					"every retained header key must already be canonical: %q", key)
			}
			tc.assert(t, snapshot)
		})
	}
}

// keysMatching returns the snapshot keys that canonicalize onto field.
//
// Parameters:
//   - snapshot: the client's header map.
//   - field: the header field to look for.
//
// Return values:
//   - []string: matching keys.
func keysMatching(snapshot map[string]string, field string) []string {
	target := http.CanonicalHeaderKey(field)
	var out []string
	for key := range snapshot {
		if http.CanonicalHeaderKey(key) == target {
			out = append(out, key)
		}
	}
	return out
}
