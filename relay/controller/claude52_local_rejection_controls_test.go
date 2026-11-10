package controller

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestClaude52LocalSettlementOwnership verifies cancellation and refund failure without weakening paid-attempt protection.
func TestClaude52LocalSettlementOwnership(t *testing.T) {
	for _, name := range []string{"canceled", "refund_failure", "possibly_forwarded", "response_settlement"} {
		t.Run(name, func(t *testing.T) {
			c, info, provisionalID := incidentBodyResetBillingFixture(t, name == "possibly_forwarded")
			require.NoError(t, model.DB.AutoMigrate(&model.QuotaRefund{}, &model.UserRequestCost{}))
			requestID := c.GetString(ctxkey.RequestId)
			require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(info.UserId, requestID, incidentBodyResetBillingHold))
			if name == "canceled" {
				ctx, cancel := context.WithCancel(c.Request.Context())
				c.Request = c.Request.WithContext(ctx)
				cancel()
			}
			if name == "response_settlement" {
				c.Set(responseSettlementKey, true)
			}
			if name == "refund_failure" {
				const hook = "test:claude52_local_refund_failure"
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
					if tx.Statement.Table == "quota_refunds" {
						tx.AddError(errors.New("synthetic durable refund outage"))
					}
				}))
				t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
			}
			settleUnforwardedClaudeReservation(c, incidentBodyResetBillingHold, info.TokenId)
			settleUnforwardedClaudeReservation(c, incidentBodyResetBillingHold, info.TokenId)
			paidOrSubmitted := name == "possibly_forwarded" || name == "response_settlement"
			if paidOrSubmitted {
				require.False(t, c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed))
				require.False(t, c.GetBool(ctxkey.BillingReconciled))
			} else {
				require.True(t, c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed))
				require.True(t, c.GetBool(ctxkey.BillingReconciled))
				billingAuditSafetyNet(c)
			}
			wantDebit := incidentBodyResetBillingHold
			if name == "canceled" {
				wantDebit = 0
			}
			var owner model.User
			var token model.Token
			require.NoError(t, model.DB.First(&owner, info.UserId).Error)
			require.NoError(t, model.DB.First(&token, info.TokenId).Error)
			require.Equal(t, incidentBodyResetBillingBalance-wantDebit, owner.Quota)
			require.Equal(t, owner.Quota, token.RemainQuota)
			require.Equal(t, wantDebit, token.UsedQuota)
			require.Equal(t, wantDebit, requestCostQuota(t, requestID))
			var entry model.Log
			require.NoError(t, model.LOG_DB.First(&entry, provisionalID).Error)
			require.EqualValues(t, wantDebit, entry.Quota)
			var intents []model.QuotaRefund
			require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&intents).Error)
			if paidOrSubmitted {
				require.Empty(t, intents)
				return
			}
			require.Len(t, intents, 1, "duplicate cleanup must share one owned intent")
			if name == "refund_failure" {
				require.Equal(t, model.QuotaRefundPending, intents[0].Status)
				require.False(t, BillingAllowsRetry(c), "failed credit cannot allow replay or fabricate zero cost")
			} else {
				require.Equal(t, model.QuotaRefundCompleted, intents[0].Status)
				require.True(t, BillingAllowsRetry(c))
			}
		})
	}
}
