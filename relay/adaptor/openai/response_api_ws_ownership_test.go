package openai

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestResponseAPIWSOwnership checks real socket dispatch for owner bindings and rejects alternate-path background work.
func TestResponseAPIWSOwnership(t *testing.T) {
	for _, scenario := range []string{"initial", "owner", "foreign", "unknown", "disabled", "other-channel", "background", "binary-foreign"} {
		t.Run(scenario, func(t *testing.T) {
			store := state.NewMemoryStore(state.DefaultLimits())
			state.SetForTest(store)
			t.Cleanup(func() { state.SetForTest(nil) })
			_, err := store.CreateResponse(context.Background(), &state.ResponseStateRecord{GatewayResponseID: "resp_gateway", Owner: state.OwnerScope{UserID: 1, TokenID: 2}, StoreMode: true, Status: state.StatusCompleted, Binding: &state.ProviderBinding{ChannelID: 3, APIType: 0, UpstreamResponseID: "resp_bound_provider"}}, "")
			require.NoError(t, err)
			if scenario == "disabled" {
				state.SetForTest(nil)
			}
			received := make(chan string, 4)
			upstream := newEchoingUpstream(t, received)
			defer upstream.Close()
			done := make(chan struct{})
			router := gin.New()
			router.GET("/v1/responses", func(c *gin.Context) {
				meta := &rmeta.Meta{Mode: relaymode.ResponseAPI, BaseURL: upstream.URL, APIKey: "fixture", ActualModelName: "gpt-4o-mini", UserId: 1, TokenId: 2, ChannelId: 3}
				if scenario == "foreign" || scenario == "binary-foreign" {
					meta.UserId = 9
				}
				if scenario == "other-channel" {
					meta.ChannelId = 4
				}
				_, _, _ = ResponseAPIWebSocketHandler(c, meta)
				close(done)
			})
			proxy := httptest.NewServer(router)
			defer proxy.Close()
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			defer func() { require.NoError(t, conn.Close()) }()
			payload := `{"type":"response.create","model":"gpt-4o-mini","previous_response_id":"resp_gateway","input":"continue"}`
			if scenario == "initial" {
				payload = `{"type":"response.create","model":"gpt-4o-mini","input":"hello"}`
			}
			if scenario == "unknown" {
				payload = strings.ReplaceAll(payload, "resp_gateway", "resp_unknown")
			}
			if scenario == "background" {
				payload = `{"type":"response.create","model":"gpt-4o-mini","background":true,"input":"hello"}`
			}
			frameType := websocket.TextMessage
			if scenario == "binary-foreign" {
				frameType = websocket.BinaryMessage
			}
			require.NoError(t, conn.WriteMessage(frameType, []byte(payload)))
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
			_, _, readErr := conn.ReadMessage()
			if scenario == "owner" || scenario == "initial" {
				require.NoError(t, readErr)
				select {
				case forwarded := <-received:
					if scenario == "owner" {
						require.Contains(t, forwarded, "resp_bound_provider")
					}
				case <-time.After(time.Second):
					t.Fatal("valid request was not forwarded")
				}
			} else {
				require.Error(t, readErr, "unauthorized frame must close the socket before forwarding")
				require.Empty(t, received, "rejected frame reached the provider")
			}
			if readErr == nil {
				require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("proxy did not terminate")
			}
		})
	}
}
