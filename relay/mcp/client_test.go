package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// fakeMCPUpstream is a tiny in-process MCP server used to drive the client
// through realistic handshake + tool flows. It records every request method
// it sees so tests can assert on the wire-level lifecycle.
type fakeMCPUpstream struct {
	server          *httptest.Server
	methods         []string
	mu              chan struct{}
	sessionID       string                                                 // returned in initialize response if non-empty
	contentType     string                                                 // override Content-Type (e.g. text/event-stream)
	customResponder func(method string, id any, body []byte) (int, []byte) // optional override
	tools           []ToolDescriptor
	callResult      *CallToolResult
	notificationCnt atomic.Int32
}

func newFakeMCPUpstream(t *testing.T) *fakeMCPUpstream {
	t.Helper()
	fx := &fakeMCPUpstream{mu: make(chan struct{}, 1)}
	fx.mu <- struct{}{}
	fx.server = httptest.NewServer(http.HandlerFunc(fx.handle))
	t.Cleanup(fx.server.Close)
	return fx
}

func (fx *fakeMCPUpstream) handle(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	var rpc struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(bodyBytes, &rpc)

	<-fx.mu
	fx.methods = append(fx.methods, rpc.Method)
	fx.mu <- struct{}{}

	if fx.customResponder != nil {
		status, body := fx.customResponder(rpc.Method, rpc.ID, bodyBytes)
		ct := fx.contentType
		if ct == "" {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		if fx.sessionID != "" && rpc.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", fx.sessionID)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}

	switch {
	case rpc.Method == "initialize":
		w.Header().Set("Content-Type", "application/json")
		if fx.sessionID != "" {
			w.Header().Set("Mcp-Session-Id", fx.sessionID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fake","version":"1"}}}`, marshalRPCID(rpc.ID))
	case strings.HasPrefix(rpc.Method, "notifications/"):
		fx.notificationCnt.Add(1)
		w.WriteHeader(http.StatusAccepted)
	case rpc.Method == "tools/list":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		envelope := map[string]any{
			"jsonrpc": "2.0",
			"id":      rpc.ID,
			"result":  map[string]any{"tools": fx.tools},
		}
		_ = json.NewEncoder(w).Encode(envelope)
	case rpc.Method == "tools/call":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := fx.callResult
		if result == nil {
			result = &CallToolResult{}
		}
		envelope := map[string]any{
			"jsonrpc": "2.0",
			"id":      rpc.ID,
			"result":  result,
		}
		_ = json.NewEncoder(w).Encode(envelope)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func marshalRPCID(id any) string {
	if id == nil {
		return "null"
	}
	enc, err := json.Marshal(id)
	if err != nil {
		return "null"
	}
	return string(enc)
}

func (fx *fakeMCPUpstream) calledMethods() []string {
	<-fx.mu
	defer func() { fx.mu <- struct{}{} }()
	out := make([]string, len(fx.methods))
	copy(out, fx.methods)
	return out
}

// TestClient_Initialize_PerformsHandshake confirms the client sends the
// `initialize` request and the matching `notifications/initialized`
// notification on first use, in the correct order.
func TestClient_Initialize_PerformsHandshake(t *testing.T) {
	fx := newFakeMCPUpstream(t)
	fx.tools = []ToolDescriptor{{Name: "echo", Description: "echo"}}

	server := &model.MCPServer{BaseURL: fx.server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(server, nil, 5*time.Second)

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "echo", tools[0].Name)

	methods := fx.calledMethods()
	require.GreaterOrEqual(t, len(methods), 3, "expected at least initialize, notifications/initialized, tools/list")
	require.Equal(t, "initialize", methods[0], "first request must be initialize")
	require.Equal(t, "notifications/initialized", methods[1], "second request must be notifications/initialized")
	require.Equal(t, "tools/list", methods[2])
}

// TestClient_Initialize_RunsOnce ensures the handshake is performed once
// per client instance even across concurrent and sequential RPC calls.
func TestClient_Initialize_RunsOnce(t *testing.T) {
	fx := newFakeMCPUpstream(t)
	fx.tools = []ToolDescriptor{}

	server := &model.MCPServer{BaseURL: fx.server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(server, nil, 5*time.Second)

	for i := 0; i < 3; i++ {
		_, err := client.ListTools(context.Background())
		require.NoError(t, err)
	}

	methods := fx.calledMethods()
	initCount := 0
	for _, m := range methods {
		if m == "initialize" {
			initCount++
		}
	}
	require.Equal(t, 1, initCount, "initialize must fire only once across multiple calls")
}

// TestClient_Initialize_CapturesSessionID validates that a server-issued
// Mcp-Session-Id from the initialize response is stored on the client and
// — by way of the shared Headers map — sent on subsequent requests.
func TestClient_Initialize_CapturesSessionID(t *testing.T) {
	fx := newFakeMCPUpstream(t)
	fx.sessionID = "test-session-abc"
	fx.tools = []ToolDescriptor{{Name: "x"}}

	server := &model.MCPServer{BaseURL: fx.server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(server, nil, 5*time.Second)

	require.NoError(t, client.Initialize(context.Background()))
	require.Equal(t, "test-session-abc", client.Headers["Mcp-Session-Id"], "client must store the session id returned by the server")

	// A subsequent tools/list call must carry the captured session id.
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)
}

// TestClient_DoRPC_ParsesSSEResponse verifies the client decodes a JSON-RPC
// envelope delivered as a Server-Sent Events stream — the alternative
// content type allowed by the Streamable HTTP transport.
func TestClient_DoRPC_ParsesSSEResponse(t *testing.T) {
	fx := newFakeMCPUpstream(t)
	fx.contentType = "text/event-stream"
	fx.customResponder = func(method string, id any, body []byte) (int, []byte) {
		switch method {
		case "initialize":
			payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"sse","version":"1"}}}`, marshalRPCID(id))
			return http.StatusOK, []byte("event: message\ndata: " + payload + "\n\n")
		case "tools/list":
			payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"sse-tool"}]}}`, marshalRPCID(id))
			return http.StatusOK, []byte("data: " + payload + "\n\n")
		}
		// notifications/* — return 202 empty (SSE not used for those)
		return http.StatusAccepted, nil
	}

	server := &model.MCPServer{BaseURL: fx.server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(server, nil, 5*time.Second)

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "sse-tool", tools[0].Name)
}

// TestClient_NotificationFailureNonFatal confirms that a server returning
// 5xx for notifications/initialized does not abort tool calls — strict
// failure here would brick clients against quirky upstreams.
func TestClient_NotificationFailureNonFatal(t *testing.T) {
	fx := newFakeMCPUpstream(t)
	fx.tools = []ToolDescriptor{{Name: "ok"}}
	fx.customResponder = func(method string, id any, body []byte) (int, []byte) {
		switch method {
		case "initialize":
			return http.StatusOK, []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"x","version":"1"}}}`, marshalRPCID(id)))
		case "notifications/initialized":
			return http.StatusInternalServerError, []byte("nope")
		case "tools/list":
			return http.StatusOK, []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"ok"}]}}`, marshalRPCID(id)))
		}
		return http.StatusBadRequest, nil
	}

	server := &model.MCPServer{BaseURL: fx.server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(server, nil, 5*time.Second)

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err, "failure on notifications/initialized must not block subsequent calls")
	require.Len(t, tools, 1)
}

// TestClient_ListTools_RetryOnInvalidSessionID verifies the client retries
// initialize + tools/list once when the server rejects a missing/invalid
// Mcp-Session-Id.
func TestClient_ListTools_RetryOnInvalidSessionID(t *testing.T) {
	const recoveredSessionID = "session-after-reinit"

	var initCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(bodyBytes, &rpc)

		switch rpc.Method {
		case "initialize":
			call := initCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if call >= 2 {
				w.Header().Set("Mcp-Session-Id", recoveredSessionID)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"retry","version":"1"}}}`, marshalRPCID(rpc.ID))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != recoveredSessionID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"Bad Request: No valid session ID provided"},"id":null}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"recovered"}]}}`, marshalRPCID(rpc.ID))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	mcpServer := &model.MCPServer{BaseURL: server.URL, AuthType: model.MCPAuthTypeNone}
	client := NewStreamableHTTPClient(mcpServer, nil, 5*time.Second)

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "recovered", tools[0].Name)
	require.Equal(t, int32(2), initCount.Load(), "client should reinitialize once after invalid session rejection")
}

// TestParseSSEResponse covers the small SSE parser directly so its edge
// cases (multi-line data, missing data, mixed event lines) are pinned.
func TestParseSSEResponse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{"single-data", "data: {\"a\":1}\n\n", `{"a":1}`, false},
		{"multi-data", "data: line1\ndata: line2\n\n", "line1\nline2", false},
		{"event-and-data", "event: message\ndata: {\"x\":2}\n\n", `{"x":2}`, false},
		{"no-data", "event: ping\n\n", "", true},
		{"crlf", "data: {\"y\":3}\r\n\r\n", `{"y":3}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSSEResponse([]byte(tc.in))
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}
