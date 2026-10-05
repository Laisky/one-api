package controller

import (
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

const terminalWSBalance = int64(100000)
const terminalWSHold = int64(1000)
const terminalWSFirstID = "resp_ws_terminal_first"
const terminalWSSecondID = "resp_ws_terminal_second"

// terminalWSWire records synthetic provider headers and each bounded application frame.
type terminalWSWire struct {
	path, query, authorization, beta string
	body                             []byte
}

// terminalWSResult captures controller outcome and reservation before its request context is recycled.
type terminalWSResult struct {
	err  *relaymodel.ErrorWithStatusCode
	held int64
}

// terminalWSFixture owns a real client/proxy/provider socket session and observable controller settlement.
type terminalWSFixture struct {
	client       *websocket.Conn
	done         <-chan terminalWSResult
	providerDone <-chan error
	wire         <-chan terminalWSWire
	creates      *atomic.Int32
	id           string
	store        *state.MemoryStore
}

// terminalWSEvent returns a Response event with independently specified event type, status, ID, and complete usage counters.
func terminalWSEvent(t *testing.T, eventType, status, id string, output int) []byte {
	t.Helper()
	response := map[string]any{"id": id, "object": "response", "model": "gpt-4", "store": true, "output": []any{},
		"usage": map[string]any{"input_tokens": 3, "output_tokens": output, "total_tokens": 3 + output}}
	if status != "" {
		response["status"] = status
	}
	encoded, err := json.Marshal(map[string]any{"type": eventType, "response": response})
	require.NoError(t, err)
	return encoded
}

// startTerminalWSFixture starts bounded local sockets and the production Responses controller with real SQLite billing.
// It returns the authenticated client and observations without invoking any external provider.
func startTerminalWSFixture(t *testing.T, firstEvents [][]byte, closeAfterEvents, continueSession bool) *terminalWSFixture {
	t.Helper()
	xaiVideoSetup(t, terminalWSBalance, false)
	canonicalAdmissionConfiguration(t)
	require.Equal(t, "sqlite", model.DB.Dialector.Name())
	oldEstimate := config.PreconsumeTokenForBackgroundRequest
	config.PreconsumeTokenForBackgroundRequest = int(terminalWSHold)
	t.Cleanup(func() { config.PreconsumeTokenForBackgroundRequest = oldEstimate })
	store := enableStateForTest(t)
	owner := state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}
	_, err := store.GetResponseBinding(context.Background(), owner, terminalWSFirstID)
	require.ErrorIs(t, err, state.ErrNotFound, "fixture must not begin with a persistent continuation binding")
	wire := make(chan terminalWSWire, 3)
	providerDone := make(chan error, 1)
	creates := new(atomic.Int32)
	child := terminalWSEvent(t, "response.completed", "completed", terminalWSSecondID, 1)
	upgrader := websocket.Upgrader{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(providerDone)
		conn, upgradeErr := upgrader.Upgrade(w, r, nil)
		if upgradeErr != nil {
			providerDone <- upgradeErr
			return
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				t.Logf("fixture provider close: %v", closeErr)
			}
		}()
		if deadlineErr := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); deadlineErr != nil {
			providerDone <- deadlineErr
			return
		}
		for {
			_, body, readErr := conn.ReadMessage()
			if readErr != nil {
				// The controller closes this leg after either client close or denied continuation.
				providerDone <- nil
				return
			}
			count := creates.Add(1)
			wire <- terminalWSWire{path: r.URL.Path, query: r.URL.RawQuery, authorization: r.Header.Get("Authorization"), beta: r.Header.Get("OpenAI-Beta"), body: body}
			events := firstEvents
			if count > 1 {
				events = [][]byte{child}
			}
			for _, event := range events {
				if writeErr := conn.WriteMessage(websocket.TextMessage, event); writeErr != nil {
					providerDone <- writeErr
					return
				}
			}
			if closeAfterEvents {
				providerDone <- conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "fixture ended before terminal receipt"), time.Now().Add(time.Second))
				return
			}
			if count >= 2 || !continueSession {
				// Keep the socket open until the test has read every delivered receipt and closes normally.
				_, _, closingErr := conn.ReadMessage()
				if closingErr == nil {
					providerDone <- fmt.Errorf("fixture received an unexpected additional application frame")
				} else {
					providerDone <- nil
				}
				return
			}
		}
	}))
	t.Cleanup(provider.Close)
	// The native capability guard recognizes this marker, while the resolver replaces
	// the path with /v1/responses and connects exclusively to the local test host.
	configured, _, id := protocolContext(t, channeltype.OpenAI, "gpt-4", "/v1/responses", "", provider.URL+"/api.openai.com",
		terminalWSBalance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	done := make(chan terminalWSResult, 1)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		defer close(done)
		for key, value := range configured.Keys {
			c.Set(key, value)
		}
		gmw.SetLogger(c, logger.Logger)
		c.Request.Header.Set("Authorization", "Bearer upstream-fixture-key")
		apiErr := RelayResponseAPIHelper(c)
		done <- terminalWSResult{err: apiErr, held: c.GetInt64(ctxkey.PreConsumedQuotaAmount)}
	})
	proxy := httptest.NewServer(router)
	t.Cleanup(proxy.Close)
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses?model=alias", nil)
	if err != nil {
		select {
		case result := <-done:
			t.Logf("fixture handshake controller_error=%+v held=%d", result.err, result.held)
		case <-time.After(5 * time.Second):
			t.Log("fixture handshake controller did not return")
		}
	}
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("fixture client close: %v", closeErr)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("controller did not terminate during fixture cleanup")
		}
		drainCriticalTasks(t)
	})
	return &terminalWSFixture{client: conn, done: done, providerDone: providerDone, wire: wire, creates: creates, id: id, store: store}
}

// sendTerminalWSCreate writes one client creation with an optional previous response ID and no unbounded external work.
func sendTerminalWSCreate(t *testing.T, fixture *terminalWSFixture, previous string) {
	t.Helper()
	create := map[string]any{"type": "response.create", "model": "alias", "input": "synthetic fixture", "max_output_tokens": 200}
	if previous != "" {
		create["previous_response_id"] = previous
	}
	require.NoError(t, fixture.client.WriteJSON(create))
}

// finishTerminalWSFixture closes the client when requested and collects both controller and provider lifecycles before billing checks.
func finishTerminalWSFixture(t *testing.T, fixture *terminalWSFixture, closeClient bool) terminalWSResult {
	t.Helper()
	if closeClient {
		require.NoError(t, fixture.client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "test done"), time.Now().Add(time.Second)))
	}
	var result terminalWSResult
	select {
	case result = <-fixture.done:
	case <-time.After(5 * time.Second):
		t.Fatal("controller did not complete its bounded socket session")
	}
	select {
	case providerErr := <-fixture.providerDone:
		require.NoError(t, providerErr)
	case <-time.After(5 * time.Second):
		t.Fatal("provider fixture did not terminate")
	}
	drainCriticalTasks(t)
	require.Nil(t, result.err, "executed socket sessions must reconcile instead of releasing their hold")
	require.Equal(t, terminalWSHold, result.held, "reservation must match the independent fixed estimate and unit tariff")
	for i := int32(0); i < fixture.creates.Load(); i++ {
		observed := <-fixture.wire
		require.Equal(t, "/v1/responses", observed.path)
		require.Equal(t, "model=alias", observed.query)
		require.Equal(t, "Bearer upstream-fixture-key", observed.authorization)
		require.Equal(t, "responses-api=v1", observed.beta)
		var create map[string]any
		require.NoError(t, json.Unmarshal(observed.body, &create))
		require.Equal(t, "response.create", create["type"])
		require.Equal(t, "gpt-4", create["model"], "production model guard must map the same billed model on the provider wire")
		if i > 0 {
			require.Equal(t, terminalWSFirstID, create["previous_response_id"])
		}
	}
	return result
}

// assertTerminalWSLedger reads all four durable settlement views and asserts exact debit, receipt authority, provenance, and one final consume row.
func assertTerminalWSLedger(t *testing.T, fixture *terminalWSFixture, want int64, estimated bool, measuredOutput int) {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", fixture.id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	var costRows int64
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", fixture.id).Count(&costRows).Error)
	cost := requestCostQuota(t, fixture.id)
	t.Logf("WS_TERMINAL_LEDGER creates=%d user_debit=%d token_debit=%d cost=%d cost_rows=%d consume_rows=%d expected=%d", fixture.creates.Load(), terminalWSBalance-reloadUserQuota(t), terminalWSBalance-token.RemainQuota, cost, costRows, len(logs), want)
	for _, entry := range logs {
		t.Logf("WS_TERMINAL_LOG type=%d quota=%d input=%d output=%d estimated=%v reason=%v", entry.Type, entry.Quota, entry.PromptTokens, entry.CompletionTokens, entry.Metadata["billing_estimated"], entry.Metadata["billing_estimate_reason"])
	}
	require.Equal(t, terminalWSBalance-want, reloadUserQuota(t))
	require.Equal(t, terminalWSBalance-want, token.RemainQuota)
	require.Equal(t, want, cost)
	require.EqualValues(t, 1, costRows)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	require.EqualValues(t, want, logs[0].Quota)
	if measuredOutput >= 0 {
		require.Equal(t, measuredOutput, logs[0].CompletionTokens, "independent terminal output counter must survive settlement")
	}
	if estimated {
		require.Equal(t, true, logs[0].Metadata["billing_estimated"])
		require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
	} else {
		require.Empty(t, logs[0].Metadata["billing_estimate_reason"])
	}
}

// TestSecurityResponseWSTerminalReceiptHTTP bills the authoritative final receipt once and preserves unresolved work at the full handshake estimate.
func TestSecurityResponseWSTerminalReceiptHTTP(t *testing.T) {
	for _, scenario := range []string{"progress_then_terminal", "close_before_terminal", "different_id_terminal", "completed_only", "duplicate_terminal"} {
		t.Run(scenario, func(t *testing.T) {
			progress := terminalWSEvent(t, "response.in_progress", "in_progress", terminalWSFirstID, 0)
			terminal := terminalWSEvent(t, "response.completed", "completed", terminalWSFirstID, 100)
			events := [][]byte{progress, terminal}
			closeAfter := scenario == "close_before_terminal"
			estimated := closeAfter || scenario == "different_id_terminal"
			if closeAfter {
				events = [][]byte{progress}
			} else if scenario == "different_id_terminal" {
				events = [][]byte{progress, terminalWSEvent(t, "response.completed", "completed", terminalWSSecondID, 100)}
			} else if scenario == "completed_only" {
				events = [][]byte{terminal}
			} else if scenario == "duplicate_terminal" {
				events = [][]byte{terminal, terminal}
			}
			fixture := startTerminalWSFixture(t, events, closeAfter, false)
			sendTerminalWSCreate(t, fixture, "")
			for _, expected := range events {
				_, observed, err := fixture.client.ReadMessage()
				require.NoError(t, err)
				require.JSONEq(t, string(expected), string(observed), "raw socket forwarding must preserve the provider event")
			}
			if closeAfter {
				_, _, err := fixture.client.ReadMessage()
				require.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
			}
			finishTerminalWSFixture(t, fixture, !closeAfter)
			require.EqualValues(t, 1, fixture.creates.Load())
			want := int64(3 + 100)
			output := 100
			if estimated {
				want = terminalWSHold
				if closeAfter {
					output = -1
				}
			}
			assertTerminalWSLedger(t, fixture, want, estimated, output)
		})
	}
}

// TestSecurityResponseWSTerminalOwnershipHTTP denies continuation from explicitly unfinished completion envelopes while preserving completed and legacy status-omitted flows.
func TestSecurityResponseWSTerminalOwnershipHTTP(t *testing.T) {
	for _, status := range []string{"queued", "in_progress", "completed", ""} {
		t.Run("status="+status, func(t *testing.T) {
			valid := status == "completed" || status == ""
			first := terminalWSEvent(t, "response.completed", status, terminalWSFirstID, 1)
			fixture := startTerminalWSFixture(t, [][]byte{first}, false, true)
			sendTerminalWSCreate(t, fixture, "")
			_, observed, err := fixture.client.ReadMessage()
			require.NoError(t, err)
			require.JSONEq(t, string(first), string(observed))
			sendTerminalWSCreate(t, fixture, terminalWSFirstID)
			_, reply, readErr := fixture.client.ReadMessage()
			finishTerminalWSFixture(t, fixture, readErr == nil)
			owner := state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}
			binding, bindingErr := fixture.store.GetResponseBinding(context.Background(), owner, terminalWSFirstID)
			t.Logf("WS_TERMINAL_OWNERSHIP status=%q creates=%d read_error=%v stored_binding=%v", status, fixture.creates.Load(), readErr, binding != nil)
			if valid {
				require.NoError(t, readErr)
				require.Contains(t, string(reply), terminalWSSecondID)
				require.EqualValues(t, 2, fixture.creates.Load())
				require.NoError(t, bindingErr)
				require.NotNil(t, binding)
				require.Equal(t, terminalWSFirstID, binding.UpstreamResponseID)
				assertTerminalWSLedger(t, fixture, 2*(3+1), false, 2)
			} else {
				require.ErrorIs(t, bindingErr, state.ErrNotFound)
				require.Nil(t, binding)
				require.EqualValues(t, 1, fixture.creates.Load(), "unfinished provider ID must not authorize a second same-socket dispatch")
				require.True(t, websocket.IsCloseError(readErr, websocket.ClosePolicyViolation), "unfinished continuation must be denied on this socket")
				assertTerminalWSLedger(t, fixture, terminalWSHold, true, -1)
			}
		})
	}
}
