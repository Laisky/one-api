package gemini

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// liveHandshakeExpectedJSON is the client-visible JSON the handshake failure has
// always produced; provenance must not change a byte of it.
const liveHandshakeExpectedJSON = `{"message":"Google Live upstream handshake failed (HTTP %d): invalid Gemini Live protocol","type":"one_api_error","param":"","code":"gemini_live_connect","status_code":%d}`

// runLiveHandshake drives LiveHandler against baseURL with a WebSocket upgrade
// request that never reaches the downstream upgrade, because the provider
// handshake fails first.
//
// Parameters:
//   - t: the running test.
//   - baseURL: the provider base URL configured on the channel.
//
// Returns:
//   - *model.ErrorWithStatusCode: the pre-upgrade error LiveHandler returned.
//   - *model.Usage: the usage LiveHandler returned.
func runLiveHandshake(t *testing.T, baseURL string) (*model.ErrorWithStatusCode, *model.Usage) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=friendly", nil)
	c.Request.Header.Set("Connection", "Upgrade")
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Sec-WebSocket-Version", "13")
	c.Request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	gmw.SetLogger(c, logger.Logger)
	SetLiveSpendGate(c, newTestLiveGate(-1))
	m := &meta.Meta{
		ChannelType: channeltype.Gemini, Mode: relaymode.Realtime,
		ActualModelName: "gemini-3.8-live", OriginModelName: "friendly",
		BaseURL: baseURL, APIKey: "provider-fixture",
	}
	return LiveHandler(c, m)
}

// TestGeminiLiveHandshakeProviderStatusProvenance verifies that a provider that
// refuses the Live WebSocket handshake with 429 or 5xx is attributed to the
// provider, keeps its status, and leaves the client-visible JSON unchanged.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestGeminiLiveHandshakeProviderStatusProvenance(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "private-provider-detail", status)
			}))
			t.Cleanup(upstream.Close)

			failure, usage := runLiveHandshake(t, upstream.URL)
			require.Nil(t, usage)
			require.NotNil(t, failure)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, status, failure.StatusCode)
			clientJSON, err := json.Marshal(failure)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(liveHandshakeExpectedJSON, status, status), string(clientJSON))
			require.True(t, failure.FromUpstream, "provider handshake status must be provider-marked")
			require.False(t, failure.IsOneAPIError(), "provider handshake status must not be one-api's own")
		})
	}
}

// TestGeminiLiveHandshakeWithoutProviderStatusStaysLocal verifies that a dial
// with no provider response, or a provider answer outside 400..599, stays
// one-api's own 502: the error carries no provider status.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestGeminiLiveHandshakeWithoutProviderStatusStaysLocal(t *testing.T) {
	t.Run("closed_port", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedURL := "http://" + listener.Addr().String()
		require.NoError(t, listener.Close())

		assertLiveHandshakeLocal(t, closedURL)
	})
	t.Run("non_upgrade_200", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(upstream.Close)

		assertLiveHandshakeLocal(t, upstream.URL)
	})
}

// assertLiveHandshakeLocal asserts that a failed handshake against baseURL is a
// local 502 one_api_error with unchanged client JSON.
//
// Parameters:
//   - t: the running test.
//   - baseURL: the provider base URL configured on the channel.
//
// Returns: none.
func assertLiveHandshakeLocal(t *testing.T, baseURL string) {
	t.Helper()
	failure, usage := runLiveHandshake(t, baseURL)
	require.Nil(t, usage)
	require.NotNil(t, failure)
	require.Equal(t, http.StatusBadGateway, failure.StatusCode)
	require.False(t, failure.FromUpstream)
	require.True(t, failure.IsOneAPIError())

	clientJSON, err := json.Marshal(failure)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf(liveHandshakeExpectedJSON, http.StatusBadGateway, http.StatusBadGateway), string(clientJSON))
}
