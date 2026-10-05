package controller

import (
	"context"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

const responseSettlementKey = "billing_response_settlement_started"

// BillingAllowsRetry reports whether another upstream attempt can be made
// without replaying committed output, a response submitted for settlement or an
// uncertain paid Jina call. It does not certify that settlement has committed.
func BillingAllowsRetry(c *gin.Context) bool {
	return c != nil && (c.Writer == nil || !c.Writer.Written()) &&
		!c.GetBool(responseSettlementKey) && !JinaAttemptMayHaveCost(c)
}

// markResponseSettlement records an errored response with usage before
// asynchronous settlement. The outer retry loop must not replay it.
func markResponseSettlement(c *gin.Context, usage *relaymodel.Usage, apiErr *relaymodel.ErrorWithStatusCode) {
	if apiErr != nil && usage != nil {
		c.Set(responseSettlementKey, true)
	}
}

// hasBillableUsage reports whether usage contains any measurable charge bucket,
// including cache-only and tool-only requests with zero ordinary token counts.
func hasBillableUsage(usage *relaymodel.Usage) bool {
	if usage == nil {
		return false
	}
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.ToolsCost > 0 ||
		usage.CacheWrite5mTokens > 0 || usage.CacheWrite1hTokens > 0 || usage.CacheWriteTokens > 0 || usage.Realtime != nil {
		return true
	}
	if d := usage.PromptTokensDetails; d != nil {
		if d.CachedTokens > 0 || d.AudioTokens > 0 || d.TextTokens > 0 || d.ImageTokens > 0 ||
			d.VideoTokens > 0 || d.DocumentTokens > 0 || d.ImageCount > 0 || d.AudioSeconds > 0 || d.VideoFrames > 0 || d.DocumentPages > 0 {
			return true
		}
	}
	if d := usage.CompletionTokensDetails; d != nil {
		return d.AudioTokens > 0 || d.TextTokens > 0 || d.ReasoningTokens > 0
	}
	return false
}

// unpricedUsageEstimateReason marks a settlement that retained its reservation
// because the tariff could not authoritatively price the receipt.
const unpricedUsageEstimateReason = "unpriced_usage_retained_reservation"

// retainUnpricedUsage flags a receipt the tariff could not fully price so the
// caller's estimate floor keeps at least the reservation instead of trusting a
// partial or zero total. Parameters: ctx carries the request logger, usage is the
// settled receipt and result is its computed price. Returns: true when flagged.
func retainUnpricedUsage(ctx context.Context, usage *relaymodel.Usage, result quotautil.ComputeResult) bool {
	if usage == nil || !result.UnpricedUsage {
		return false
	}
	if usage.BillingEstimateReason == "" {
		usage.BillingEstimateReason = unpricedUsageEstimateReason
	}
	gmw.GetLogger(ctx).Warn("settlement could not price the receipt; retaining the reservation",
		zap.Int64("priced_quota", result.TotalQuota),
		zap.Strings("billing_issues", result.BillingIssues))
	return true
}

// billingEstimateMetadata adds an explicit estimation label without mutating an
// existing metadata map. An empty reason preserves measured-usage metadata.
func billingEstimateMetadata(metadata model.LogMetadata, reason string) model.LogMetadata {
	if reason == "" {
		return metadata
	}
	out := make(model.LogMetadata, len(metadata)+2)
	for key, value := range metadata {
		out[key] = value
	}
	out["billing_estimated"] = true
	out[model.LogMetadataKeyEstimatedCharge] = true
	out["billing_estimate_reason"] = reason
	return out
}

// recordZeroCostAfterFailure avoids publishing a zero bill for retained Jina
// work or a failed refund. Other providers retain their existing error semantics.
func recordZeroCostAfterFailure(c *gin.Context, userID int, requestID string) error {
	if JinaAttemptMayHaveCost(c) || c.GetBool(responseSettlementKey) {
		return nil
	}
	if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, 0); err != nil {
		return errors.Wrap(err, "record zero cost after confirmed failure")
	}
	return nil
}
