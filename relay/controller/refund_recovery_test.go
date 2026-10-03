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

// TestPR421TransientRefundRecovery injects a real failed database UPDATE after
// reserving quota. Both terminal and cross-channel retry paths must eventually
// refund exactly once, even after the request is cancelled and Gin keys change.
// This test also compiles against the original review-fix commit as a negative control.
func TestPR421TransientRefundRecovery(t *testing.T) {
	for _, task := range []string{"audio", "video"} {
		for _, retry := range []bool{false, true} {
			name := task + "/terminal"
			if retry {
				name = task + "/retry"
			}
			t.Run(name, func(t *testing.T) {
				const initial, hold = int64(50000), int64(1234)
				start := billingAccountingSetup(t, initial)
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
					Updates(map[string]any{"unlimited_quota": false, "remain_quota": start, "used_quota": 0}).Error)
				preConsume(t, hold)
				var failed atomic.Bool
				const hook = "pr421:fail_first_refund"
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
					if tx.Statement.Table == "users" && failed.CompareAndSwap(false, true) {
						tx.AddError(errors.New("injected transient refund write failure"))
					}
				}))
				t.Cleanup(func() {
					drainBilling(t)
					require.NoError(t, model.DB.Callback().Update().Remove(hook))
				})
				c, cancel := newRollbackContext(t, "refund-recovery-"+name, true)
				defer cancel()
				markPreConsumed(c, hold)
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
				markBillingReconciled(c)
				gate := make(chan struct{})
				goRollbackPreConsumed(c, task+"RefundRecovery", fallbackTokenID, hold, gate, nil)
				if retry {
					ResetPerAttemptBillingForRetry(context.Background(), c)
				}
				cancel()
				c.Set(ctxkey.Id, -999)
				c.Set(ctxkey.TokenId, -999)
				c.Set(ctxkey.RequestId, "recycled-context")
				close(gate)
				drainBilling(t)
				require.True(t, failed.Load(), "the fault must reach the financial UPDATE")
				require.Equal(t, start, reloadUserQuota(t), "a failed owned refund must recover without another client request")
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, start, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
			})
		}
	}
}
