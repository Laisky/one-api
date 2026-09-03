package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// negotiatingServer answers initialize with a fixed protocolVersion and serves a
// single tool, so a test can assert which negotiated revisions are usable.
//
// Parameters:
//   - t: the running test.
//   - negotiated: the protocolVersion the server reports from initialize.
//
// Return values:
//   - *httptest.Server: the started server; the caller closes it.
func negotiatingServer(t *testing.T, negotiated string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var rpc struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &rpc)

		switch rpc.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":%q,"capabilities":{"tools":{}},"serverInfo":{"name":"mock","version":"1.0"}}}`, rpc.ID, negotiated)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"web_search","description":"d","inputSchema":{"type":"object"}}]}}`, rpc.ID)
		}
	}))
}

// TestClientAcceptsEveryStreamableHTTPRevision pins which negotiated protocol
// revisions the client can work with.
//
// 2025-03-26 is the revision that INTRODUCED the Streamable HTTP transport and is
// still what a large share of deployed servers pin. The client rejected it
// outright with "negotiated unsupported legacy version", even though everything it
// actually uses — initialize, notifications/initialized, tools/list, tools/call,
// Mcp-Session-Id — exists unchanged in that revision. Every tool on such a server
// was unreachable, and TestMCPServer / SyncServerTools failed the same way.
func TestClientAcceptsEveryStreamableHTTPRevision(t *testing.T) {
	for _, tc := range []struct {
		name       string
		negotiated string
		wantErr    bool
	}{
		{name: "current legacy", negotiated: LegacyProtocolVersion},
		{name: "previous legacy", negotiated: LegacyProtocolVersionFallback},
		{name: "revision that introduced streamable http", negotiated: LegacyProtocolVersionStreamableHTTPOrigin},
		{name: "a revision that does not exist", negotiated: "1999-01-01", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := negotiatingServer(t, tc.negotiated)
			defer server.Close()

			client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
			tools, err := client.ListTools(context.Background())
			if tc.wantErr {
				require.Error(t, err, "an unknown revision must still be refused")
				return
			}
			require.NoErrorf(t, err, "a server negotiating %s is fully usable and must not be refused", tc.negotiated)
			require.Len(t, tools, 1)
		})
	}
}
