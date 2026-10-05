package controller

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/state"
)

// assertNoIDWSLedger reads owner, finite-token, request-cost and log records before requiring exact final settlement and provenance.
func assertNoIDWSLedger(t *testing.T, fixture *terminalWSFixture, want int64, estimated bool) {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	var costs []model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", fixture.id).Find(&costs).Error)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", fixture.id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	t.Logf("WS_NO_ID_LEDGER creates=%d user_debit=%d token_debit=%d token_used=%d cost_rows=%d log_rows=%d expected=%d", fixture.creates.Load(), terminalWSBalance-reloadUserQuota(t), terminalWSBalance-token.RemainQuota, token.UsedQuota, len(costs), len(logs), want)
	for _, cost := range costs {
		t.Logf("WS_NO_ID_COST quota=%d", cost.Quota)
	}
	for _, entry := range logs {
		t.Logf("WS_NO_ID_LOG type=%d quota=%d input=%d output=%d estimated=%v reason=%v", entry.Type, entry.Quota, entry.PromptTokens, entry.CompletionTokens, entry.Metadata["billing_estimated"], entry.Metadata["billing_estimate_reason"])
	}
	require.Equal(t, terminalWSBalance-want, reloadUserQuota(t))
	require.Equal(t, terminalWSBalance-want, token.RemainQuota)
	require.Equal(t, want, token.UsedQuota)
	if want == 0 {
		require.Empty(t, costs)
		require.Len(t, logs, 1, "pre-dispatch rejection records one zero-charge refund audit row")
		require.Equal(t, model.LogTypeConsume, logs[0].Type)
		require.Zero(t, logs[0].Quota)
		require.Empty(t, logs[0].Metadata["billing_estimate_reason"])
		return
	}
	require.Len(t, costs, 1, "dispatched exchange must finalize one request-cost row")
	require.Equal(t, want, costs[0].Quota)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	require.EqualValues(t, want, logs[0].Quota)
	if estimated {
		require.Equal(t, true, logs[0].Metadata["billing_estimated"])
		require.Equal(t, "response_stream_incomplete_or_missing_receipt", logs[0].Metadata["billing_estimate_reason"])
	} else {
		require.Empty(t, logs[0].Metadata["billing_estimate_reason"])
	}
}

// TestSecurityResponseWSNoIDClosureHTTP checks an executed exchange without any response ID, a rejected first create, and a measured terminal control through real sockets and SQLite.
func TestSecurityResponseWSNoIDClosureHTTP(t *testing.T) {
	for _, scenario := range []string{"dispatched_without_response_id", "terminal_then_unidentified_creation", "rejected_before_dispatch", "valid_terminal_receipt"} {
		t.Run(scenario, func(t *testing.T) {
			var events [][]byte
			if scenario == "valid_terminal_receipt" || scenario == "terminal_then_unidentified_creation" {
				events = [][]byte{terminalWSEvent(t, "response.completed", "completed", terminalWSFirstID, 100)}
			}
			var fixture *terminalWSFixture
			if scenario == "terminal_then_unidentified_creation" {
				fixture = startTerminalWSFixture(t, events, false, true, [][]byte{})
			} else {
				fixture = startTerminalWSFixture(t, events, true, false)
			}
			if scenario == "rejected_before_dispatch" {
				require.NoError(t, fixture.client.WriteJSON(map[string]any{"type": "response.create", "model": "alias", "previous_response_id": "resp_unowned_synthetic", "input": "synthetic fixture"}))
			} else {
				sendTerminalWSCreate(t, fixture, "")
			}
			for _, event := range events {
				_, observed, err := fixture.client.ReadMessage()
				require.NoError(t, err)
				require.JSONEq(t, string(event), string(observed))
			}
			if scenario == "terminal_then_unidentified_creation" {
				sendTerminalWSCreate(t, fixture, terminalWSFirstID)
			}
			_, _, closeErr := fixture.client.ReadMessage()
			if scenario == "rejected_before_dispatch" {
				require.True(t, websocket.IsCloseError(closeErr, websocket.ClosePolicyViolation))
				select {
				case result := <-fixture.done:
					require.NotNil(t, result.err)
					require.Equal(t, "response_websocket_request_rejected", result.err.Code)
				case <-time.After(5 * time.Second):
					t.Fatal("controller did not finish pre-dispatch rejection")
				}
				select {
				case providerErr := <-fixture.providerDone:
					require.NoError(t, providerErr)
				case <-time.After(5 * time.Second):
					t.Fatal("provider did not finish pre-dispatch rejection")
				}
				drainCriticalTasks(t)
				require.Zero(t, fixture.creates.Load())
				assertNoIDWSLedger(t, fixture, 0, false)
				return
			}
			require.True(t, websocket.IsCloseError(closeErr, websocket.CloseNormalClosure))
			finishTerminalWSFixture(t, fixture, false)
			wantCreates := 1
			if scenario == "terminal_then_unidentified_creation" {
				wantCreates = 2
			}
			require.EqualValues(t, wantCreates, fixture.creates.Load())
			if scenario == "valid_terminal_receipt" {
				assertNoIDWSLedger(t, fixture, 103, false)
			} else {
				bindingID := terminalWSFirstID
				if scenario == "terminal_then_unidentified_creation" {
					bindingID = terminalWSSecondID
				}
				_, bindingErr := fixture.store.GetResponseBinding(context.Background(), state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}, bindingID)
				require.ErrorIs(t, bindingErr, state.ErrNotFound)
				assertNoIDWSLedger(t, fixture, terminalWSHold, true)
			}
		})
	}
}
