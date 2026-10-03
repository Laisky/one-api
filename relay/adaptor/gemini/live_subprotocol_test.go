package gemini

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGeminiLiveNegotiatesClientSubprotocol exercises actual upgrade responses
// and native setup frames. Parameters: t is the test handle. Returns: none.
// Authentication tokens must never be selected as the negotiated subprotocol.
func TestGeminiLiveNegotiatesClientSubprotocol(t *testing.T) {
	for _, channel := range []int{channeltype.Gemini, channeltype.GeminiOpenAICompatible} {
		for _, tc := range []struct {
			name      string
			protocols []string
			want      string
		}{
			{"realtime", []string{"realtime", "openai-insecure-api-key.browser-fixture"}, "realtime"},
			{"native_after_auth", []string{"openai-insecure-api-key.browser-fixture", "gemini-live"}, "gemini-live"},
			{"no_protocol", nil, ""},
			{"auth_only_not_echoed", []string{"openai-insecure-api-key.browser-fixture"}, ""},
		} {
			t.Run(fmt.Sprintf("%d/%s", channel, tc.name), func(t *testing.T) {
				endpoint, results := liveFixture(t, channel, "gemini-3.8-live", func(up *websocket.Conn) error {
					return acknowledgeLiveFixture(up, "gemini-3.8-live")
				})
				dialer := websocket.Dialer{Subprotocols: tc.protocols, HandshakeTimeout: 3 * time.Second}
				conn, response, err := dialer.Dial(endpoint, http.Header{"Authorization": []string{"Bearer downstream-fixture"}})
				if response != nil && response.Body != nil {
					t.Cleanup(func() { _ = response.Body.Close() })
				}
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"setup":{"model":"friendly"}}`)))
				_, ack, err := conn.ReadMessage()
				require.NoError(t, err)
				require.JSONEq(t, `{"setupComplete":{}}`, string(ack))
				usage := receiveLiveFixture(t, results)
				require.False(t, usage.Realtime.HasUsageGap())
				require.Empty(t, usage.Realtime.Records)
				t.Logf("offered=%v negotiated=%q", tc.protocols, conn.Subprotocol())
				require.Equal(t, tc.want, conn.Subprotocol())
				require.NotContains(t, response.Header.Get("Sec-WebSocket-Protocol"), "browser-fixture")
			})
		}
	}
}
