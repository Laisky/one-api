package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
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
			upstream := newOwnershipUpstream(t, received)
			defer upstream.Close()
			done := make(chan *rmodel.ErrorWithStatusCode, 1)
			router := gin.New()
			router.GET("/v1/responses", func(c *gin.Context) {
				meta := &rmeta.Meta{Mode: relaymode.ResponseAPI, BaseURL: upstream.URL, APIKey: "fixture", ActualModelName: "gpt-4o-mini", UserId: 1, TokenId: 2, ChannelId: 3}
				if scenario == "foreign" || scenario == "binary-foreign" {
					meta.UserId = 9
				}
				if scenario == "other-channel" {
					meta.ChannelId = 4
				}
				bizErr, _, _ := ResponseAPIWebSocketHandler(c, meta)
				done <- bizErr
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
			if scenario == "initial" {
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","previous_response_id":"resp_session","input":"continue locally"}`)))
				_, _, err := conn.ReadMessage()
				require.NoError(t, err, "provider-confirmed store=false responses remain usable on this socket")
				select {
				case forwarded := <-received:
					require.Contains(t, forwarded, "resp_session")
				case <-time.After(time.Second):
					t.Fatal("same-socket continuation was not forwarded")
				}
			}

			if readErr == nil {
				require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
			}
			select {
			case bizErr := <-done:
				if scenario == "owner" || scenario == "initial" {
					require.Nil(t, bizErr)
				} else {
					require.NotNil(t, bizErr, "rejection before execution must release the handshake reservation")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("proxy did not terminate")
			}
		})
	}
}

// newOwnershipUpstream records every application frame, including binary frames, and returns a terminal fixture response.
func newOwnershipUpstream(t *testing.T, received chan<- string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade fixture socket: %v", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close fixture socket: %v", err)
			}
		}()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			} // Closing either proxy leg terminates the fixture.
			received <- string(payload)
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_session","store":false,"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				t.Errorf("write fixture completion: %v", err)
				return
			}
		}
	}))
}
