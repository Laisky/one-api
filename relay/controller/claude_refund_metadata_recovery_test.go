package controller

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestClaudeRefundMetadataRecovery replays durable recovery after independent accounting writes fail.
func TestClaudeRefundMetadataRecovery(t *testing.T) {
	oldGate := config.RefundCostRecoveryEnabled
	config.RefundCostRecoveryEnabled = true
	t.Cleanup(func() { config.RefundCostRecoveryEnabled = oldGate })
	for _, target := range []string{"logs", "user_request_costs"} {
		t.Run(target, func(t *testing.T) {
			c, info, logID := incidentBodyResetBillingFixture(t, false)
			require.NoError(t, model.DB.AutoMigrate(&model.QuotaRefund{}, &model.UserRequestCost{}))
			requestID := c.GetString(ctxkey.RequestId)
			require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(info.UserId, requestID, incidentBodyResetBillingHold))
			db := model.DB
			if target == "logs" {
				db = model.LOG_DB
			}
			hook := "test:refund_metadata_failure"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == target {
					tx.AddError(errors.New("synthetic independent metadata outage"))
				}
			}))
			require.True(t, refundAdmissionReservation(c, incidentBodyResetBillingHold, info.TokenId, "metadata_recovery_test"))
			require.NoError(t, db.Callback().Update().Remove(hook))
			var user model.User
			require.NoError(t, model.DB.First(&user, info.UserId).Error)
			require.Equal(t, incidentBodyResetBillingBalance, user.Quota, "credit must already be durable")
			require.NoError(t, model.RecoverQuotaRefunds(context.Background()))
			require.NoError(t, model.RecoverQuotaRefunds(context.Background()))
			var log model.Log
			require.NoError(t, model.LOG_DB.First(&log, logID).Error)
			require.Zero(t, log.Quota, "restart recovery must reconcile provisional log")
			require.Zero(t, requestCostQuota(t, requestID), "restart recovery must reconcile request cost")
			require.NoError(t, model.DB.First(&user, info.UserId).Error)
			require.Equal(t, incidentBodyResetBillingBalance, user.Quota, "recovery must not credit again")
		})
	}
}

// TestClaudeRefundSnapshotFailure preserves balance/log recovery without inventing a cost target.
func TestClaudeRefundSnapshotFailure(t *testing.T) {
	c, info, logID := incidentBodyResetBillingFixture(t, false)
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaRefund{}, &model.UserRequestCost{}))
	requestID := c.GetString(ctxkey.RequestId)
	require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(info.UserId, requestID, incidentBodyResetBillingHold))
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("metadata:snapshot_outage", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_request_costs" {
			tx.AddError(errors.New("synthetic snapshot read outage"))
		}
	}))
	require.True(t, refundAdmissionReservation(c, incidentBodyResetBillingHold, info.TokenId, "snapshot_failure"))
	require.NoError(t, model.DB.Callback().Query().Remove("metadata:snapshot_outage"))
	var user model.User
	require.NoError(t, model.DB.First(&user, info.UserId).Error)
	require.Equal(t, incidentBodyResetBillingBalance, user.Quota, "metadata reads must not prevent durable credit")
	var entry model.Log
	require.NoError(t, model.LOG_DB.First(&entry, logID).Error)
	require.Zero(t, entry.Quota)
	var intents []model.QuotaRefund
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&intents).Error)
	require.Len(t, intents, 1)
	require.Equal(t, model.QuotaRefundCompleted, intents[0].Status)
	require.True(t, intents[0].CostSnapshotUnavailable)
	require.False(t, intents[0].MetadataCompleted)
	require.Zero(t, intents[0].CostRecordID)
	oldGate := config.RefundCostRecoveryEnabled
	config.RefundCostRecoveryEnabled = true
	defer func() { config.RefundCostRecoveryEnabled = oldGate }()
	require.NoError(t, model.RecoverQuotaRefunds(context.Background()), "manual-only snapshot must be excluded, never inferred")
	require.Equal(t, incidentBodyResetBillingHold, requestCostQuota(t, requestID))
	require.False(t, BillingAllowsRetry(c))
}
