package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiLiveNeverForwardsCallerQuery pins the go/request-forgery review of
// the Gemini Live dial: the provider endpoint is derived only from the
// administrator's channel configuration, so caller query values — including a
// competing `key` credential, a different model or host-looking keys — never
// reach the provider handshake (the fixture fails the test if the provider sees
// any query or a caller credential). Parameters: t is the test handle.
// Returns: none.
func TestGeminiLiveNeverForwardsCallerQuery(t *testing.T) {
	const name = "gemini-3.8-live"
	endpoint, results := liveFixture(t, channeltype.Gemini, name, func(up *websocket.Conn) error {
		if err := acknowledgeLiveFixture(up, name); err != nil {
			return err
		}
		liveClose(up, websocket.CloseNormalClosure, "fixture_complete")
		return nil
	})
	require.Contains(t, endpoint, "?model=friendly")
	client := connectLiveFixture(t, endpoint+"&key=attacker-key&model=gemini-ultra&alt=sse&%40evil.example=1")
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	for {
		if _, _, err := client.ReadMessage(); err != nil {
			break
		}
	}
	select {
	case result := <-results:
		require.Nil(t, result.biz)
		require.NotEmpty(t, result.endpoint)
		require.False(t, strings.Contains(result.endpoint, "?"), "provider endpoint must carry no query: %s", result.endpoint)
		require.NotContains(t, result.endpoint, "attacker")
		require.NotContains(t, result.endpoint, "evil.example")
	case <-time.After(8 * time.Second):
		require.FailNow(t, "Live handler did not finish")
	}
}

// TestGeminiLiveRefusesEndpointOutsideProviderPolicy verifies the shared
// dial-site policy runs before any credential is sent: a transport endpoint
// with userinfo or a fragment is refused with a configuration error and the
// provider is never dialed. Parameters: t is the test handle. Returns: none.
func TestGeminiLiveRefusesEndpointOutsideProviderPolicy(t *testing.T) {
	var dials atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dials.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(provider.Close)
	host := strings.TrimPrefix(provider.URL, "http://")
	for _, endpoint := range []string{
		"ws://user:secret@" + host + "/ws/live",
		"ws://" + host + "/ws/live#secret",
		"http://" + host + "/ws/live",
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=friendly", nil)
		c.Request.Header.Set("Connection", "Upgrade")
		c.Request.Header.Set("Upgrade", "websocket")
		c.Request.Header.Set("Sec-WebSocket-Version", "13")
		c.Request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		gmw.SetLogger(c, logger.Logger)
		SetLiveSpendGate(c, newTestLiveGate(-1))
		m := &meta.Meta{ChannelType: channeltype.Gemini, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live", OriginModelName: "friendly"}
		biz, usage := LiveHandlerWithTransport(c, m, LiveTransport{
			Endpoint: endpoint, Headers: http.Header{"X-Goog-Api-Key": []string{"provider-fixture"}},
			ModelResource: "models/gemini-3.8-live",
		})
		require.NotNil(t, biz, "endpoint=%q", endpoint)
		require.Nil(t, usage)
		require.Equal(t, http.StatusBadRequest, biz.StatusCode)
		require.NotContains(t, biz.Error.Message, "secret")
	}
	require.Zero(t, dials.Load(), "a refused endpoint must never be dialed")
}
