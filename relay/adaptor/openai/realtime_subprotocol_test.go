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
	seen          bool
}

// snapshot returns the recorded handshake fields without racing the server goroutine.
func (c *capturedHandshake) snapshot() (string, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authorization, c.subprotocols, c.seen
}

// newRecordingUpstream accepts one WebSocket upgrade and records its request headers.
func newRecordingUpstream(t *testing.T, got *capturedHandshake) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.mu.Lock()
		got.authorization = r.Header.Get("Authorization")
		got.subprotocols = r.Header.Get("Sec-WebSocket-Protocol")
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

	_ = clientConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = rmodel.Usage{}
}
