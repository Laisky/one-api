package zhipu

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// zhipuHandshakeAPIKey is the fixture channel credential; the loopback provider
// asserts it arrives as a Bearer token and the error must never echo it.
const zhipuHandshakeAPIKey = "zhipu-handshake-id.zhipu-handshake-secret"

// zhipuHandshakePrivateBody is a provider body marker that must never reach the
// returned error.
const zhipuHandshakePrivateBody = "private-zhipu-handshake-detail"

// zhipuHandshakeMessage is the message a failed GLM-Realtime handshake has
// always produced; gorilla reports every non-101 answer as a bad handshake.
const zhipuHandshakeMessage = "upstream realtime connect failed: websocket: bad handshake"

// runZhipuRealtimeHandshake upgrades a real downstream client through the
// GLM-Realtime handler pointed at endpoint and returns what the handler
// returned, the close frame the already-upgraded client received and every
// entry the handler wrote to its observed request-scoped logger.
//
// Parameters:
//   - t: the running test.
//   - endpoint: the upstream WebSocket URL the handler dials.
//
// Returns:
//   - *model.ErrorWithStatusCode: the error the handler returned.
//   - *websocket.CloseError: the close frame the downstream client observed.
//   - *observer.ObservedLogs: the handler's request-scoped log entries.
func runZhipuRealtimeHandshake(t *testing.T, endpoint string) (*model.ErrorWithStatusCode, *websocket.CloseError, *observer.ObservedLogs) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	requestLogger, err := glog.New(glog.WithName("zhipu-realtime-handshake"), glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	results := make(chan *model.ErrorWithStatusCode, 1)
	engine := gin.New()
	engine.GET("/v1/realtime", func(c *gin.Context) {
		gmw.SetLogger(c, requestLogger)
		m := &meta.Meta{Mode: relaymode.Realtime, APIKey: zhipuHandshakeAPIKey, ActualModelName: "glm-realtime-flash"}
		// Report through a buffered channel, never through t: the handler runs
		// on the server goroutine of a hijacked connection.
		bizErr, _ := realtimeHandlerAt(c, m, endpoint)
		results <- bizErr
	})
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clientConn, response, err := dialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/realtime", nil)
	if response != nil && response.Body != nil {
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, err, "the downstream client is upgraded before the upstream dial")
	t.Cleanup(func() {
		if closeErr := clientConn.Close(); closeErr != nil {
			t.Logf("close downstream client: %v", closeErr)
		}
	})
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, readErr := clientConn.ReadMessage()
	var closeFrame *websocket.CloseError
	require.ErrorAs(t, readErr, &closeFrame)

	select {
	case bizErr := <-results:
		return bizErr, closeFrame, observed
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the GLM-Realtime handler did not return")
		return nil, nil, nil
	}
}

// requireZhipuHandshakeFailureReturnedNotLogged asserts the handler returned its
// handshake failure without also logging it at WARN or above (each error is
// either returned or logged, never both), and that no log entry carries the
// channel credential or the provider body.
//
// Parameters:
//   - t: the running test.
//   - logs: the handler's request-scoped log entries.
//
// Returns: none.
func requireZhipuHandshakeFailureReturnedNotLogged(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	require.NotNil(t, logs)
	for _, entry := range logs.All() {
		require.Less(t, entry.Level, zapcore.WarnLevel,
			"the returned handshake failure must not also be logged: %s", entry.Message)
		for _, field := range entry.Context {
			rendered := fmt.Sprint(field.String, field.Interface)
			require.NotContains(t, rendered, zhipuHandshakeAPIKey)
			require.NotContains(t, rendered, zhipuHandshakePrivateBody)
		}
	}
}

// requireZhipuTryAgainClose asserts the downstream client kept receiving the
// existing 1013 "upstream connect failed" close frame.
//
// Parameters:
//   - t: the running test.
//   - closeFrame: the close frame the client observed.
//
// Returns: none.
func requireZhipuTryAgainClose(t *testing.T, closeFrame *websocket.CloseError) {
	t.Helper()
	require.NotNil(t, closeFrame)
	require.Equal(t, websocket.CloseTryAgainLater, closeFrame.Code)
	require.Equal(t, "upstream connect failed", closeFrame.Text)
}

// requireZhipuHandshakeErrorHidesSecrets asserts that neither the provider body
// nor the channel credential reaches the returned error, its JSON or its cause.
//
// Parameters:
//   - t: the running test.
//   - failure: the error the handler returned.
//
// Returns: none.
func requireZhipuHandshakeErrorHidesSecrets(t *testing.T, failure *model.ErrorWithStatusCode) {
	t.Helper()
	encoded, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotNil(t, failure.RawError)
	for _, secret := range []string{zhipuHandshakePrivateBody, zhipuHandshakeAPIKey} {
		require.NotContains(t, string(encoded), secret)
		require.NotContains(t, failure.Message, secret)
		require.NotContains(t, fmt.Sprintf("%+v", failure.RawError), secret)
	}
}

// TestZhipuRealtimeHandshakeProviderStatusProvenance verifies that a GLM-Realtime
// provider that refuses the WebSocket handshake with an HTTP error status yields
// an error carrying that status and provider provenance, keeps the message,
// code, type and close frame, and never echoes the provider body.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestZhipuRealtimeHandshakeProviderStatusProvenance(t *testing.T) {
	statuses := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls, authorized atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") == "Bearer "+zhipuHandshakeAPIKey && r.URL.Query().Get("model") == "glm-realtime-flash" {
					authorized.Add(1)
				}
				http.Error(w, zhipuHandshakePrivateBody, status)
			}))
			t.Cleanup(upstream.Close)

			failure, closeFrame, logs := runZhipuRealtimeHandshake(t, "ws"+strings.TrimPrefix(upstream.URL, "http")+"/api/paas/v4/realtime?model=glm-realtime-flash")
			require.EqualValues(t, 1, calls.Load(), "a refused handshake must not be redialed")
			require.EqualValues(t, 1, authorized.Load(), "the channel key and bound model must reach the provider")
			requireZhipuTryAgainClose(t, closeFrame)
			require.NotNil(t, failure)
			require.Equal(t, status, failure.StatusCode)
			require.Equal(t, zhipuHandshakeMessage, failure.Message)
			require.Equal(t, "upstream_connect_failed", failure.Code)
			require.Equal(t, model.ErrorTypeUpstream, failure.Type)
			require.True(t, failure.FromUpstream, "a provider handshake status must be provider-marked")
			require.False(t, failure.IsOneAPIError(), "a provider handshake status must not be one-api's own")
			require.ErrorIs(t, failure.RawError, websocket.ErrBadHandshake)
			requireZhipuHandshakeErrorHidesSecrets(t, failure)
			requireZhipuHandshakeFailureReturnedNotLogged(t, logs)
		})
	}
}

// TestZhipuRealtimeHandshakeWithoutProviderStatusStaysLocal verifies that a dial
// with no provider response, or a provider answer outside 400..599, stays
// one-api's own 502 with the existing message shape and close frame.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestZhipuRealtimeHandshakeWithoutProviderStatusStaysLocal(t *testing.T) {
	t.Run("closed_port", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedURL := "ws://" + listener.Addr().String() + "/api/paas/v4/realtime"
		require.NoError(t, listener.Close())

		failure, closeFrame, logs := runZhipuRealtimeHandshake(t, closedURL)
		requireZhipuTryAgainClose(t, closeFrame)
		requireZhipuHandshakeLocal(t, failure)
		require.Contains(t, failure.Message, "upstream realtime connect failed: dial tcp")
		requireZhipuHandshakeErrorHidesSecrets(t, failure)
		requireZhipuHandshakeFailureReturnedNotLogged(t, logs)
	})
	t.Run("non_upgrade_200", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(zhipuHandshakePrivateBody))
		}))
		t.Cleanup(upstream.Close)

		failure, closeFrame, logs := runZhipuRealtimeHandshake(t, "ws"+strings.TrimPrefix(upstream.URL, "http"))
		requireZhipuTryAgainClose(t, closeFrame)
		requireZhipuHandshakeLocal(t, failure)
		require.Equal(t, zhipuHandshakeMessage, failure.Message)
		requireZhipuHandshakeErrorHidesSecrets(t, failure)
		requireZhipuHandshakeFailureReturnedNotLogged(t, logs)
	})
}

// requireZhipuHandshakeLocal asserts that failure is a local 502
// upstream_connect_failed one_api_error without provider provenance.
//
// Parameters:
//   - t: the running test.
//   - failure: the error the handler returned.
//
// Returns: none.
func requireZhipuHandshakeLocal(t *testing.T, failure *model.ErrorWithStatusCode) {
	t.Helper()
	require.NotNil(t, failure)
	require.Equal(t, http.StatusBadGateway, failure.StatusCode)
	require.Equal(t, "upstream_connect_failed", failure.Code)
	require.False(t, failure.FromUpstream)
	require.True(t, failure.IsOneAPIError(), "a failure without a provider status is one-api's own")
}
