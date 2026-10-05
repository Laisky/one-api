package controller

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// liveToolFloorCase is one function-result timing scenario. behavior is the
// declared FunctionDeclaration behavior ("" omits the field); closeFirst ends
// the calling turn with its receipt before the result arrives; continuation
// streams model output after the result; interrupt sends an interruption
// before the calling turn's receipt; laterReceipt adds a receipt for a turn
// that starts after the result; estimated is whether the result must stay
// unreceipted and settle at the funded estimate.
type liveToolFloorCase struct {
	name         string
	behavior     string
	balance      int64
	closeFirst   bool
	continuation bool
	interrupt    bool
	laterReceipt bool
	forwarded    bool
	estimated    bool
}

// TestGeminiLiveFunctionResultReceiptFloor checks which receipt may cover a
// forwarded function result. Only an explicitly BLOCKING result answering a
// call of the still-open turn, followed by model output in that turn before
// any interruption, is covered by that turn's receipt; every other result
// needs a receipt for a turn that starts after it, and without one the session
// settles at the measured receipts plus the result's input estimate instead of
// serving the result unbilled. Parameters: t owns real loopback sockets and
// the in-memory user/token ledger. Returns: none.
func TestGeminiLiveFunctionResultReceiptFloor(t *testing.T) {
	const measured = int64(30_000)
	for _, tc := range []liveToolFloorCase{
		{name: "nonblocking_late_result_without_later_receipt", behavior: "NON_BLOCKING", balance: 100_000, closeFirst: true, forwarded: true, estimated: true},
		{name: "nonblocking_underfunded_result_is_not_forwarded", behavior: "NON_BLOCKING", balance: 50_000, closeFirst: true},
		{name: "nonblocking_late_result_with_later_receipt", behavior: "NON_BLOCKING", balance: 100_000, closeFirst: true, laterReceipt: true, forwarded: true},
		{name: "nonblocking_same_turn_result_without_later_receipt", behavior: "NON_BLOCKING", balance: 100_000, forwarded: true, estimated: true},
		{name: "nonblocking_same_turn_result_with_model_output", behavior: "NON_BLOCKING", balance: 100_000, continuation: true, forwarded: true, estimated: true},
		{name: "nonblocking_same_turn_result_with_later_receipt", behavior: "NON_BLOCKING", balance: 100_000, laterReceipt: true, forwarded: true},
		{name: "default_behavior_same_turn_result_without_later_receipt", balance: 100_000, forwarded: true, estimated: true},
		{name: "blocking_consumed_result_is_covered_by_its_receipt", behavior: "BLOCKING", balance: 100_000, continuation: true, forwarded: true},
		{name: "blocking_result_without_model_output", behavior: "BLOCKING", balance: 100_000, forwarded: true, estimated: true},
		{name: "blocking_result_after_its_turn_closed", behavior: "BLOCKING", balance: 100_000, closeFirst: true, forwarded: true, estimated: true},
		{name: "blocking_result_of_interrupted_turn", behavior: "BLOCKING", balance: 100_000, interrupt: true, forwarded: true, estimated: true},
		{name: "blocking_result_consumed_before_interruption", behavior: "BLOCKING", balance: 100_000, continuation: true, interrupt: true, forwarded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runLiveToolFloorCase(t, tc, measured)
		})
	}
}

// runLiveToolFloorCase drives one scenario through the production Live route
// and asserts its single settlement. Parameters: t owns the scenario, tc is
// the scenario and measured is the quota of the calling turn's receipt.
// Returns: none.
func runLiveToolFloorCase(t *testing.T, tc liveToolFloorCase, measured int64) {
	t.Helper()
	behavior, scheduling := "", ""
	if tc.behavior != "" {
		behavior = `"behavior":"` + tc.behavior + `",`
	}
	if tc.behavior == "NON_BLOCKING" {
		scheduling = `,"scheduling":"SILENT"`
	}
	setup := `{"setup":{"model":"` + liveBudgetModel + `","tools":[{"functionDeclarations":[{"name":"lookup",` + behavior + `"parameters":{"type":"OBJECT","properties":{"query":{"type":"STRING"}}}}]}]}}`
	response := liveToolResponse(`"response":{"result":"` + strings.Repeat("x", 40_000) + `"` + scheduling + `}`)
	require.True(t, json.Valid([]byte(response)), "the native function response is valid JSON")

	received := make(chan string, 1)
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: tc.balance, TokenQuota: tc.balance, Group: 1}, func(conn *websocket.Conn) error {
		if _, _, err := conn.ReadMessage(); err != nil {
			return errors.Wrap(err, "read initial input")
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"toolCall":{"functionCalls":[{"id":"call-1","name":"lookup","args":{}}]}}`)); err != nil {
			return errors.Wrap(err, "issue function call")
		}
		if tc.closeFirst {
			if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(8000, 0, 0, 12000, 0, 0)); err != nil {
				return errors.Wrap(err, "complete the calling turn")
			}
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			received <- ""
			return nil
		}
		received <- string(raw)
		if tc.continuation {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"serverContent":{"modelTurn":{"parts":[{"text":"looked up"}]}}}`)); err != nil {
				return errors.Wrap(err, "continue after the result")
			}
		}
		if tc.interrupt {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"serverContent":{"interrupted":true}}`)); err != nil {
				return errors.Wrap(err, "interrupt the calling turn")
			}
		}
		if !tc.closeFirst {
			if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(8000, 0, 0, 12000, 0, 0)); err != nil {
				return errors.Wrap(err, "complete the calling turn")
			}
		}
		if tc.laterReceipt {
			if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(60000, 0, 0, 1, 0, 0)); err != nil {
				return errors.Wrap(err, "complete a turn that starts after the result")
			}
		}
		return nil // Disconnect after the scripted receipts, without invented usage.
	})
	conn := env.connectSetup(setup)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(liveTextFrame(600))))
	_, call, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(call), "call-1")
	if tc.closeFirst {
		_, receipt, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Contains(t, string(receipt), `"turnComplete":true`)
	}
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(response)))
	reason := awaitClose(conn, 3*time.Second)
	env.finish()
	got := <-received
	require.Equal(t, tc.forwarded, got != "", "provider dispatch decision")
	if tc.forwarded {
		require.True(t, got == response, "funded result is forwarded unchanged")
	} else {
		require.Equal(t, liveQuotaCloseReason, reason)
	}

	logs := env.consumeLogs()
	require.Len(t, logs, 1)
	user, token := env.balances()
	require.Equal(t, user, token, "finite token and owner settle the same amount")
	require.GreaterOrEqual(t, user, int64(0))
	for _, rows := range logs {
		require.Len(t, rows, 1, "session settles once")
		row := rows[0]
		require.EqualValues(t, tc.balance-user, row.Quota)
		rawAudit, err := json.Marshal(row.Metadata["realtime_usage"])
		require.NoError(t, err)
		var audit realtime.AuditSnapshot
		require.NoError(t, json.Unmarshal(rawAudit, &audit))
		receipts := 1
		if tc.laterReceipt {
			receipts = 2
		}
		require.Equal(t, receipts, audit.ReceiptCount)
		require.Len(t, audit.SampleRecords, receipts)
		require.EqualValues(t, 8000, audit.SampleRecords[0].Tokens.Input)
		require.EqualValues(t, 12000, audit.SampleRecords[0].Tokens.Output)

		t.Logf("forwarded=%t charge=%d observed=%v complete=%v estimated=%v reserved=%v user=%d token=%d", got != "", row.Quota, row.Metadata["realtime_observed_quota"], row.Metadata["realtime_billing_complete"], row.Metadata[model.LogMetadataKeyEstimatedCharge], row.Metadata["realtime_budget_reserved"], user, token)
		if tc.estimated {
			want := measured + int64(math.Ceil(float64(len(response))*liveBudgetTextQuotaPerToken))
			require.EqualValues(t, want, row.Quota, "completed receipt plus the unreceipted result's input evidence is retained")
			require.GreaterOrEqual(t, int64(row.Quota), measured+int64(math.Ceil(40_000.0/4*liveBudgetTextQuotaPerToken)), "independent weak token bound is retained")
			require.LessOrEqual(t, float64(row.Quota), row.Metadata["realtime_budget_reserved"].(float64), "estimated charge never exceeds prepaid reservation")
			require.EqualValues(t, measured, row.Metadata["realtime_observed_quota"], "prior measured receipt stays authoritative")
			require.Equal(t, false, row.Metadata["realtime_billing_complete"])
			require.Equal(t, true, row.Metadata[model.LogMetadataKeyEstimatedCharge])
			continue
		}
		want := measured
		if tc.laterReceipt {
			want = int64(math.Ceil(float64(measured) + liveReceiptQuota(60000, 0, 0, 1, 0, 0)))
		}
		require.EqualValues(t, want, row.Quota, "complete receipts refund unused headroom")
		require.Equal(t, true, row.Metadata["realtime_billing_complete"])
		require.Nil(t, row.Metadata[model.LogMetadataKeyEstimatedCharge])
	}
}
