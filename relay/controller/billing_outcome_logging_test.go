package controller

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// billingOutcomeLogFixture seeds a physical hold in isolated SQLite and returns
// an observer logger with no production identity or payload fields.
func billingOutcomeLogFixture(t *testing.T, forwarded bool) (*gin.Context, int, *observer.ObservedLogs, context.Context) {
	t.Helper()
	c, _, provisionalID := incidentBodyResetBillingFixture(t, forwarded)
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("billing-outcome-fixture"),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	gmw.SetLogger(c, lg)
	return c, provisionalID, logs, gmw.SetLogger(context.Background(), lg)
}

// billingOutcomeRecords selects structured accounting events without depending
// on the pre-existing warning or audit message text.
func billingOutcomeRecords(logs *observer.ObservedLogs) []observer.LoggedEntry {
	var records []observer.LoggedEntry
	for _, entry := range logs.All() {
		if entry.ContextMap()["log_stage"] == "accounting" {
			records = append(records, entry)
		}
	}
	return records
}

// billingAppliedRecords selects only claimed refund completion events.
func billingAppliedRecords(logs *observer.ObservedLogs) []observer.LoggedEntry {
	var records []observer.LoggedEntry
	for _, entry := range billingOutcomeRecords(logs) {
		if entry.ContextMap()["accounting_outcome"] == "refund_balance_applied" {
			records = append(records, entry)
		}
	}
	return records
}

// requireBillingOutcome checks the bounded outcome schema and avoids stronger
// claims about exactly-once processing or independently verified row durability.
func requireBillingOutcome(t *testing.T, logs *observer.ObservedLogs, outcome, balance, ledger string) observer.LoggedEntry {
	t.Helper()
	records := billingOutcomeRecords(logs)
	require.Len(t, records, 1)
	fields := records[0].ContextMap()
	require.Equal(t, outcome, fields["accounting_outcome"])
	require.Equal(t, balance, fields["balance_update"])
	require.Equal(t, ledger, fields["ledger_reconciliation"])
	require.NotContains(t, fields, "exactly_once")
	require.NotContains(t, fields, "durable_balance")
	return records[0]
}

// billingOutcomeBalances reads the synthetic owner and finite-token balances,
// independently of lifecycle markers and the logging assertions.
func billingOutcomeBalances(t *testing.T, expected int64) {
	t.Helper()
	var owner model.User
	var token model.Token
	require.NoError(t, model.DB.First(&owner, 93001).Error)
	require.NoError(t, model.DB.First(&token, 93002).Error)
	require.Equal(t, expected, owner.Quota)
	require.Equal(t, expected, token.RemainQuota)
}

// TestBillingRefundOutcomeLogging exercises real refund writes, reconciliation,
// rollback and cancellation rather than asserting helper-generated fields alone.
func TestBillingRefundOutcomeLogging(t *testing.T) {
	for _, tc := range []struct {
		name, faultTable, ledger          string
		skip, canceled, noLog, missingLog bool
		wantRefund                        bool
		outcome, balance                  string
	}{
		{name: "ledger_update_returned_no_error", ledger: "update_returned_no_error", wantRefund: true, outcome: "refund_balance_applied", balance: "call_returned_no_error"},
		{name: "ledger_skipped", ledger: "skipped", noLog: true, wantRefund: true, outcome: "refund_balance_applied", balance: "call_returned_no_error"},
		{name: "missing_ledger_row_is_not_verified", ledger: "update_returned_no_error", missingLog: true, wantRefund: true, outcome: "refund_balance_applied", balance: "call_returned_no_error"},
		{name: "ledger_write_failed", faultTable: "logs", ledger: "failed", wantRefund: true, outcome: "refund_balance_applied", balance: "call_returned_no_error"},
		{name: "balance_write_failed", faultTable: "users", ledger: "not_attempted", outcome: "refund_balance_failed", balance: "failed"},
		{name: "canceled_context", canceled: true, ledger: "not_attempted", outcome: "refund_balance_failed", balance: "failed"},
		{name: "possibly_forwarded_retains_hold", skip: true, ledger: "not_attempted", outcome: "reservation_retained", balance: "not_attempted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, provisionalID, logs, ctx := billingOutcomeLogFixture(t, tc.skip)
			snapshot := newConservativeRefundSnapshot(c, incidentBodyResetBillingHold, c.GetInt(ctxkey.TokenId), "do_request_failed")
			if tc.noLog {
				snapshot.provisionalLogID = 0
			}
			if tc.missingLog {
				snapshot.provisionalLogID = provisionalID + 1000
			}
			if tc.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			var successBeforeWrite atomic.Bool
			const callback = "billing_outcome_control"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil {
					table := tx.Statement.Schema.Table
					if (table == "users" || table == "tokens") && len(billingAppliedRecords(logs)) > 0 {
						successBeforeWrite.Store(true)
					}
					if table == tc.faultTable {
						tx.AddError(errors.New("controlled accounting write failure"))
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })

			require.Equal(t, tc.wantRefund, snapshot.refund(ctx))
			require.False(t, successBeforeWrite.Load(), "refund success cannot be logged before its balance write")
			expected := incidentBodyResetBillingBalance - incidentBodyResetBillingHold
			if tc.wantRefund {
				expected = incidentBodyResetBillingBalance
			}
			billingOutcomeBalances(t, expected)

			var row model.Log
			require.NoError(t, model.LOG_DB.First(&row, provisionalID).Error)
			if tc.wantRefund && !tc.noLog && !tc.missingLog && tc.faultTable != "logs" {
				require.Equal(t, model.LogTypeConsume, row.Type)
				require.Zero(t, row.Quota)
			} else {
				require.Equal(t, model.LogTypeProvisional, row.Type)
				require.EqualValues(t, incidentBodyResetBillingHold, row.Quota)
			}
			event := requireBillingOutcome(t, logs, tc.outcome, tc.balance, tc.ledger)
			if tc.wantRefund {
				require.Equal(t, zapcore.InfoLevel, event.Level)
				fields := event.ContextMap()
				for _, key := range []string{"request_id", "user_id", "token_id", "provisional_log_id", "payload"} {
					require.NotContains(t, fields, key, "new refund completion fields must not repeat raw identities or payloads")
				}
			} else {
				require.Empty(t, billingAppliedRecords(logs))
			}
		})
	}
}

// TestBillingRefundNonpositiveOutcomeControl ensures a no-op cannot claim an
// applied balance change and does not add noisy completion logs.
func TestBillingRefundNonpositiveOutcomeControl(t *testing.T) {
	for _, quota := range []int64{0, -1} {
		t.Run(map[bool]string{true: "zero", false: "negative"}[quota == 0], func(t *testing.T) {
			c, _, logs, ctx := billingOutcomeLogFixture(t, false)
			snapshot := newConservativeRefundSnapshot(c, quota, c.GetInt(ctxkey.TokenId), "no_refund_needed")
			require.False(t, snapshot.refund(ctx))
			billingOutcomeBalances(t, incidentBodyResetBillingBalance-incidentBodyResetBillingHold)
			require.Empty(t, billingOutcomeRecords(logs))
		})
	}
}

// TestBillingRetryRefundOutcomeLogging observes the actual detached retry refund;
// repeated lifecycle reset must neither duplicate its debit reversal nor its log.
func TestBillingRetryRefundOutcomeLogging(t *testing.T) {
	c, provisionalID, logs, _ := billingOutcomeLogFixture(t, true)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	ResetPerAttemptBillingForRetry(context.Background(), c)
	ResetPerAttemptBillingForRetry(context.Background(), c)
	incidentBodyResetBillingDrain(t)
	billingOutcomeBalances(t, incidentBodyResetBillingBalance)
	var row model.Log
	require.NoError(t, model.LOG_DB.First(&row, provisionalID).Error)
	require.Equal(t, model.LogTypeConsume, row.Type)
	require.Zero(t, row.Quota)
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	event := requireBillingOutcome(t, logs, "refund_balance_applied", "call_returned_no_error", "update_returned_no_error")
	require.Equal(t, zapcore.InfoLevel, event.Level)
}

// TestBillingAuditRetainedOutcomeLogging makes unresolved forwarding visible
// without asserting that the lifecycle flag implies any balance or ledger write.
func TestBillingAuditRetainedOutcomeLogging(t *testing.T) {
	c, provisionalID, logs, _ := billingOutcomeLogFixture(t, true)
	require.False(t, c.GetBool(ctxkey.BillingReconciled))
	billingAuditSafetyNet(c)
	incidentBodyResetBillingDrain(t)
	billingOutcomeBalances(t, incidentBodyResetBillingBalance-incidentBodyResetBillingHold)
	var row model.Log
	require.NoError(t, model.LOG_DB.First(&row, provisionalID).Error)
	require.Equal(t, model.LogTypeProvisional, row.Type)
	event := requireBillingOutcome(t, logs, "reservation_retained", "not_attempted", "not_attempted")
	require.Equal(t, zapcore.ErrorLevel, event.Level)
	require.Empty(t, billingAppliedRecords(logs))
}
