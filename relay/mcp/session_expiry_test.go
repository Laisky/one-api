package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestClientReinitializesAfterSessionExpiry pins the Streamable HTTP session rule:
// "When a client receives HTTP 404 in response to a request containing an
// Mcp-Session-Id, it MUST start a new session by sending a new InitializeRequest
// without a session ID attached."
//
// The client used to surface the 404 as a tool-call failure. Worse, it stayed
// wedged: Initialize returns early while c.initialized is true, so every later
// request replayed the dead session id and 404'd forever. Any server that reaps
// idle sessions — the MCP TypeScript SDK's StreamableHTTPServerTransport with a
// sessionIdGenerator, behind an autoscaler — broke the client permanently.
func TestClientReinitializesAfterSessionExpiry(t *testing.T) {
	var (
		mu            sync.Mutex
		sessionSerial int
		liveSession   string
		initCount     int
		sessionSeen   []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var rpc struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &rpc)

		mu.Lock()
		defer mu.Unlock()

		if rpc.Method == "initialize" {
			require.Empty(t, r.Header.Get(mcpSessionIDHeader),
				"a re-initialize must not carry the dead session id")
			initCount++
			sessionSerial++
			liveSession = fmt.Sprintf("session-%d", sessionSerial)
			w.Header().Set(mcpSessionIDHeader, liveSession)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"mock","version":"1.0"}}}`, rpc.ID)
			return
		}
		if rpc.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		presented := r.Header.Get(mcpSessionIDHeader)
		sessionSeen = append(sessionSeen, presented)

		// The first session is reaped between initialize and the first tools/list.
		if sessionSerial == 1 {
			liveSession = "reaped"
		}
		if presented != liveSession {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Session not found"}}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"web_search","description":"d","inputSchema":{"type":"object"}}]}}`, rpc.ID)
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err, "an expired session must be recovered, not reported as a failure")
	require.Len(t, tools, 1)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, initCount, "the client must have handshaken again after the 404")
	require.Len(t, sessionSeen, 2)
	require.NotEqual(t, sessionSeen[0], sessionSeen[1], "the retry must present the new session id")
}
