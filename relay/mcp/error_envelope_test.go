package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestClientSurfacesErrorEnvelopeWithNullID pins that a JSON-RPC error response
// carrying `"id": null` is reported as the protocol error it is.
//
// JSON-RPC 2.0 section 5 requires the id to be Null when the server could not read
// the request id, and the MCP error-response shape makes id optional for exactly
// that case. The correlation check used to reject those envelopes with
// "mcp response id mismatch", which threw away the real error code and — because
// the replacement was a plain error rather than a *ProtocolError —
// IsModernFallbackCandidate then refused to fall back to the legacy handshake.
//
// This is reachable against one-api itself: its own respondMCPError writes
// HTTP 200 with `"id": null` for a parse/invalid-request failure.
func TestClientSurfacesErrorEnvelopeWithNullID(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null id", body: `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}}`},
		{name: "absent id", body: `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
			_, err := client.ListTools(context.Background())
			require.Error(t, err)

			var protocolErr *ProtocolError
			require.ErrorAs(t, err, &protocolErr,
				"an error envelope must surface as *ProtocolError so the fallback logic can read its code; got %v", err)
			require.NotEqual(t, 0, protocolErr.Code)
			require.NotContains(t, err.Error(), "id mismatch")
		})
	}
}

// TestClientStillRejectsMismatchedSuccessID keeps the correlation check honest:
// a *result* envelope carrying someone else's id must still be rejected, because
// accepting it would hand one request's answer to another.
func TestClientStillRejectsMismatchedSuccessID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"someone-elses-request","result":{"tools":[]}}`))
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	_, err := client.ListTools(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "id mismatch")
}
