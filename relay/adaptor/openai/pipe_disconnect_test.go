package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestWebSocketBridgeReaderCloseStopsUpstream checks that a downstream reader
// closing before consuming the first event promptly closes the upstream socket.
func TestWebSocketBridgeReaderCloseStopsUpstream(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		close(closed)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := buildStreamingWebSocketHTTPResponse(c, conn, []byte(`{"type":"response.created"}`))
	require.NoError(t, resp.Body.Close())
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("upstream websocket remains open after downstream pipe closed")
	}
}

// TestClaudeBridgeStopsOnClosedReader checks failed downstream delivery is
// observed and stops the conversion producer when its pipe reader is closed.
func TestClaudeBridgeStopsOnClosedReader(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("pipe-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	gmw.SetLogger(c, lg)
	input := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	resp, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, input)
	require.Nil(t, failure)
	require.NoError(t, resp.Body.Close())
	require.Eventually(t, func() bool { return len(logs.FilterMessage("Claude stream downstream closed").All()) == 1 }, time.Second, 10*time.Millisecond)
}

// TestWebSocketBridgeClosedReaderOnDone verifies a terminal marker write
// failure is logged when the downstream closes after reading the terminal event.
func TestWebSocketBridgeClosedReaderOnDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("terminal-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	gmw.SetLogger(c, lg)
	msg := `{"type":"response.completed"}`
	resp := buildStreamingWebSocketHTTPResponse(c, conn, []byte(msg))
	first := make([]byte, len("data: "+msg+"\n\n"))
	_, err = io.ReadFull(resp.Body, first)
	require.NoError(t, err)
	require.Equal(t, "data: "+msg+"\n\n", string(first))
	require.NoError(t, resp.Body.Close())
	require.Eventually(t, func() bool {
		return len(logs.FilterMessage("websocket stream bridge: terminal marker delivery failed").All()) == 1
	}, time.Second, 10*time.Millisecond)
}
