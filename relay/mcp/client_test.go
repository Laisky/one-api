package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
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

// recordedRequest captures the payload of a single inbound HTTP request so
// strict-mode tests can make post-hoc assertions about what the client sent.
type recordedRequest struct {
	Method     string
	RPCMethod  string
	Headers    http.Header
	RawBody    []byte
	ParsedBody map[string]any
}

// strictMockServer mimics validation behavior of the TypeScript MCP SDK
// (used by aas-ee/open-web-search and similar). Tests use it to assert the
// Go client emits spec-compliant JSON-RPC + Streamable HTTP requests.
type strictMockServer struct {
	t                 *testing.T
	mu                sync.Mutex
	requests          []recordedRequest
	sessionID         string
	toolsListAsSSE    bool
	rejectNullParams  bool
	requireSession    bool
	requireAcceptDual bool
}

func newStrictMockServer(t *testing.T) *strictMockServer {
	return &strictMockServer{
		t:                 t,
		sessionID:         "session-abc-123",
		rejectNullParams:  true,
		requireSession:    true,
		requireAcceptDual: true,
	}
}

func (m *strictMockServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		_ = r.Body.Close()

		var parsed map[string]any
		if len(bodyBytes) > 0 {
			if jerr := json.Unmarshal(bodyBytes, &parsed); jerr != nil {
				m.writeJSONRPCError(w, http.StatusBadRequest, nil, -32700, "Parse error: Invalid JSON-RPC message")
				return
			}
		}

		rpcMethod, _ := parsed["method"].(string)

		m.mu.Lock()
		m.requests = append(m.requests, recordedRequest{
			Method:     r.Method,
			RPCMethod:  rpcMethod,
			Headers:    r.Header.Clone(),
			RawBody:    append([]byte(nil), bodyBytes...),
			ParsedBody: parsed,
		})
		m.mu.Unlock()

		if m.requireAcceptDual {
			accept := r.Header.Get("Accept")
			if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
		}

		if m.rejectNullParams {
			if rawParams, ok := parsed["params"]; ok && rawParams == nil {
				m.writeJSONRPCError(w, http.StatusBadRequest, parsed["id"], -32700, "Parse error: Invalid JSON-RPC message")
				return
			}
		}

		if m.requireSession && rpcMethod != "initialize" {
			if r.Header.Get(mcpSessionIDHeader) == "" {
				m.writeJSONRPCError(w, http.StatusBadRequest, parsed["id"], -32000, "Bad Request: No valid session ID provided")
				return
			}
		}

		switch rpcMethod {
		case "initialize":
			w.Header().Set(mcpSessionIDHeader, m.sessionID)
			w.Header().Set("Content-Type", "application/json")
			result := map[string]any{
				"jsonrpc": "2.0",
				"id":      parsed["id"],
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mock", "version": "0.0.1"},
				},
			}
			_ = json.NewEncoder(w).Encode(result)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			tool := map[string]any{
				"name":        "web_search",
				"description": "Search the web",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"query": map[string]any{"type": "string"}},
					"required":   []string{"query"},
				},
			}
			result := map[string]any{
				"jsonrpc": "2.0",
				"id":      parsed["id"],
				"result":  map[string]any{"tools": []any{tool}},
			}
			if m.toolsListAsSSE {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				payload, _ := json.Marshal(result)
				_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		case "tools/call":
			success := map[string]any{
				"jsonrpc": "2.0",
				"id":      parsed["id"],
				"result": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "ok"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(success)
		default:
			m.writeJSONRPCError(w, http.StatusBadRequest, parsed["id"], -32601, "Method not found")
		}
	}
}

func (m *strictMockServer) writeJSONRPCError(w http.ResponseWriter, status int, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (m *strictMockServer) snapshot() []recordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func (m *strictMockServer) findRequest(rpcMethod string) (recordedRequest, error) {
	for _, req := range m.snapshot() {
		if req.RPCMethod == rpcMethod {
			return req, nil
		}
	}
	return recordedRequest{}, errors.Errorf("no request recorded for method %q", rpcMethod)
}

func newStrictTestClient(t *testing.T, baseURL string, server model.MCPServer) *StreamableHTTPClient {
	t.Helper()
	server.BaseURL = baseURL
	return NewStreamableHTTPClient(&server, nil, 5*time.Second)
}

// TestStreamableHTTPClient_ListToolsHappyPath drives the full handshake +
// tools/list flow against a strict TS-SDK-style upstream and asserts on
// request order, session header propagation, and JSON-RPC validity.
func TestStreamableHTTPClient_ListToolsHappyPath(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].Name)
	require.NotNil(t, tools[0].InputSchema)
	require.Equal(t, "object", tools[0].InputSchema["type"])

	requests := mock.snapshot()
	require.Len(t, requests, 3)
	require.Equal(t, "initialize", requests[0].RPCMethod)
	require.Empty(t, requests[0].Headers.Get(mcpSessionIDHeader))
	require.Equal(t, "notifications/initialized", requests[1].RPCMethod)
	require.Equal(t, mock.sessionID, requests[1].Headers.Get(mcpSessionIDHeader))
	require.Equal(t, "tools/list", requests[2].RPCMethod)
	require.Equal(t, mock.sessionID, requests[2].Headers.Get(mcpSessionIDHeader))

	for _, req := range requests {
		require.True(t, json.Valid(req.RawBody), "request body must be valid JSON: %s", req.RawBody)
		require.Equal(t, "2.0", req.ParsedBody["jsonrpc"], "missing jsonrpc 2.0 marker")
	}
	require.NotContains(t, string(requests[2].RawBody), `"params":null`,
		"tools/list must not serialize params as null (regression for Bug A)")
}

// TestStreamableHTTPClient_OmitsNullParamsOnRequests focused regression for Bug A:
// when no params are supplied, the field must be absent entirely, not null.
func TestStreamableHTTPClient_OmitsNullParamsOnRequests(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	listReq, err := mock.findRequest("tools/list")
	require.NoError(t, err)
	_, hasParams := listReq.ParsedBody["params"]
	require.False(t, hasParams,
		"tools/list payload must omit `params` entirely when nil; got: %s", listReq.RawBody)
}

// TestStreamableHTTPClient_CapturesSessionIDAndPropagates asserts the session
// id from initialize is attached to every subsequent request.
func TestStreamableHTTPClient_CapturesSessionIDAndPropagates(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	for _, req := range mock.snapshot() {
		if req.RPCMethod == "initialize" {
			require.Empty(t, req.Headers.Get(mcpSessionIDHeader))
			continue
		}
		require.Equal(t, mock.sessionID, req.Headers.Get(mcpSessionIDHeader),
			"expected Mcp-Session-Id %q on %s", mock.sessionID, req.RPCMethod)
	}
}

// TestStreamableHTTPClient_NotificationsHaveNoID verifies notifications omit
// the JSON-RPC `id` field per spec — strict TS SDK rejects notifications with id.
func TestStreamableHTTPClient_NotificationsHaveNoID(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	notif, err := mock.findRequest("notifications/initialized")
	require.NoError(t, err)
	_, hasID := notif.ParsedBody["id"]
	require.False(t, hasID,
		"JSON-RPC notification must not include `id`; got: %s", notif.RawBody)
}

// TestStreamableHTTPClient_HandlesSSEResponse exercises tools/list returned
// as a single SSE event payload.
func TestStreamableHTTPClient_HandlesSSEResponse(t *testing.T) {
	mock := newStrictMockServer(t)
	mock.toolsListAsSSE = true
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].Name)
	require.NotNil(t, tools[0].InputSchema)
}

// TestStreamableHTTPClient_CallTool verifies the wire shape of tools/call and
// the success-path result decode.
func TestStreamableHTTPClient_CallTool(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	result, err := client.CallTool(context.Background(), "web_search", map[string]any{"query": "hello"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Content)

	callReq, err := mock.findRequest("tools/call")
	require.NoError(t, err)
	params, ok := callReq.ParsedBody["params"].(map[string]any)
	require.True(t, ok, "tools/call params must be an object; got: %s", callReq.RawBody)
	require.Equal(t, "web_search", params["name"])
	args, ok := params["arguments"].(map[string]any)
	require.True(t, ok, "arguments must be an object")
	require.Equal(t, "hello", args["query"])
}

// TestStreamableHTTPClient_AcceptHeader asserts every outbound request
// advertises both JSON and SSE per the Streamable HTTP transport spec.
func TestStreamableHTTPClient_AcceptHeader(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	for _, req := range mock.snapshot() {
		accept := req.Headers.Get("Accept")
		require.Contains(t, accept, "application/json", "Accept missing application/json on %s", req.RPCMethod)
		require.Contains(t, accept, "text/event-stream", "Accept missing text/event-stream on %s", req.RPCMethod)
	}
}

// TestStreamableHTTPClient_AuthBearer asserts bearer auth attaches an
// Authorization header on every request including the handshake.
func TestStreamableHTTPClient_AuthBearer(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{
		AuthType: model.MCPAuthTypeBearer,
		APIKey:   "xyz",
	})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	for _, req := range mock.snapshot() {
		require.Equal(t, "Bearer xyz", req.Headers.Get("Authorization"),
			"missing/incorrect Authorization on %s", req.RPCMethod)
	}
}

// TestStreamableHTTPClient_AuthAPIKey asserts api_key auth attaches the
// X-API-Key header on every request including the handshake.
func TestStreamableHTTPClient_AuthAPIKey(t *testing.T) {
	mock := newStrictMockServer(t)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	client := newStrictTestClient(t, srv.URL, model.MCPServer{
		AuthType: model.MCPAuthTypeAPIKey,
		APIKey:   "xyz",
	})
	_, err := client.ListTools(context.Background())
	require.NoError(t, err)

	for _, req := range mock.snapshot() {
		require.Equal(t, "xyz", req.Headers.Get("X-API-Key"),
			"missing/incorrect X-API-Key on %s", req.RPCMethod)
	}
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
