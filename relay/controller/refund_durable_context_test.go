package controller

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestPR421PendingRefundOutlivesRetryReset verifies that exhausting the immediate
// retry budget cannot erase a failed refund. Its original intent survives Gin
// reset/cancellation and is recovered exactly once without that request context.
func TestPR421PendingRefundOutlivesRetryReset(t *testing.T) {
	for _, task := range []string{"audio", "video"} {
		t.Run(task, func(t *testing.T) {
			const initial, hold = int64(50000), int64(1234)
			start := billingAccountingSetup(t, initial)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
				Updates(map[string]any{"unlimited_quota": false, "remain_quota": start, "used_quota": 0}).Error)
			preConsume(t, hold)
			var outage atomic.Bool
			outage.Store(true)
			const hook = "pr421:persistent_refund_outage"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == "users" && outage.Load() {
					tx.AddError(errors.New("injected persistent refund credit outage"))
				}
			}))
			t.Cleanup(func() {
				drainBilling(t)
				require.NoError(t, model.DB.Callback().Update().Remove(hook))
			})
			requestID := "pending-after-reset-" + task
			c, cancel := newRollbackContext(t, requestID, true)
			defer cancel()
			markPreConsumed(c, hold)
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
			markBillingReconciled(c)
			goRollbackPreConsumed(c, task+"PendingRefund", fallbackTokenID, hold, nil, nil)
			ResetPerAttemptBillingForRetry(context.Background(), c)
			cancel()
			c.Set(ctxkey.Id, -999)
			c.Set(ctxkey.TokenId, -999)
			drainBilling(t)
			require.Equal(t, start-hold, reloadUserQuota(t))
			var intent model.QuotaRefund
			require.NoError(t, model.DB.Where("request_id = ?", requestID).Take(&intent).Error)
			require.Equal(t, model.QuotaRefundPending, intent.Status)
			require.Equal(t, fallbackUserID, intent.UserID)
			require.Equal(t, fallbackTokenID, intent.TokenID)
			require.Equal(t, hold, intent.Amount)
			outage.Store(false)
			require.NoError(t, model.RecoverQuotaRefunds(context.Background()))
			require.NoError(t, model.RecoverQuotaRefunds(context.Background()))
			require.Equal(t, start, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, start, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
		})
	}
}
