package openai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
)

// responseWSFrameOutcome is what one client frame produced on a real proxied socket.
type responseWSFrameOutcome struct {
	readErr   error
	closeErr  *websocket.CloseError
	forwarded []string
	bizErr    *rmodel.ErrorWithStatusCode
}

// runResponseWSFrame opens a client socket to ResponseAPIWebSocketHandler bound
// to owner (1,2) on channel 3 with model gpt-4o-mini, sends frame, and returns
// the client-observed result plus every frame the provider received.
func runResponseWSFrame(t *testing.T, frame string) responseWSFrameOutcome {
	t.Helper()
	received := make(chan string, 4)
	upstream := newOwnershipUpstream(t, received)
	defer upstream.Close()
	done := make(chan *rmodel.ErrorWithStatusCode, 1)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		meta := &rmeta.Meta{Mode: relaymode.ResponseAPI, BaseURL: upstream.URL, APIKey: "fixture", OriginModelName: "gpt-4o-mini", ActualModelName: "gpt-4o-mini", UserId: 1, TokenId: 2, ChannelId: 3}
		bizErr, _, _ := ResponseAPIWebSocketHandler(c, meta)
		done <- bizErr
	})
	proxy := httptest.NewServer(router)
	defer proxy.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(frame)))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var outcome responseWSFrameOutcome
	_, _, outcome.readErr = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if errors.As(outcome.readErr, &closeErr) {
		outcome.closeErr = closeErr
	}
	if outcome.readErr == nil {
		require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
	}
	select {
	case outcome.bizErr = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not terminate")
	}
	for drained := false; !drained; {
		select {
		case got := <-received:
			outcome.forwarded = append(outcome.forwarded, got)
		default:
			drained = true
		}
	}
	return outcome
}

// TestSecurityResponseWSConversation proves a WebSocket response.create never
// forwards a conversation selector. Gateway conversations live only in the
// owner-scoped store and cannot be hydrated on the passthrough socket, so every
// non-empty selector, owned or not, is rejected with a clear close reason before
// dispatch; empty selectors name nothing and are stripped.
func TestSecurityResponseWSConversation(t *testing.T) {
	owner := state.OwnerScope{UserID: 1, TokenID: 2}
	for _, scenario := range []string{"owned-string", "owned-object", "foreign", "provider-style", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			store := state.NewMemoryStore(state.DefaultLimits())
			state.SetForTest(store)
			t.Cleanup(func() { state.SetForTest(nil) })
			convOwner := owner
			if scenario == "foreign" {
				convOwner = state.OwnerScope{UserID: 9, TokenID: 9}
			}
			convID, err := state.NewConversationID()
			require.NoError(t, err)
			_, err = store.CreateConversation(context.Background(), &state.ConversationStateRecord{GatewayConversationID: convID, Owner: convOwner}, "")
			require.NoError(t, err)
			selector := `"` + convID + `"`
			switch scenario {
			case "owned-object":
				selector = `{"id":"` + convID + `"}`
			case "provider-style":
				convID = "conv_" + strings.Repeat("0123456789abcdef", 3)
				selector = `"` + convID + `"`
			case "disabled":
				state.SetForTest(nil)
			}

			outcome := runResponseWSFrame(t, `{"type":"response.create","model":"gpt-4o-mini","input":"continue","conversation":`+selector+`}`)
			require.Empty(t, outcome.forwarded, "a conversation selector must never reach the provider socket")
			require.Error(t, outcome.readErr, "the frame must close the socket before forwarding")
			require.NotNil(t, outcome.closeErr, "the client must receive a close frame")
			require.Equal(t, websocket.ClosePolicyViolation, outcome.closeErr.Code)
			require.Contains(t, outcome.closeErr.Text, "conversation", "the close reason must explain the rejection")
			require.NotNil(t, outcome.bizErr, "rejection before execution must release the handshake reservation")
		})
	}
}

// TestSecurityResponseWSEmptyConversationStripped keeps selector-free sockets
// working: an empty or null conversation selector names nothing, so the frame is
// forwarded without any conversation key.
func TestSecurityResponseWSEmptyConversationStripped(t *testing.T) {
	for name, selector := range map[string]string{"null": `null`, "empty-string": `""`, "empty-object": `{}`, "empty-id": `{"id":""}`} {
		t.Run(name, func(t *testing.T) {
			state.SetForTest(nil)
			outcome := runResponseWSFrame(t, `{"type":"response.create","model":"gpt-4o-mini","input":"hello","conversation":`+selector+`}`)
			require.NoError(t, outcome.readErr)
			require.Nil(t, outcome.bizErr)
			require.Len(t, outcome.forwarded, 1)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(outcome.forwarded[0]), &root))
			require.NotContains(t, root, "conversation", "the provider must never receive a conversation selector")
		})
	}
}
