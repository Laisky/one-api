package controller

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	awsutils "github.com/Laisky/one-api/relay/adaptor/aws/utils"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// refundClaudeAdmission retains its historical name for compatibility and handles
// only private proof of a Claude JSON or non-retried AWS SDK admission rejection.
// It completes an owned durable refund before allowing replay, retaining a pending
// recovery intent and blocking replay if the database cannot confirm the credit.
// The boolean reports whether this function owns the error's refund disposition.
func refundClaudeAdmission(c *gin.Context, failure *relaymodel.ErrorWithStatusCode, amount int64, tokenID int) bool {
	if c == nil || (!anthropic.IsAdmissionRejection(failure) && !awsutils.IsAdmissionRejection(failure)) {
		return false
	}
	reason := "claude_admission_rejected"
	if awsutils.IsAdmissionRejection(failure) {
		reason = "aws_admission_rejected"
	}
	return refundAdmissionReservation(c, amount, tokenID, reason)
}

// refundAdmissionReservation owns a proven non-billable reservation and reconciles its balance,
// provisional log and request cost before replay. Callers must establish rejection proof first.
func refundAdmissionReservation(c *gin.Context, amount int64, tokenID int, reason string) bool {
	if c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed) {
		return true
	}
	c.Set(ctxkey.PreConsumedQuotaRefundClaimed, true)
	markBillingReconciled(c)
	logger := gmw.GetLogger(c)
	timeout := time.Duration(config.BillingTimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(relayctx.Detach(c), timeout)
	defer cancel()
	intent := model.QuotaRefund{
		ID: uuid.NewString(), UserID: c.GetInt(ctxkey.Id), TokenID: tokenID,
		Amount: amount, RequestID: c.GetString(ctxkey.RequestId), Reason: reason,
	}
	if err := model.PrepareQuotaRefundMetadata(ctx, &intent, c.GetInt(ctxkey.ProvisionalLogId)); err != nil {
		logger.Warn("admission refund cost target unavailable; balance refund remains durable",
			zap.String("refund_id", intent.ID), zap.String("request_id", intent.RequestID), zap.Error(err))
	}
	if persisted, err := model.RefundCurrentAttemptWithRecovery(ctx, intent); err != nil {
		c.Set(responseSettlementKey, true)
		logger.Error("CRITICAL BILLING AUDIT: admission refund requires recovery",
			zap.String("refund_id", intent.ID), zap.Bool("intent_persisted", persisted),
			zap.Int("user_id", intent.UserID), zap.Int("token_id", tokenID),
			zap.Int64("quota", amount), zap.String("request_id", intent.RequestID), zap.Error(err))
	}
	return true
}
