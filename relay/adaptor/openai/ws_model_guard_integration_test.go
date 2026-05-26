package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// newGuardedProxyTestServer creates a gin-based test server hosting the
// ResponseAPIWebSocketHandler with a meta populated with the supplied
// origin/actual model pair. The optional `upstreamReceived` channel receives
// every text frame the proxy forwards to upstream, so tests can assert what
// the upstream actually sees (or does NOT see, in attack scenarios).
func newGuardedProxyTestServer(
	t *testing.T,
	upstreamURL string,
	originModel, actualModel string,
	usageOut chan<- *rmodel.Usage,
) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		meta := &rmeta.Meta{
			Mode:            relaymode.ResponseAPI,
			BaseURL:         upstreamURL,
			APIKey:          "test-key",
			OriginModelName: originModel,
			ActualModelName: actualModel,
		}
		_, usage := ResponseAPIWebSocketHandler(c, meta)
		if usageOut != nil {
			usageOut <- usage
		}
	})

	return httptest.NewServer(router)
}

// newEchoingUpstream creates a test WS server that records every text frame it
// receives into `received` and replies with a single `response.completed` event
// after each `response.create` so the proxy completes a request lifecycle.
func newEchoingUpstream(
	t *testing.T,
	received chan<- string,
) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		respIdx := 0
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			received <- string(msg)
			respIdx++
			completed := map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":     "resp_guarded_" + string(rune('0'+respIdx)),
					"status": "completed",
					"usage": map[string]any{
						"input_tokens":  3,
						"output_tokens": 2,
						"total_tokens":  5,
					},
				},
			}
			data, _ := json.Marshal(completed)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}))
}

// TestResponseAPIWS_ModelSwitchAttackRejected reproduces the security
// vulnerability scenario: a client opens the WS with a cheap model bound at
// handshake time, then sends a `response.create` event specifying a different
// (expensive) model. The proxy must reject the attempt and the upstream must
// NEVER receive the malicious frame.
func TestResponseAPIWS_ModelSwitchAttackRejected(t *testing.T) {
	received := make(chan string, 4)
	upstream := newEchoingUpstream(t, received)
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newGuardedProxyTestServer(t, upstream.URL, "gpt-4o-mini", "gpt-4o-mini", usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	// ATTACK: send response.create with a model the proxy did NOT bind.
	attack := `{"type":"response.create","model":"gpt-5","input":[]}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(attack)))

	// Read until the connection closes; capture the last text event.
	clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var lastTextEvent map[string]any
	var sawClose bool
	var closeCode int
	for {
		_, msg, err := clientConn.ReadMessage()
		if err != nil {
			if c, ok := err.(*websocket.CloseError); ok {
				sawClose = true
				closeCode = c.Code
			}
			break
		}
		var ev map[string]any
		if json.Unmarshal(msg, &ev) == nil {
			lastTextEvent = ev
		}
	}

	require.True(t, sawClose, "connection must be closed on model switch attempt")
	require.Equal(t, websocket.ClosePolicyViolation, closeCode,
		"close code must signal policy violation so SDKs surface the error")
	require.NotNil(t, lastTextEvent, "client must receive an error event before close")
	require.Equal(t, "error", lastTextEvent["type"])
	errBody, ok := lastTextEvent["error"].(map[string]any)
	require.True(t, ok, "error event must include error body")
	require.Equal(t, "model_switch_denied", errBody["code"])

	// CRITICAL: the upstream must NEVER have received the attack frame.
	select {
	case got := <-received:
		t.Fatalf("upstream received a frame on a denied model switch; got=%q", got)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing forwarded
	}

	// The handler returned without billable usage; the controller pre-consume
	// logic still applies, but the WS-aggregated usage must be empty.
	usage := <-usageCh
	require.NotNil(t, usage)
	require.Zero(t, usage.PromptTokens)
	require.Zero(t, usage.CompletionTokens)
}

// TestResponseAPIWS_MissingModelFieldInjected verifies that a `response.create`
// event without a `model` field has the handshake-bound model injected before
// being forwarded upstream. This keeps the upstream-recorded model aligned
// with the billed model.
func TestResponseAPIWS_MissingModelFieldInjected(t *testing.T) {
	received := make(chan string, 4)
	upstream := newEchoingUpstream(t, received)
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newGuardedProxyTestServer(t, upstream.URL, "gpt-4o-mini", "gpt-4o-mini", usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	require.NoError(t, clientConn.WriteMessage(
		websocket.TextMessage,
		[]byte(`{"type":"response.create","input":[]}`),
	))

	var forwarded string
	select {
	case forwarded = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the response.create event")
	}

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(forwarded), &parsed))
	require.Equal(t, "gpt-4o-mini", parsed["model"],
		"upstream must see the handshake-bound model injected into response.create")

	// Explicitly close the client so the proxy unwinds and we can read usage.
	_ = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	_ = clientConn.Close()

	usage := <-usageCh
	require.NotNil(t, usage)
}

// TestResponseAPIWS_OriginModelRewrittenToActual verifies that a client using
// the user-facing alias gets rewritten to the upstream-actual model, so
// channel-side model mapping still applies through the WS path.
func TestResponseAPIWS_OriginModelRewrittenToActual(t *testing.T) {
	received := make(chan string, 4)
	upstream := newEchoingUpstream(t, received)
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newGuardedProxyTestServer(t, upstream.URL, "gpt-mini-alias", "gpt-4o-mini", usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	require.NoError(t, clientConn.WriteMessage(
		websocket.TextMessage,
		[]byte(`{"type":"response.create","model":"gpt-mini-alias","input":[]}`),
	))

	var forwarded string
	select {
	case forwarded = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the response.create event")
	}

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(forwarded), &parsed))
	require.Equal(t, "gpt-4o-mini", parsed["model"],
		"origin alias must be rewritten to the upstream-actual model")

	_ = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	_ = clientConn.Close()

	<-usageCh
}

// TestResponseAPIWS_MatchingModelForwardedAsIs verifies the happy path: a
// client whose `response.create.model` exactly matches the handshake-bound
// actual model is forwarded without rewrite.
func TestResponseAPIWS_MatchingModelForwardedAsIs(t *testing.T) {
	received := make(chan string, 4)
	upstream := newEchoingUpstream(t, received)
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newGuardedProxyTestServer(t, upstream.URL, "gpt-4o-mini", "gpt-4o-mini", usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	payload := `{"type":"response.create","model":"gpt-4o-mini","input":[]}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(payload)))

	var forwarded string
	select {
	case forwarded = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the response.create event")
	}
	require.JSONEq(t, payload, forwarded)

	_ = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	_ = clientConn.Close()

	<-usageCh
}

// TestResponseAPIWS_LegacyEmptyBoundModelDoesNotEnforce documents the
// backward-compatibility branch: when the proxy was constructed without a
// resolved actual model (legacy no-`?model=` handshake), the guard does not
// enforce model pinning. This preserves the previous behavior while still
// fixing the most common attack vector.
func TestResponseAPIWS_LegacyEmptyBoundModelDoesNotEnforce(t *testing.T) {
	received := make(chan string, 4)
	upstream := newEchoingUpstream(t, received)
	defer upstream.Close()

	usageCh := make(chan *rmodel.Usage, 1)
	proxy := newGuardedProxyTestServer(t, upstream.URL, "", "", usageCh)
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/v1/responses"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	payload := `{"type":"response.create","model":"gpt-anything","input":[]}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(payload)))

	select {
	case got := <-received:
		require.JSONEq(t, payload, got,
			"legacy no-binding mode must forward the frame unchanged")
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the response.create event")
	}

	_ = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	_ = clientConn.Close()

	<-usageCh
}

// ---------------------------------------------------------------------------
// Realtime session.update guard integration tests
// ---------------------------------------------------------------------------

// newRealtimeProxyTestServer hosts the RealtimeHandler against the given
// upstream URL with `gpt-4o-realtime-preview` as the bound model. The
// `received` channel records every frame forwarded to upstream.
func newRealtimeProxyTestServer(
	t *testing.T,
	upstreamURL string,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		writer := &ginResponseWriter{w: w, ResponseWriter: c.Writer}
		c.Writer = writer

		meta := &rmeta.Meta{
			Mode:            relaymode.Realtime,
			BaseURL:         upstreamURL,
			APIKey:          "sk-test",
			ActualModelName: "gpt-4o-realtime-preview",
			OriginModelName: "gpt-4o-realtime-preview",
		}
		_, _ = RealtimeHandler(c, meta)
	}))
}

// newRecordingRealtimeUpstream creates a WS server that records all text
// frames received and stays open for the test duration.
func newRecordingRealtimeUpstream(t *testing.T, received chan<- string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.TextMessage {
				received <- string(msg)
			}
		}
	}))
}

// TestRealtimeWS_SessionUpdateModelDenied verifies that a client attempting
// to mutate `session.model` on a Realtime WS connection is rejected and the
// upstream never sees the frame.
func TestRealtimeWS_SessionUpdateModelDenied(t *testing.T) {
	received := make(chan string, 4)
	upstream := newRecordingRealtimeUpstream(t, received)
	defer upstream.Close()

	proxy := newRealtimeProxyTestServer(t, upstream.URL)
	defer proxy.Close()

	wsURL := strings.Replace(proxy.URL, "http://", "ws://", 1) +
		"/v1/realtime?model=gpt-4o-realtime-preview"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Skipf("WebSocket dial failed (expected in some test envs): %v", err)
	}
	defer clientConn.Close()

	attack := `{"type":"session.update","session":{"model":"gpt-4o-realtime-preview","instructions":"x"}}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(attack)))

	clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var lastTextEvent map[string]any
	var sawClose bool
	for {
		_, msg, err := clientConn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				sawClose = true
			}
			break
		}
		var ev map[string]any
		if json.Unmarshal(msg, &ev) == nil {
			lastTextEvent = ev
		}
	}

	require.True(t, sawClose, "connection must be closed on session.update model switch")
	require.NotNil(t, lastTextEvent)
	require.Equal(t, "error", lastTextEvent["type"])
	errBody, ok := lastTextEvent["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "model_switch_denied", errBody["code"])

	select {
	case got := <-received:
		t.Fatalf("upstream received a frame on a denied session.update; got=%q", got)
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

// TestRealtimeWS_SessionUpdateWithoutModelForwarded verifies legitimate
// session.update calls (without model field) are forwarded unchanged.
func TestRealtimeWS_SessionUpdateWithoutModelForwarded(t *testing.T) {
	received := make(chan string, 4)
	upstream := newRecordingRealtimeUpstream(t, received)
	defer upstream.Close()

	proxy := newRealtimeProxyTestServer(t, upstream.URL)
	defer proxy.Close()

	wsURL := strings.Replace(proxy.URL, "http://", "ws://", 1) +
		"/v1/realtime?model=gpt-4o-realtime-preview"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Skipf("WebSocket dial failed (expected in some test envs): %v", err)
	}
	defer clientConn.Close()

	payload := `{"type":"session.update","session":{"instructions":"be brief"}}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(payload)))

	select {
	case got := <-received:
		require.JSONEq(t, payload, got)
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the session.update event")
	}

	_ = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
}
