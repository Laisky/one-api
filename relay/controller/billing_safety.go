package controller

import (
	"context"
	"fmt"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// shouldSkipPreConsumedRefund reports whether a refund should be skipped because
// the request may already have been forwarded upstream.
//
// Parameters:
//   - c: request context containing forwarding marker.
//
// Returns:
//   - bool: true when conservative policy requires skipping refund.
func shouldSkipPreConsumedRefund(c *gin.Context) bool {
	if c == nil {
		return false
	}
	forwardedAny, exists := c.Get(ctxkey.UpstreamRequestPossiblyForwarded)
	if !exists {
		return false
	}
	forwarded, ok := forwardedAny.(bool)
	return ok && forwarded
}

// userVisibleModelName returns the origin/public model name for user-facing responses and logs.
func userVisibleModelName(meta *metalib.Meta, fallback string) string {
	if meta != nil {
		if origin := strings.TrimSpace(meta.OriginModelName); origin != "" {
			return origin
		}
	}
	return strings.TrimSpace(fallback)
}

// returnPreConsumedQuotaConservative refunds pre-consumed quota only when the request
// has not potentially been forwarded upstream.
//
// Parameters:
//   - ctx: execution context for quota refund operations.
//   - c: request context carrying forwarding marker and logger.
//   - preConsumedQuota: amount to refund.
//   - tokenID: token identifier used for quota accounting.
//   - reason: short reason label for logs.
//
// Returns:
//   - bool: true when refund was executed, false when skipped for no-underbilling safety.
func returnPreConsumedQuotaConservative(
	ctx context.Context,
	c *gin.Context,
	preConsumedQuota int64,
	tokenID int,
	reason string,
) bool {
	if preConsumedQuota <= 0 {
		return false
	}

	if c != nil {
		if shouldSkipPreConsumedRefund(c) {
			gmw.GetLogger(c).Warn("skip pre-consumed refund to prevent underbilling",
				zap.Int64("pre_consumed_quota", preConsumedQuota),
				zap.Int("token_id", tokenID),
				zap.String("reason", reason),
			)
			// Even though we skip refund, mark reconciled so the safety net
			// doesn't try to refund again (this is an intentional no-refund).
			markBillingReconciled(c)
			return false
		}
		markBillingReconciled(c)

		// Reconcile provisional log to 0 so it doesn't appear as a duplicate entry
		if provLogID := c.GetInt(ctxkey.ProvisionalLogId); provLogID > 0 {
			if err := model.ReconcileConsumeLog(ctx, provLogID, 0,
				fmt.Sprintf("refunded: %s", reason), 0, 0, 0, nil); err != nil {
				gmw.GetLogger(c).Warn("failed to reconcile provisional log on refund",
					zap.Error(err), zap.Int("provisional_log_id", provLogID))
			}
		}
	}

	billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, tokenID)
	return true
}

// markPreConsumed records the pre-consumed quota amount in the gin context
// for the billing audit safety net.
func markPreConsumed(c *gin.Context, amount int64) {
	c.Set(ctxkey.PreConsumedQuotaAmount, amount)
}

// markBillingReconciled marks that post-billing or refund has been completed,
// clearing the audit safety net flag.
func markBillingReconciled(c *gin.Context) {
	c.Set(ctxkey.BillingReconciled, true)
}

// billingAuditSafetyNet should be deferred at the start of each relay handler
// (after pre-consume). It detects cases where pre-consumed quota was never
// reconciled (post-billed or refunded) and logs a CRITICAL warning for audit.
// If the request was NOT forwarded upstream, it also attempts to refund the quota.
func billingAuditSafetyNet(c *gin.Context) {
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	if reconciled != nil {
		if r, ok := reconciled.(bool); ok && r {
			return
		}
	}

	preConsumedAny, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	if !exists {
		return
	}
	preConsumed, ok := preConsumedAny.(int64)
	if !ok || preConsumed <= 0 {
		return
	}

	lg := gmw.GetLogger(c)
	userId := c.GetInt(ctxkey.Id)
	tokenId := c.GetInt(ctxkey.TokenId)
	requestId := c.GetString(ctxkey.RequestId)
	channelId := c.GetInt(ctxkey.ChannelId)

	lg.Error("CRITICAL BILLING AUDIT: pre-consumed quota was not reconciled (no post-billing or refund)",
		zap.Int64("pre_consumed_quota", preConsumed),
		zap.Int("user_id", userId),
		zap.Int("token_id", tokenId),
		zap.Int("channel_id", channelId),
		zap.String("request_id", requestId),
	)

	// Attempt emergency refund if the request was NOT forwarded upstream
	if !shouldSkipPreConsumedRefund(c) {
		lg.Warn("billing audit safety net: attempting emergency refund of unreconciled pre-consumed quota",
			zap.Int64("pre_consumed_quota", preConsumed),
			zap.Int("user_id", userId),
			zap.String("request_id", requestId),
		)
		graceful.GoCritical(gmw.BackgroundCtx(c), "billingAuditRefund", func(ctx context.Context) {
			billing.ReturnPreConsumedQuota(ctx, preConsumed, tokenId)
		})
	} else {
		lg.Error("CRITICAL BILLING AUDIT: cannot refund - request was possibly forwarded upstream, manual reconciliation required",
			zap.Int64("pre_consumed_quota", preConsumed),
			zap.Int("user_id", userId),
			zap.String("request_id", requestId),
		)
	}
}

// recordProvisionalLog writes a provisional consume log entry at pre-consume time.
// This ensures every quota deduction has an audit trail in the logs table,
// even if post-billing never runs. Returns the log ID for later reconciliation.
func recordProvisionalLog(c *gin.Context, meta *metalib.Meta, modelName string, estimatedQuota int64) int {
	if estimatedQuota <= 0 || meta == nil {
		return 0
	}

	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceIDFromContext(gmw.Ctx(c))

	logEntry := &model.Log{
		UserId:    meta.UserId,
		ChannelId: meta.ChannelId,
		ModelName: modelName,
		TokenName: meta.TokenName,
		IsStream:  meta.IsStream,
		RequestId: requestId,
		TraceId:   traceId,
	}

	return model.RecordProvisionalConsumeLog(gmw.Ctx(c), logEntry, estimatedQuota)
}
