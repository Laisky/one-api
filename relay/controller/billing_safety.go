package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/billing"
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
			return false
		}
	}

	billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, tokenID)
	return true
}
