package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// capturedHandshake records what the upstream saw on the WebSocket handshake.
type capturedHandshake struct {
	mu            sync.Mutex
	authorization string
	subprotocols  string
	beta          string
	seen          bool
}

// snapshot returns the recorded handshake fields without racing the server goroutine.
func (c *capturedHandshake) snapshot() (string, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authorization, c.subprotocols, c.seen
}

// betaHeader returns the recorded OpenAI-Beta value.
func (c *capturedHandshake) betaHeader() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.beta
}

// newRecordingUpstream accepts one WebSocket upgrade and records its request headers.
func newRecordingUpstream(t *testing.T, got *capturedHandshake) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.mu.Lock()
		got.authorization = r.Header.Get("Authorization")
		got.subprotocols = r.Header.Get("Sec-WebSocket-Protocol")
		got.beta = r.Header.Get("OpenAI-Beta")
		got.seen = true
		got.mu.Unlock()

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
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
}

// TestRealtimeHandler_DoesNotForwardClientAPIKeySubprotocol pins the upstream
// handshake credentials. Browsers cannot set headers on a WebSocket, so they
// authenticate to this gateway with an "openai-insecure-api-key.*" subprotocol.
// Relaying that subprotocol upstream alongside the channel's own Authorization
// header makes OpenAI refuse the handshake with "You must only send one of
// protocol api key and Authorization header", which killed every browser call.
func TestRealtimeHandler_DoesNotForwardClientAPIKeySubprotocol(t *testing.T) {
	t.Parallel()

	got := &capturedHandshake{}
	upstream := newRecordingUpstream(t, got)
	defer upstream.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		c.Writer = &ginResponseWriter{w: w, ResponseWriter: c.Writer}

		meta := &rmeta.Meta{
			Mode:            relaymode.Realtime,
			BaseURL:         upstream.URL,
			APIKey:          "sk-channel-key",
			ActualModelName: "gpt-realtime",
		}
		if bizErr, _ := RealtimeHandler(c, meta); bizErr != nil {
			return
		}
	}))
	defer proxy.Close()

	wsURL := strings.Replace(proxy.URL, "http://", "ws://", 1) + "/v1/realtime?model=gpt-realtime"
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"realtime", "openai-insecure-api-key.sk-browser-token"},
	}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "the proxy must accept a browser subprotocol handshake")
	defer clientConn.Close()

	// Drive one frame so the upstream handshake has certainly completed.
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = clientConn.ReadMessage()
	require.NoError(t, err)

	authorization, subprotocols, seen := got.snapshot()
	require.True(t, seen, "upstream must have been dialed")
	require.Equal(t, "Bearer sk-channel-key", authorization,
		"the channel key is the only credential the upstream may receive")
	require.NotContains(t, subprotocols, "openai-insecure-api-key",
		"the browser's gateway token must never reach the upstream handshake")
	require.NotContains(t, subprotocols, "sk-browser-token",
		"the browser's gateway token must never reach the upstream handshake")
	require.Contains(t, subprotocols, "realtime",
		"the non-auth subprotocol must still be negotiated upstream")

	// The client must also never see its own key echoed back.
	require.NotContains(t, clientConn.Subprotocol(), "openai-insecure-api-key")

	// The Realtime beta interface was removed upstream on 2026-05-12. Defaulting
	// the beta header selects the retired beta schema, which rejects GA session
	// fields such as `session.type`, so a client that did not ask for it must not
	// get it.
	require.Empty(t, got.betaHeader(),
		"the proxy must not invent an OpenAI-Beta header for a GA client")

	_ = clientConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = rmodel.Usage{}
}

// TestRealtimeHandler_ForwardsGASessionUpdateUnchanged drives the exact first frame
// the GPTChat browser client sends and asserts the relay hands it to OpenAI intact.
// This covers the whole browser-to-upstream path in one place: browser subprotocol
// auth, credential hygiene on the upstream handshake, absence of the retired beta
// header, and the session-model guard accepting a GA payload.
func TestRealtimeHandler_ForwardsGASessionUpdateUnchanged(t *testing.T) {
	t.Parallel()

	// Byte-for-byte the payload from createRealtimeSessionUpdate in
	// web/src/pages/gptchat/audio/realtime-session.ts, with the model field absent.
	const gaSessionUpdate = `{"type":"session.update","session":{` +
		`"type":"realtime","output_modalities":["audio"],"instructions":"Be brief.",` +
		`"audio":{"input":{"format":{"type":"audio/pcm","rate":24000},` +
		`"turn_detection":{"type":"semantic_vad","create_response":true,"interrupt_response":true}},` +
		`"output":{"format":{"type":"audio/pcm","rate":24000},"voice":"marin"}},` +
		`"tools":[{"type":"function","name":"end_call","description":"End this voice call.",` +
		`"parameters":{"type":"object","properties":{"reason":{"type":"string"}},` +
		`"required":["reason"],"additionalProperties":false}}],"tool_choice":"auto"}}`

	got := &capturedHandshake{}
	relayed := make(chan string, 4)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.mu.Lock()
		got.authorization = r.Header.Get("Authorization")
		got.subprotocols = r.Header.Get("Sec-WebSocket-Protocol")
		got.beta = r.Header.Get("OpenAI-Beta")
		got.seen = true
		got.mu.Unlock()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case relayed <- string(msg):
			default:
			}
			// Acknowledge the way the GA API does.
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.updated"}`))
		}
	}))
	defer upstream.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		c.Writer = &ginResponseWriter{w: w, ResponseWriter: c.Writer}

		meta := &rmeta.Meta{
			Mode:            relaymode.Realtime,
			BaseURL:         upstream.URL,
			APIKey:          "sk-channel-key",
			ActualModelName: "gpt-realtime-2.1",
		}
		if bizErr, _ := RealtimeHandler(c, meta); bizErr != nil {
			return
		}
	}))
	defer proxy.Close()

	wsURL := strings.Replace(proxy.URL, "http://", "ws://", 1) +
		"/v1/realtime?model=gpt-realtime-2.1"
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{"realtime", "openai-insecure-api-key.sk-browser-token"},
	}
	clientConn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer clientConn.Close()

	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(gaSessionUpdate)))

	// The guard must not reject a GA payload, so the acknowledgement comes back.
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, ack, err := clientConn.ReadMessage()
	require.NoError(t, err, "a GA session.update must not be rejected by the model guard")
	require.JSONEq(t, `{"type":"session.updated"}`, string(ack))

	select {
	case frame := <-relayed:
		require.JSONEq(t, gaSessionUpdate, frame,
			"the relay must forward the GA session.update byte-equivalent")
	case <-time.After(5 * time.Second):
		t.Fatal("upstream never received the session.update frame")
	}

	authorization, subprotocols, seen := got.snapshot()
	require.True(t, seen)
	require.Equal(t, "Bearer sk-channel-key", authorization)
	require.NotContains(t, subprotocols, "openai-insecure-api-key")
	require.Empty(t, got.betaHeader(),
		"OpenAI answers a beta header on the GA path with "+
			"\"The Realtime Beta API is no longer supported\"")

	_ = clientConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}
