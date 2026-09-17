package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// prepareJinaBudget validates a mapped request and stores a conservative allowance.
// It returns prompt usage for admission, or a client error before any paid dispatch.
func prepareJinaBudget(c *gin.Context, m *metalib.Meta, request *relaymodel.GeneralOpenAIRequest) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	budget, err := jina.QuoteRequest(request, m.Mode)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "unbounded_jina_request", http.StatusBadRequest)
	}
	jina.StoreBillingBudget(c, budget)
	return &relaymodel.Usage{PromptTokens: budget.Input, TotalTokens: budget.Input}, nil
}

// preConsumeJinaQuota resolves effective flat prices and physically reserves the
// complete allowance, even for trusted/unlimited tokens. It returns a reservation
// or an error; no paid work is dispatched when admission or pricing is uncertain.
func preConsumeJinaQuota(c *gin.Context, m *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	budget, ok := jina.GetBillingBudget(c)
	if !ok {
		return 0, openai.ErrorWrapper(errors.New("jina billing allowance missing"), "jina_billing_not_prepared", http.StatusInternalServerError)
	}
	configs := getChannelModelConfigs(c)
	inputOverrides, outputOverrides := getChannelRatios(c)
	provider := &jina.Adaptor{}
	cfg, found := pricing.ResolveModelConfig(m.ActualModelName, configs, provider, m.StartTime)
	if !found || len(cfg.Tiers) > 0 || cfg.PerCall != nil {
		return 0, openai.ErrorWrapper(errors.New("jina conservative admission requires verified flat token pricing"), "unsupported_jina_pricing", http.StatusBadRequest)
	}
	inputRatio := pricing.ResolveModelRatioAt(m.ActualModelName, configs, inputOverrides, provider, m.StartTime)
	outputRatio := pricing.ResolveCompletionRatioAt(m.ActualModelName, configs, outputOverrides, provider, m.StartTime)
	amount, err := jina.BudgetQuota(budget, inputRatio, outputRatio, c.GetFloat64(ctxkey.ChannelRatio))
	if err != nil {
		return 0, openai.ErrorWrapper(err, "invalid_jina_pricing", http.StatusBadRequest)
	}
	if amount > 0 {
		if err := model.PreConsumeTokenQuota(gmw.Ctx(c), m.TokenId, amount); err != nil {
			return 0, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
		syncUserQuotaCacheAfterPreConsume(gmw.Ctx(c), m.UserId, amount, "jina_preconsume")
	}
	return amount, nil
}

// JinaAttemptMayHaveCost reports whether automatic replay would risk another paid
// inference. HTTP rejection before inference is distinguished from an ambiguous
// timeout, broken stream, missing usage, or server error after dispatch.
func JinaAttemptMayHaveCost(c *gin.Context) bool {
	return c != nil && c.GetInt(ctxkey.Channel) == channeltype.Jina &&
		c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) && !jina.RejectedBeforeInference(c)
}

// settleJinaRetainedReservation turns an unreconciled, possibly forwarded Jina
// attempt into an explicitly estimated consume record. It never debits the hold
// twice. All asynchronous data is snapshotted before c can be recycled.
func settleJinaRetainedReservation(c *gin.Context) bool {
	if !JinaAttemptMayHaveCost(c) || c.GetBool(ctxkey.BillingReconciled) {
		return false
	}
	amount := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	if amount <= 0 {
		return false
	}
	markBillingReconciled(c)
	m := metalib.GetByContext(c)
	entry := &model.Log{UserId: m.UserId, ChannelId: m.ChannelId, ModelName: m.ActualModelName,
		TokenName: m.TokenName, RequestId: c.GetString(ctxkey.RequestId), IsStream: m.IsStream,
		Content:  "Conservative Jina reservation retained: upstream completion or usage could not be verified",
		Metadata: billingEstimateMetadata(nil, "jina_unresolved_upstream_attempt")}
	model.SetLogExternalUUIDs(entry, m.UserUUID, m.ChannelUUID, m.TokenUUID)
	tokenID, provID, userID, requestID := m.TokenId, c.GetInt(ctxkey.ProvisionalLogId), m.UserId, entry.RequestId
	goDetachedBillingWork(relayctx.Detach(c), "settleJinaRetainedReservation", func(ctx context.Context) {
		billing.PostConsumeQuotaWithLog(ctx, tokenID, 0, amount, entry, provID)
		if requestID != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, amount); err != nil {
				gmw.GetLogger(ctx).Error("record retained jina reservation failed", zap.Error(err))
			}
		}
	})
	return true
}

// exactJinaUsageQuota uses decimal arithmetic for the admitted flat price
// contract. Unsupported receipt arithmetic retains the larger existing charge
// or reservation and marks it estimated instead of silently returning zero.
func exactJinaUsageQuota(ctx context.Context, m *metalib.Meta, usage *relaymodel.Usage,
	calculated, reserved int64, inputRatio, completionRatio, groupRatio float64) int64 {
	if m.ChannelType != channeltype.Jina {
		return calculated
	}
	amount, err := jina.BudgetQuota(jina.BillingBudget{Input: usage.PromptTokens, Output: usage.CompletionTokens}, inputRatio, completionRatio, groupRatio)
	if err != nil {
		usage.BillingEstimateReason = "jina_unrepresentable_usage_retained_reservation"
		gmw.GetLogger(ctx).Error("jina usage cannot be settled exactly; manual reconciliation required", zap.Error(err))
		return max(calculated, reserved)
	}
	return amount
}

// refundJinaAdmission completes a proven pre-inference rejection refund before
// allowing a retry. Waiting here prevents an old attempt's delayed cost=0 write
// from erasing the next attempt's charge. Failed refunds block automatic replay.
func refundJinaAdmission(c *gin.Context, amount int64, tokenID int, reason string) (bool, bool) {
	if c.GetInt(ctxkey.Channel) != channeltype.Jina || !jina.RejectedBeforeInference(c) {
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
			gmw.GetLogger(c).Error("record jina admission refund cost failed", zap.Error(err))
		}
	}
	return true, refunded
}
