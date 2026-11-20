package openai

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

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// realtimeHandshakeExpectedJSON is the client-visible JSON a failed upstream
// realtime handshake has always produced; only its status may follow the
// provider, and it never echoes the provider's response body.
const realtimeHandshakeExpectedJSON = `{"message":"upstream realtime connect failed: websocket: bad handshake","type":"one_api_error","param":"","code":"upstream_connect_failed","status_code":%d}`

// realtimeHandshakePrivateBody is a provider body marker that must never reach
// the client-visible error.
const realtimeHandshakePrivateBody = "private-provider-handshake-detail"

// runRealtimeHandshake upgrades a real downstream client through RealtimeHandler
// against baseURL and returns what the handler returned plus the close frame the
// already-upgraded client received.
//
// Parameters:
//   - t: the running test.
//   - baseURL: the provider base URL configured on the channel.
//
// Returns:
//   - *rmodel.ErrorWithStatusCode: the error RealtimeHandler returned.
//   - *websocket.CloseError: the close frame the downstream client observed.
func runRealtimeHandshake(t *testing.T, baseURL string) (*rmodel.ErrorWithStatusCode, *websocket.CloseError) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	results := make(chan *rmodel.ErrorWithStatusCode, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		c.Writer = &ginResponseWriter{w: w, ResponseWriter: c.Writer}
		m := &rmeta.Meta{Mode: relaymode.Realtime, BaseURL: baseURL, APIKey: "sk-handshake-fixture", ActualModelName: "gpt-realtime"}
		// Report through a buffered channel, never through t: the handler runs
		// on the server goroutine of a hijacked connection.
		bizErr, _ := RealtimeHandler(c, m)
		results <- bizErr
	}))
	t.Cleanup(proxy.Close)

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clientConn, _, err := dialer.Dial(strings.Replace(proxy.URL, "http://", "ws://", 1)+"/v1/realtime?model=gpt-realtime", nil)
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
		return bizErr, closeFrame
	case <-time.After(5 * time.Second):
		require.FailNow(t, "RealtimeHandler did not return")
		return nil, nil
	}
}

// requireRealtimeTryAgainClose asserts the downstream client kept receiving the
// existing 1013 "upstream connect failed" close frame.
//
// Parameters:
//   - t: the running test.
//   - closeFrame: the close frame the client observed.
//
// Returns: none.
func requireRealtimeTryAgainClose(t *testing.T, closeFrame *websocket.CloseError) {
	t.Helper()
	require.NotNil(t, closeFrame)
	require.Equal(t, websocket.CloseTryAgainLater, closeFrame.Code)
	require.Equal(t, "upstream connect failed", closeFrame.Text)
}

// TestRealtimeHandshakeProviderStatusProvenance verifies that a provider that
// refuses the realtime WebSocket handshake with an HTTP error status yields an
// error carrying that status and provider provenance, keeps the client JSON
// shape and close frame, and never echoes the provider body.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeHandshakeProviderStatusProvenance(t *testing.T) {
	statuses := []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, realtimeHandshakePrivateBody, status)
			}))
			t.Cleanup(upstream.Close)

			failure, closeFrame := runRealtimeHandshake(t, upstream.URL)
			require.EqualValues(t, 1, calls.Load())
			requireRealtimeTryAgainClose(t, closeFrame)
			require.NotNil(t, failure)
			require.Equal(t, status, failure.StatusCode)
			clientJSON, err := json.Marshal(failure)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(realtimeHandshakeExpectedJSON, status), string(clientJSON))
			require.NotContains(t, string(clientJSON), realtimeHandshakePrivateBody)
			require.True(t, failure.FromUpstream, "a provider handshake status must be provider-marked")
			require.False(t, failure.IsOneAPIError(), "a provider handshake status must not be one-api's own")
		})
	}
}

// TestRealtimeHandshakeWithoutProviderStatusStaysLocal verifies that a dial with
// no provider response, or a provider answer outside 400..599, stays one-api's
// own 502 with the existing close frame.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeHandshakeWithoutProviderStatusStaysLocal(t *testing.T) {
	t.Run("closed_port", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedURL := "http://" + listener.Addr().String()
		require.NoError(t, listener.Close())

		failure, closeFrame := runRealtimeHandshake(t, closedURL)
		requireRealtimeTryAgainClose(t, closeFrame)
		requireRealtimeHandshakeLocal(t, failure)
		require.Contains(t, failure.Message, "upstream realtime connect failed: dial tcp")
	})
	t.Run("non_upgrade_200", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(upstream.Close)

		failure, closeFrame := runRealtimeHandshake(t, upstream.URL)
		requireRealtimeTryAgainClose(t, closeFrame)
		requireRealtimeHandshakeLocal(t, failure)
		clientJSON, err := json.Marshal(failure)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf(realtimeHandshakeExpectedJSON, http.StatusBadGateway), string(clientJSON))
	})
}

// requireRealtimeHandshakeLocal asserts that failure is a local 502
// upstream_connect_failed one_api_error without provider provenance.
//
// Parameters:
//   - t: the running test.
//   - failure: the error RealtimeHandler returned.
//
// Returns: none.
func requireRealtimeHandshakeLocal(t *testing.T, failure *rmodel.ErrorWithStatusCode) {
	t.Helper()
	require.NotNil(t, failure)
	require.Equal(t, http.StatusBadGateway, failure.StatusCode)
	require.Equal(t, "upstream_connect_failed", failure.Code)
	require.False(t, failure.FromUpstream)
	require.True(t, failure.IsOneAPIError())
}
