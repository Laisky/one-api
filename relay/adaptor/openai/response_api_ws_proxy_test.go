package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// ---------------------------------------------------------------------------
// Unit tests for usage extraction helpers
// ---------------------------------------------------------------------------

func TestExtractResponseAPIUsage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":10,"output_tokens":20,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":5}}}}`)

	responseID, usage, ok := extractResponseAPIUsage(payload)
	require.True(t, ok)
	require.Equal(t, "resp_123", responseID)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
	require.NotNil(t, usage.CompletionTokensDetails)
	require.Equal(t, 5, usage.CompletionTokensDetails.ReasoningTokens)
}

func TestExtractResponseAPIUsageMissingUsage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.created","response":{"id":"resp_123"}}`)

	responseID, usage, ok := extractResponseAPIUsage(payload)
	require.False(t, ok)
	require.Equal(t, "", responseID)
	require.Nil(t, usage)
}

func TestAccumulateResponseAPIUsageDeduplicate(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":7,"output_tokens":9,"total_tokens":16}}}`)
	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	accumulateResponseAPIUsage(payload, usage, counted)
	accumulateResponseAPIUsage(payload, usage, counted)

	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 9, usage.CompletionTokens)
	require.Equal(t, 16, usage.TotalTokens)
	require.Len(t, counted, 1)
}

// TestAccumulateResponseAPIUsageMultipleResponses verifies usage from different
// response IDs are accumulated correctly.
func TestAccumulateResponseAPIUsageMultipleResponses(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	payload1 := []byte(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)
	payload2 := []byte(`{"type":"response.completed","response":{"id":"resp_2","usage":{"input_tokens":5,"output_tokens":15,"total_tokens":20}}}`)

	accumulateResponseAPIUsage(payload1, usage, counted)
	accumulateResponseAPIUsage(payload2, usage, counted)

	require.Equal(t, 15, usage.PromptTokens)
	require.Equal(t, 35, usage.CompletionTokens)
	require.Equal(t, 50, usage.TotalTokens)
	require.Len(t, counted, 2)
}

// TestExtractResponseAPIUsageIgnoresInvalidJSON verifies malformed JSON is ignored.
func TestExtractResponseAPIUsageIgnoresInvalidJSON(t *testing.T) {
	t.Parallel()

	_, _, ok := extractResponseAPIUsage([]byte(`{invalid json`))
	require.False(t, ok)
}

// TestExtractResponseAPIUsageIgnoresEmptyResponseID verifies events without
// response ID are not counted.
func TestExtractResponseAPIUsageIgnoresEmptyResponseID(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":20}}}`)
	_, _, ok := extractResponseAPIUsage(payload)
	require.False(t, ok)
}

// TestAccumulateResponseAPIUsageNilUsage verifies nil usage pointer doesn't panic.
func TestAccumulateResponseAPIUsageNilUsage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":10}}}`)
	counted := map[string]struct{}{}

	// Should not panic
	accumulateResponseAPIUsage(payload, nil, counted)
}

// ---------------------------------------------------------------------------
// Unit test for resolveResponseAPIWebSocketUpstreamURL
// ---------------------------------------------------------------------------

func TestResolveResponseAPIWebSocketUpstreamURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		wantURL string
		wantErr bool
	}{
		{
			name:    "https base",
			baseURL: "https://api.openai.com",
			wantURL: "wss://api.openai.com/v1/responses",
		},
		{
			name:    "http base",
			baseURL: "http://localhost:8080",
			wantURL: "ws://localhost:8080/v1/responses",
		},
		{
			name:    "wss base",
			baseURL: "wss://api.openai.com",
			wantURL: "wss://api.openai.com/v1/responses",
		},
		{
			name:    "empty base defaults to wss",
			baseURL: "",
			wantURL: "wss://api.openai.com/v1/responses",
		},
		{
			name:    "unsupported scheme",
			baseURL: "ftp://api.openai.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

			meta := &rmeta.Meta{BaseURL: tt.baseURL}
			got, err := resolveResponseAPIWebSocketUpstreamURL(ctx, meta)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantURL, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests: full proxy lifecycle using gin router
// ---------------------------------------------------------------------------

// newProxyTestServer creates an httptest.Server that runs a gin router with
// the ResponseAPIWebSocketHandler. The usageOut channel receives the usage
// after the handler returns.
func newProxyTestServer(t *testing.T, upstreamURL string, usageOut chan<- *rmodel.Usage) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		meta := &rmeta.Meta{
			Mode:    relaymode.ResponseAPI,
			BaseURL: upstreamURL,
			APIKey:  "test-key",
		}
		_, usage := ResponseAPIWebSocketHandler(c, meta)
		if usageOut != nil {
			usageOut <- usage
		}
	})

	return httptest.NewServer(router)
}

// TestProxyBidirectionalFrameForwarding verifies that client frames are forwarded
// to upstream and upstream frames are forwarded to client.
func TestProxyBidirectionalFrameForwarding(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	var mu sync.Mutex
	var receivedFromClient []byte

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		mu.Lock()
		receivedFromClient = msg
		mu.Unlock()

		delta := `{"type":"response.output_text.delta","delta":"world"}`
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(delta)))

		completed := `{"type":"response.completed","response":{"id":"resp_proxy_1","model":"gpt-4o","status":"completed","usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}}`
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(completed)))

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newProxyTestServer(t, upstream.URL, usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	err = clientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o"}`))
	require.NoError(t, err)

	var events []map[string]any
	for {
		_, msg, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
		var ev map[string]any
		require.NoError(t, json.Unmarshal(msg, &ev))
		events = append(events, ev)
	}

	mu.Lock()
	require.NotEmpty(t, receivedFromClient)
	mu.Unlock()

	require.GreaterOrEqual(t, len(events), 2)
	require.Equal(t, "response.output_text.delta", events[0]["type"])
	require.Equal(t, "response.completed", events[1]["type"])

	usage := <-usageCh
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 8, usage.TotalTokens)
}

// TestProxyInvalidMode verifies that a non-ResponseAPI mode is rejected.
func TestProxyInvalidMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	meta := &rmeta.Meta{Mode: 0}
	bizErr, usage := ResponseAPIWebSocketHandler(ctx, meta)
	require.NotNil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
}

// TestProxyNilMeta verifies that nil meta is rejected.
func TestProxyNilMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	bizErr, usage := ResponseAPIWebSocketHandler(ctx, nil)
	require.NotNil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
}

// TestProxyUpstreamHeadersIncludeOpenAIBeta verifies the proxy sends the required
// OpenAI-Beta header to upstream.
func TestProxyUpstreamHeadersIncludeOpenAIBeta(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	var mu sync.Mutex
	var capturedHeaders http.Header

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		completed := `{"type":"response.completed","response":{"id":"resp_h1","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`
		_ = conn.WriteMessage(websocket.TextMessage, []byte(completed))
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, upstream.URL, nil)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	for {
		_, _, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, capturedHeaders)
	require.Equal(t, "Bearer test-key", capturedHeaders.Get("Authorization"))
	require.Equal(t, "responses-api=v1", capturedHeaders.Get("OpenAI-Beta"))
}

// TestProxyClientDisconnectClosesUpstream verifies that when the client
// disconnects, the upstream connection is also closed.
func TestProxyClientDisconnectClosesUpstream(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			conn.Close()
			close(upstreamClosed)
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, upstream.URL, nil)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Client disconnects
	clientConn.Close()

	select {
	case <-upstreamClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream connection was not closed after client disconnect")
	}
}

// TestProxyCloseFramePropagationUpstreamToClient verifies that when upstream
// sends a close frame, it is forwarded to the client.
func TestProxyCloseFramePropagationUpstreamToClient(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, upstream.URL, nil)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = clientConn.ReadMessage()
	require.Error(t, err)

	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseGoingAway, closeErr.Code)
}

// TestProxyUsageAccumulationAcrossMultipleResponses verifies that in a long-lived
// WebSocket proxy session with multiple response.create round-trips, usage is
// accumulated correctly across all responses.
func TestProxyUsageAccumulationAcrossMultipleResponses(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for i := range 2 {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}

			respID := "resp_multi_" + string(rune('1'+i))
			completed := map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":     respID,
					"status": "completed",
					"usage": map[string]any{
						"input_tokens":  10 * (i + 1),
						"output_tokens": 5 * (i + 1),
						"total_tokens":  15 * (i + 1),
					},
				},
			}
			data, _ := json.Marshal(completed)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newProxyTestServer(t, upstream.URL, usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	for range 2 {
		err = clientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`))
		require.NoError(t, err)

		_, _, err = clientConn.ReadMessage()
		require.NoError(t, err)
	}

	for {
		_, _, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
	}

	usage := <-usageCh
	require.NotNil(t, usage)
	require.Equal(t, 30, usage.PromptTokens)
	require.Equal(t, 15, usage.CompletionTokens)
	require.Equal(t, 45, usage.TotalTokens)
}

// TestProxySecWebSocketProtocolForwarded verifies the Sec-WebSocket-Protocol
// header from the client is forwarded to upstream.
func TestProxySecWebSocketProtocolForwarded(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return true },
		Subprotocols: []string{"openai-beta.responses-api-v1"},
	}

	var mu sync.Mutex
	var capturedProtocol string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedProtocol = r.Header.Get("Sec-WebSocket-Protocol")
		mu.Unlock()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, upstream.URL, nil)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	dialer := websocket.Dialer{
		Subprotocols: []string{"openai-beta.responses-api-v1"},
	}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	for {
		_, _, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
	}

	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, capturedProtocol, "openai-beta.responses-api-v1")
}
