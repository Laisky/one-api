package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// wsSettlementBalance is the fixture balance for WebSocket settlement tests.
const wsSettlementBalance = int64(5_000_000)

// wsScriptedUpstream is a fake native Responses WebSocket provider that answers
// the i-th client frame with script[i] and records every received frame.
type wsScriptedUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	frames []string
}

// newWSScriptedUpstream starts the scripted provider. When closeAfterScript is
// true it closes the socket normally after answering the last scripted frame.
// It returns the running fixture.
func newWSScriptedUpstream(t *testing.T, script [][]string, closeAfterScript bool) *wsScriptedUpstream {
	t.Helper()
	upstream := &wsScriptedUpstream{}
	upgrader := websocket.Upgrader{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade scripted provider socket: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		for index := 0; ; index++ {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return // Closing either proxy leg terminates the fixture.
			}
			upstream.mu.Lock()
			upstream.frames = append(upstream.frames, string(payload))
			upstream.mu.Unlock()
			if index >= len(script) {
				continue
			}
			for _, event := range script[index] {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
					t.Errorf("write scripted provider event: %v", err)
					return
				}
			}
			if closeAfterScript && index == len(script)-1 {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
				return
			}
		}
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

// received returns a copy of every frame the scripted provider received.
func (u *wsScriptedUpstream) received() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.frames...)
}

// wsSettlementGateway runs the real /v1/responses WebSocket handler with ledger
// fixtures in front of upstream. It returns the gateway server, the request id
// used for settlement records, and a channel delivering the handler result.
func wsSettlementGateway(t *testing.T, upstream *wsScriptedUpstream) (*httptest.Server, string, <-chan *relaymodel.ErrorWithStatusCode) {
	t.Helper()
	hash := sha256.Sum256([]byte(t.Name()))
	requestID := fmt.Sprintf("ws-%x", hash[:10])
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.UserRequestCost{}).Error)
	done := make(chan *relaymodel.ErrorWithStatusCode, 1)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		for key, value := range map[string]any{
			ctxkey.Channel: channeltype.OpenAI, ctxkey.ChannelId: fallbackOpenAIChannelID,
			ctxkey.ChannelModel: &model.Channel{Id: fallbackOpenAIChannelID, Type: channeltype.OpenAI},
			ctxkey.TokenId:      fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
			ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{}, ctxkey.ChannelRatio: 1.0,
			ctxkey.RequestModel: "gpt-4o-mini", ctxkey.BaseURL: upstream.server.URL + "/api.openai.com",
			ctxkey.RequestId: requestID, ctxkey.Username: "response-fallback", ctxkey.Config: model.ChannelConfig{},
			ctxkey.UserObj:             &model.User{Id: fallbackUserID, Quota: wsSettlementBalance},
			ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: wsSettlementBalance,
		} {
			c.Set(key, value)
		}
		handled, bizErr := maybeHandleResponseAPIWebSocket(c, metalib.GetByContext(c))
		if !handled {
			t.Errorf("websocket upgrade was not handled")
		}
		done <- bizErr
	})
	gateway := httptest.NewServer(router)
	t.Cleanup(gateway.Close)
	return gateway, requestID, done
}

// wsSettlementSetup resets the durable ledger, enables consume logs, and
// captures every Responses settlement. It returns the captured settlements.
func wsSettlementSetup(t *testing.T) func() []billing.QuotaConsumeDetail {
	t.Helper()
	billingAccountingSetup(t, wsSettlementBalance)
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })
	config.SetLogConsumeEnabled(true)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", wsSettlementBalance).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"remain_quota": wsSettlementBalance, "used_quota": 0, "unlimited_quota": false}).Error)
	var mu sync.Mutex
	var settlements []billing.QuotaConsumeDetail
	original := postConsumeResponseAPIQuotaDetailed
	postConsumeResponseAPIQuotaDetailed = func(detail billing.QuotaConsumeDetail) {
		mu.Lock()
		settlements = append(settlements, detail)
		mu.Unlock()
		original(detail)
	}
	t.Cleanup(func() { postConsumeResponseAPIQuotaDetailed = original })
	return func() []billing.QuotaConsumeDetail {
		mu.Lock()
		defer mu.Unlock()
		return append([]billing.QuotaConsumeDetail(nil), settlements...)
	}
}

// wsReadUntilClosed reads client frames until the gateway closes the socket and returns them.
func wsReadUntilClosed(t *testing.T, conn *websocket.Conn) []string {
	t.Helper()
	var frames []string
	for {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			require.ErrorAs(t, err, &closeErr, "the gateway must close the socket rather than time out")
			return frames
		}
		frames = append(frames, string(payload))
	}
}

// wsReadN reads exactly n client frames and fails if the socket closes first.
func wsReadN(t *testing.T, conn *websocket.Conn, n int) {
	t.Helper()
	for range n {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, _, err := conn.ReadMessage()
		require.NoError(t, err)
	}
}

// wsAwaitHandler waits for the gateway handler result and all detached billing.
func wsAwaitHandler(t *testing.T, done <-chan *relaymodel.ErrorWithStatusCode) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	select {
	case bizErr := <-done:
		drainCriticalTasks(t)
		return bizErr
	case <-time.After(10 * time.Second):
		t.Fatal("websocket handler did not terminate")
		return nil
	}
}

// wsEvent renders one provider Responses event for response id with the given
// event type, status and usage counters.
func wsEvent(eventType, id, status string, input, output int) string {
	return fmt.Sprintf(`{"type":%q,"response":{"id":%q,"object":"response","status":%q,"output":[],"usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}}`,
		eventType, id, status, input, output, input+output)
}

// TestSecurityResponseWSSettlesTerminalReceipt proves the WebSocket usage
// collector settles the provider's terminal receipt rather than the first
// non-terminal snapshot, retains the full reservation with estimate provenance
// when the socket closes before a terminal receipt, and settles duplicated
// terminal receipts once. The ledger, request cost and the single consume log
// must all agree with the one settlement.
func TestSecurityResponseWSSettlesTerminalReceipt(t *testing.T) {
	const responseID = "resp_ws_receipt"
	cases := []struct {
		name     string
		events   []string
		terminal bool
	}{
		{name: "in_progress_then_completed", events: []string{wsEvent("response.in_progress", responseID, "in_progress", 3, 0), wsEvent("response.completed", responseID, "completed", 3, 100)}, terminal: true},
		{name: "close_before_terminal", events: []string{wsEvent("response.in_progress", responseID, "in_progress", 3, 0)}},
		{name: "completed_only", events: []string{wsEvent("response.completed", responseID, "completed", 3, 100)}, terminal: true},
		{name: "duplicated_terminal", events: []string{wsEvent("response.completed", responseID, "completed", 3, 100), wsEvent("response.completed", responseID, "completed", 3, 100)}, terminal: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settlements := wsSettlementSetup(t)
			upstream := newWSScriptedUpstream(t, [][]string{tc.events}, true)
			gateway, requestID, done := wsSettlementGateway(t, upstream)
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			defer func() { _ = conn.Close() }()
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":"hello"}`)))
			require.Len(t, wsReadUntilClosed(t, conn), len(tc.events))
			require.Nil(t, wsAwaitHandler(t, done))

			got := settlements()
			require.Len(t, got, 1, "exactly one final settlement")
			settled := got[0]
			t.Logf("settled prompt=%d completion=%d total=%d delta=%d", settled.PromptTokens, settled.CompletionTokens, settled.TotalQuota, settled.QuotaDelta)
			require.Equal(t, wsSettlementBalance-settled.TotalQuota, reloadUserQuota(t), "user debit must equal the settlement")
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, wsSettlementBalance-settled.TotalQuota, token.RemainQuota, "token debit must equal the settlement")
			require.Equal(t, settled.TotalQuota, requestCostQuota(t, requestID), "request cost must equal the settlement")
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1, "one settled consume row, not an orphan provisional or duplicate")
			require.EqualValues(t, settled.TotalQuota, logs[0].Quota)

			if tc.terminal {
				require.Equal(t, 3, settled.PromptTokens, "the terminal receipt prices the input")
				require.Equal(t, 100, settled.CompletionTokens, "the terminal receipt prices the output")
				require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
				return
			}
			require.Zero(t, settled.QuotaDelta, "a missing terminal receipt must retain the full reservation")
			require.Equal(t, true, logs[0].Metadata["billing_estimated"])
			require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
		})
	}
}

// TestSecurityResponseWSNonTerminalOwnership proves a response.completed envelope
// that still reports unfinished work grants neither connection-local nor durable
// continuation ownership, while a genuinely completed response remains
// continuable on the same socket and is persisted.
func TestSecurityResponseWSNonTerminalOwnership(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      string
		continuable bool
	}{{"completed_envelope_in_progress", "in_progress", false}, {"completed_envelope_queued", "queued", false}, {"genuinely_completed", "completed", true}} {
		t.Run(tc.name, func(t *testing.T) {
			wsSettlementSetup(t)
			store := enableStateForTest(t)
			parentID := "resp_ws_parent_" + tc.status
			script := [][]string{
				{wsEvent("response.completed", parentID, tc.status, 3, 0)},
				{wsEvent("response.completed", "resp_ws_child_"+tc.status, "completed", 3, 5)},
			}
			upstream := newWSScriptedUpstream(t, script, true)
			gateway, _, done := wsSettlementGateway(t, upstream)
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			defer func() { _ = conn.Close() }()

			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":"hello"}`)))
			wsReadN(t, conn, 1)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","previous_response_id":"`+parentID+`","input":"continue"}`)))
			tail := wsReadUntilClosed(t, conn)
			wsAwaitHandler(t, done)

			frames := upstream.received()
			t.Logf("provider frames=%d", len(frames))
			_, getErr := store.GetResponse(context.Background(), state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}, parentID)
			if tc.continuable {
				require.Len(t, frames, 2, "a completed response remains continuable on its socket")
				require.Contains(t, frames[1], parentID)
				require.Len(t, tail, 1)
				require.NoError(t, getErr, "a completed response is persisted for later continuation")
				return
			}
			require.Len(t, frames, 1, "unfinished work must not be continued on the same socket")
			require.Empty(t, tail, "the continuation must be refused before any provider reply")
			require.Error(t, getErr, "unfinished work must not receive a durable binding")
		})
	}
}
