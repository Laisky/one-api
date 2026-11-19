package controller

import (
	"context"
	"strings"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/tooling"
)

// upstreamHTTPErrorReason labels refunds triggered by an explicit provider error
// status; generation admissions treat it as a definitive, retryable failure.
const upstreamHTTPErrorReason = "upstream_http_error"

// settleRetainedRequestAdmission labels an uncertain Cohere or admitted generation attempt and
// reconciles its existing hold without another debit. Cohere's explicit
// pre-inference rejection statuses are handled separately. Generic relay retry
// policy is deliberately unchanged by the atomic-admission repair; Jina keeps
// its existing provider lifecycle. All asynchronous inputs are value snapshots.
func settleRetainedRequestAdmission(c *gin.Context, amount int64, tokenID int, reason string) bool {
	if c == nil || amount <= 0 || c.GetBool(ctxkey.BillingReconciled) {
		return false
	}
	value, ok := c.Get(ctxkey.Meta)
	if !ok {
		return false
	}
	m, ok := value.(*metalib.Meta)
	if !ok || m == nil || m.UserId <= 0 || m.ChannelId <= 0 || m.ActualModelName == "" || tokenID != m.TokenId {
		return false
	}
	// An explicit provider error response is not uncertain generation work: it
	// keeps the generic retained hold so the shared retry loop can refund it and
	// fail over instead of finalizing a charge and vetoing the retry.
	isConservativeDeepSeekTool := strings.HasPrefix(reason, "deepseek_tool_") &&
		tooling.ConservativeDeepSeekToolQuota(c) > 0
	if !isConservativeDeepSeekTool && c.GetInt(ctxkey.Channel) != channeltype.Cohere &&
		(reason == upstreamHTTPErrorReason || !isRetainedGenerationAdmission(c, m, amount)) {
		return false
	}
	c.Set(responseSettlementKey, true)
	markBillingReconciled(c)
	publishRetainedReservation(c, m, amount, tokenID,
		"Conservative reservation retained: upstream execution or final usage is uncertain; reconciliation required",
		"uncertain_upstream_admission_"+reason, "settleRetainedRequestAdmission")
	return true
}

// publishRetainedReservation records an already-debited reservation as a
// visible, estimate-flagged charge without another debit: it reconciles the
// provisional log into a consume log, adds the amount to used quota and
// records the request cost. Parameters: c is the request context, m the
// attempt metadata, amount the retained reservation, tokenID the debited
// token, content the log text, reason the billing_estimate_reason and stage
// the critical-task label. It returns nothing; the writes run as detached
// critical billing work.
func publishRetainedReservation(c *gin.Context, m *metalib.Meta, amount int64, tokenID int, content, reason, stage string) {
	entry := &model.Log{UserId: m.UserId, ChannelId: m.ChannelId, ModelName: userVisibleModelName(m, m.ActualModelName), TokenName: m.TokenName,
		RequestId: c.GetString(ctxkey.RequestId), IsStream: m.IsStream, Content: content,
		Metadata: billingEstimateMetadata(nil, reason)}
	model.SetLogExternalUUIDs(entry, m.UserUUID, m.ChannelUUID, m.TokenUUID)
	provisionalID, userID, requestID := c.GetInt(ctxkey.ProvisionalLogId), m.UserId, entry.RequestId
	goDetachedBillingWork(relayctx.Detach(c), stage, func(ctx context.Context) {
		billing.PostConsumeQuotaWithLog(ctx, tokenID, 0, amount, entry, provisionalID)
		if requestID != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, amount); err != nil {
				gmw.GetLogger(ctx).Error("record retained reservation cost failed", zap.Error(err))
			}
		}
	})
}

// retainedReservationPendingKey marks an attempt whose reservation the
// conservative refund policy retained (the request may have reached the
// provider and no receipt proved its cost) without publishing a charge. A
// cross-channel retry refunds it (ResetPerAttemptBillingForRetry); a terminal
// relay failure publishes it (SettleTerminalRetainedReservation). The value is
// the refund reason.
const retainedReservationPendingKey = "billing_retained_reservation_pending"

// markRetainedReservationPending records that this attempt's reservation was
// retained but not yet published. Parameters: c is the request context and
// reason the conservative refund reason. It returns nothing.
func markRetainedReservationPending(c *gin.Context, reason string) {
	if reason == "" {
		reason = "unspecified"
	}
	c.Set(retainedReservationPendingKey, reason)
}

// SettleTerminalRetainedReservation publishes the final failed attempt's
// retained reservation once the relay has stopped retrying. The documented
// policy keeps a possibly-forwarded attempt's hold on a terminal failure so
// unmetered provider work is never billed as zero; before this, the hold stayed
// debited while used quota, the request cost and the logs showed nothing (the
// provisional log stayed filtered out of every Logs query). Parameter c is the
// request context. It returns nothing and is a no-op unless a retained, still
// unpublished and unrefunded reservation exists.
func SettleTerminalRetainedReservation(c *gin.Context) {
	if c == nil {
		return
	}
	reason := c.GetString(retainedReservationPendingKey)
	if reason == "" {
		return
	}
	c.Set(retainedReservationPendingKey, "")
	amount := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	if amount <= 0 || c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed) || c.GetBool(responseSettlementKey) {
		return
	}
	value, ok := c.Get(ctxkey.Meta)
	if !ok {
		return
	}
	m, ok := value.(*metalib.Meta)
	if !ok || m == nil || m.UserId <= 0 || m.ChannelId <= 0 || m.ActualModelName == "" || m.TokenId <= 0 {
		return
	}
	gmw.GetLogger(c).Warn("publishing retained reservation after a terminal upstream failure",
		zap.Int64("retained_quota", amount), zap.String("reason", reason))
	publishRetainedReservation(c, m, amount, m.TokenId,
		"Reservation retained after a terminal upstream failure: the request may have reached the provider and no receipt proved its cost",
		"retained_reservation_"+reason, "settleTerminalRetainedReservation")
}

// retainDeepSeekToolAttempt settles an already-forwarded uncertain DeepSeek
// Search/Fetch attempt at its admitted conservative reservation. It never
// introduces a new debit or silently allows another automatic provider retry.
func retainDeepSeekToolAttempt(c *gin.Context, amount int64, tokenID int, reason string) bool {
	if tooling.ConservativeDeepSeekToolQuota(c) <= 0 {
		return false
	}
	return settleRetainedRequestAdmission(c, amount, tokenID, "deepseek_tool_"+reason)
}

// refundRejectedAdmission completes a proven pre-inference rejection refund before
// allowing a retry. Waiting here prevents an old attempt's delayed cost=0 write
// from erasing the next attempt's charge. Failed refunds block automatic replay.
func refundRejectedAdmission(c *gin.Context, amount int64, tokenID int, reason string) (bool, bool) {
	if !providerRejectedBeforeInference(c) {
		return false, false
	}
	markBillingReconciled(c)
	ctx, cancel := context.WithTimeout(relayctx.Detach(c), 30*time.Second)
	defer cancel()
	refunded := amount == 0 || newConservativeRefundSnapshot(c, amount, tokenID, reason).refund(ctx)
	cost := amount
	if refunded {
		cost = 0
	} else {
		c.Set(responseSettlementKey, true)
	}
	if requestID := c.GetString(ctxkey.RequestId); requestID != "" {
		if err := model.UpdateUserRequestCostQuotaByRequestID(c.GetInt(ctxkey.Id), requestID, cost); err != nil {
			gmw.GetLogger(c).Error("record provider admission refund cost failed", zap.Error(err))
		}
	}
	return true, refunded
}
