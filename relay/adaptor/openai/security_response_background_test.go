package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSecurityResponseNonTerminalReplyUsage proves the non-streaming Responses
// handlers never convert an unexpected queued or in-progress reply into
// prompt-only usage that would release the reservation, while completed and
// incomplete replies keep the existing synthesized fallback.
func TestSecurityResponseNonTerminalReplyUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := map[string]func(*gin.Context, *http.Response) (*rmodel.ErrorWithStatusCode, *rmodel.Usage){
		"direct": func(c *gin.Context, resp *http.Response) (*rmodel.ErrorWithStatusCode, *rmodel.Usage) {
			return ResponseAPIDirectHandler(c, resp, 40, "gpt-4o-mini")
		},
		"chat_conversion": func(c *gin.Context, resp *http.Response) (*rmodel.ErrorWithStatusCode, *rmodel.Usage) {
			return ResponseAPIHandler(c, resp, 40, "gpt-4o-mini")
		},
	}
	for handlerName, handle := range handlers {
		for _, status := range []string{"queued", "in_progress", "completed", "incomplete"} {
			t.Run(handlerName+"/"+status, func(t *testing.T) {
				body := `{"id":"resp_fixture","object":"response","status":"` + status + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial text"}]}],"usage":null}`
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
				apiErr, usage := handle(c, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				if status == "queued" || status == "in_progress" {
					require.Zero(t, usage.PromptTokens, "a non-terminal reply must not synthesize prompt usage")
					require.Zero(t, usage.CompletionTokens, "a non-terminal reply must not synthesize completion usage")
					require.NotEmpty(t, usage.BillingEstimateReason, "a non-terminal reply must retain the reservation as an explicit estimate")
					return
				}
				require.Equal(t, 40, usage.PromptTokens, "terminal replies keep the synthesized prompt fallback")
				require.Positive(t, usage.CompletionTokens, "terminal replies keep the synthesized completion fallback")
				require.Empty(t, usage.BillingEstimateReason)
			})
		}
	}
}

// TestSecurityResponseWSBackgroundVariants drives real client and provider
// sockets to prove case-folded, Unicode-folded, duplicate and non-boolean
// background keys are rejected before any frame reaches the provider, and that
// an accepted frame never carries a background key upstream.
func TestSecurityResponseWSBackgroundVariants(t *testing.T) {
	cases := map[string]struct {
		fields   string
		accepted bool
	}{
		"upper_true":                 {fields: `"BACKGROUND":true`},
		"title_true":                 {fields: `"Background":true`},
		"unicode_fold_true":          {fields: `"bacKground":true`},
		"exact_duplicate_last_false": {fields: `"background":true,"background":false`},
		"case_duplicate_false":       {fields: `"background":false,"Background":false`},
		"string_true":                {fields: `"background":"true"`},
		"number_true":                {fields: `"background":1`},
		"explicit_false":             {fields: `"background":false`, accepted: true},
		"explicit_null":              {fields: `"background":null`, accepted: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			received := make(chan string, 4)
			upstream := newOwnershipUpstream(t, received)
			defer upstream.Close()
			done := make(chan *rmodel.ErrorWithStatusCode, 1)
			router := gin.New()
			router.GET("/v1/responses", func(c *gin.Context) {
				meta := &rmeta.Meta{Mode: relaymode.ResponseAPI, BaseURL: upstream.URL, APIKey: "fixture", ActualModelName: "gpt-4o-mini", UserId: 1, TokenId: 2, ChannelId: 3}
				bizErr, _, _ := ResponseAPIWebSocketHandler(c, meta)
				done <- bizErr
			})
			proxy := httptest.NewServer(router)
			defer proxy.Close()
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			defer func() { require.NoError(t, conn.Close()) }()

			payload := `{"type":"response.create","model":"gpt-4o-mini","input":"hello",` + tc.fields + `}`
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(payload)))
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
			_, _, readErr := conn.ReadMessage()
			if tc.accepted {
				require.NoError(t, readErr)
				select {
				case forwarded := <-received:
					require.NotContains(t, strings.ToLower(forwarded), "background", "accepted frames must not carry a background key upstream")
				case <-time.After(time.Second):
					t.Fatal("valid foreground frame was not forwarded")
				}
				require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
			} else {
				select {
				case forwarded := <-received:
					t.Fatalf("rejected background frame reached the provider: %s", forwarded)
				default:
				}
				require.Error(t, readErr, "a background frame must close the socket before forwarding")
			}
			select {
			case bizErr := <-done:
				if tc.accepted {
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
