package controller

import (
	"encoding/json"
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

// TestConservativeRefundDurableEvidence separates actual balance credit from
// ledger reconciliation after each independently injected durable write failure.
// Parameters: t owns serial disposable SQLite fixtures. Returns: none.
func TestConservativeRefundDurableEvidence(t *testing.T) {
	for _, fault := range []string{"none", "users", "tokens", "logs"} {
		for _, cancelled := range []bool{false, true} {
			name := fault + "/active"
			if cancelled {
				name = fault + "/cancelled"
			}
			t.Run(name, func(t *testing.T) {
				const initial, hold = int64(10_000), int64(1234)
				oldMode := gin.Mode()
				t.Cleanup(func() { gin.SetMode(oldMode) })
				start := billingAccountingSetup(t, initial)
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
					Updates(map[string]any{"unlimited_quota": false, "remain_quota": start, "used_quota": 0}).Error)
				preConsume(t, hold)
				c, cancel := newRefundContext(t, false, true)
				t.Cleanup(cancel)
				requestID := c.GetString(ctxkey.RequestId)
				const privateContent = "synthetic-private-refund-evidence-body"
				row := model.Log{UserId: fallbackUserID, RequestId: requestID,
					Type: model.LogTypeProvisional, Quota: int(hold), Content: privateContent,
					Metadata: model.LogMetadata{model.LogMetadataKeyProvisional: true}}
				require.NoError(t, model.LOG_DB.Create(&row).Error)
				t.Cleanup(func() {
					drainBilling(t)
					require.NoError(t, model.LOG_DB.Delete(&model.Log{}, row.Id).Error)
				})
				core, logs := observer.New(zapcore.DebugLevel)
				logger, err := glog.NewWithName("refund-evidence", glog.LevelDebug,
					zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
				require.NoError(t, err)
				gmw.SetLogger(c, logger)
				var injected atomic.Int32
				var cancelledWrite atomic.Bool
				const hook = "refund-evidence:durable-write-fault"
				if fault != "none" {
					store := model.DB
					if fault == "logs" {
						store = model.LOG_DB
					}
					require.NoError(t, store.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
						if tx.Statement.Table != fault {
							return
						}
						injected.Add(1)
						if tx.Statement.Context.Err() != nil {
							cancelledWrite.Store(true)
						}
						tx.AddError(errors.New("synthetic durable write failure"))
					}))
					t.Cleanup(func() {
						drainBilling(t)
						require.NoError(t, store.Callback().Update().Remove(hook))
					})
				}
				markPreConsumed(c, hold)
				c.Set(ctxkey.ProvisionalLogId, row.Id)
				if cancelled {
					cancel()
				}
				scheduleConservativeRefund(c, hold, fallbackTokenID, "do_request_failed")
				require.True(t, c.GetBool(ctxkey.BillingReconciled), "ownership is marked before asynchronous durable work")
				billingAuditSafetyNet(c)
				drainBilling(t)

				var user model.User
				var token model.Token
				var ledger model.Log
				require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.NoError(t, model.LOG_DB.First(&ledger, row.Id).Error)
				refundFailed := fault == "users" || fault == "tokens"
				expectedDebit := int64(0)
				if refundFailed {
					expectedDebit = hold
				}
				require.Equal(t, start-expectedDebit, user.Quota, "token failure must roll back the earlier user credit")
				require.Equal(t, user.Quota, token.RemainQuota)
				require.Equal(t, expectedDebit, token.UsedQuota, "neither failure nor the deferred audit may duplicate credit")
				if fault == "none" {
					require.Equal(t, model.LogTypeConsume, ledger.Type)
					require.Zero(t, ledger.Quota)
					require.NotEqual(t, true, ledger.Metadata[model.LogMetadataKeyProvisional])
				} else {
					require.Positive(t, injected.Load(), "the selected real database write must reach the injected failure")
					require.False(t, cancelledWrite.Load(), "refund and reconciliation must outlive client cancellation")
					require.Equal(t, model.LogTypeProvisional, ledger.Type)
					require.EqualValues(t, hold, ledger.Quota, "failed reconciliation must remain distinguishable from confirmed zero cost")
					require.Equal(t, true, ledger.Metadata[model.LogMetadataKeyProvisional])
				}
				refundAudit := logs.FilterMessage("CRITICAL BILLING AUDIT: conservative refund failed to return pre-consumed quota; billing was already marked reconciled, manual reconciliation may be required").All()
				if refundFailed {
					require.Len(t, refundAudit, 1)
					require.Equal(t, requestID, refundAudit[0].ContextMap()["request_id"])
					require.EqualValues(t, hold, refundAudit[0].ContextMap()["pre_consumed_quota"])
				} else {
					require.Empty(t, refundAudit)
				}
				ledgerAudit := logs.FilterMessage("failed to reconcile provisional log on refund").All()
				if fault == "logs" {
					require.Len(t, ledgerAudit, 1)
					require.EqualValues(t, row.Id, ledgerAudit[0].ContextMap()["provisional_log_id"])
				} else {
					require.Empty(t, ledgerAudit)
				}
				require.Empty(t, logs.FilterMessage("CRITICAL BILLING AUDIT: pre-consumed quota was not reconciled (no post-billing or refund)").All(), "claimed refund must not race the emergency refund")
				for _, entry := range logs.All() {
					encoded, err := json.Marshal(entry.ContextMap())
					require.NoError(t, err)
					require.NotContains(t, entry.Message+string(encoded), privateContent)
				}
			})
		}
	}
}
