package controller

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

// settleRetainedRequestAdmission labels an uncertain Cohere attempt and
// reconciles its existing hold without another debit. Cohere's explicit
// pre-inference rejection statuses are handled separately. Generic relay retry
// policy is deliberately unchanged by the atomic-admission repair; Jina keeps
// its existing provider lifecycle. All asynchronous inputs are value snapshots.
func settleRetainedRequestAdmission(c *gin.Context, amount int64, tokenID int, reason string) bool {
	if c == nil || c.GetInt(ctxkey.Channel) != channeltype.Cohere || amount <= 0 || c.GetBool(ctxkey.BillingReconciled) {
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
	c.Set(responseSettlementKey, true)
	markBillingReconciled(c)
	entry := &model.Log{UserId: m.UserId, ChannelId: m.ChannelId, ModelName: userVisibleModelName(m, m.ActualModelName), TokenName: m.TokenName,
		RequestId: c.GetString(ctxkey.RequestId), IsStream: m.IsStream,
		Content:  "Conservative reservation retained: upstream execution or final usage is uncertain; reconciliation required",
		Metadata: billingEstimateMetadata(nil, "uncertain_upstream_admission_"+reason)}
	model.SetLogExternalUUIDs(entry, m.UserUUID, m.ChannelUUID, m.TokenUUID)
	provisionalID, userID, requestID := c.GetInt(ctxkey.ProvisionalLogId), m.UserId, entry.RequestId
	goDetachedBillingWork(relayctx.Detach(c), "settleRetainedRequestAdmission", func(ctx context.Context) {
		billing.PostConsumeQuotaWithLog(ctx, tokenID, 0, amount, entry, provisionalID)
		if requestID != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, amount); err != nil {
				gmw.GetLogger(ctx).Error("record retained admission cost failed", zap.Error(err))
			}
		}
	})
	return true
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
