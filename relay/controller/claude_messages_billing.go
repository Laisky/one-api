package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

// preConsumeClaudeMessagesQuota pre-consumes quota for Claude Messages API requests.
func preConsumeClaudeMessagesQuota(c *gin.Context, request *ClaudeMessagesRequest, promptTokens int, ratio float64, completionRatio float64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	// Use similar logic to ChatCompletion pre-consumption
	ctx := gmw.Ctx(c)
	promptQuota := float64(promptTokens) * ratio
	completionQuota := 0.0
	if request.MaxTokens > 0 {
		completionQuota = float64(request.MaxTokens) * ratio * completionRatio
	}

	baseQuota := int64(promptQuota + completionQuota)
	if ratio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}

	// Check user quota first
	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(ctx, meta.UserId, baseQuota)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*baseQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*baseQuota) {
		// in this case, we do not pre-consume quota
		// because the user and token have enough quota
		baseQuota = 0
		gmw.GetLogger(c).Info(fmt.Sprintf("user %d has enough quota %d, trusted and no need to pre-consume", meta.UserId, userQuota))
	}
	if baseQuota > 0 {
		err := model.PreConsumeTokenQuota(ctx, meta.TokenId, baseQuota)
		if err != nil {
			return baseQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}

	gmw.GetLogger(c).Debug("pre-consumed quota for Claude Messages",
		zap.Int64("quota", baseQuota),
		zap.Float64("ratio", ratio))
	return baseQuota, nil
}

// postConsumeClaudeMessagesQuotaWithTraceID calculates and applies final quota consumption for Claude Messages API with explicit trace ID.
// Parameters: ctx/requestId/traceId identify the request, usage/meta/request carry usage metadata, ratio/preConsumedQuota/incrementalCharged/modelRatio/groupRatio/channelCompletionRatio drive billing.
// Returns: the final quota charged for the request.
func postConsumeClaudeMessagesQuotaWithTraceID(ctx context.Context, requestId string, traceId string, usage *relaymodel.Usage, meta *metalib.Meta, request *ClaudeMessagesRequest, ratio float64, preConsumedQuota int64, incrementalCharged int64, modelRatio float64, groupRatio float64, channelCompletionRatio map[string]float64) int64 {
	if usage == nil {
		// Context may be detached; log with context if available
		gmw.GetLogger(ctx).Warn("usage is nil for Claude Messages API")
		return 0
	}

	pricingAdaptor := resolvePricingAdaptor(meta)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              request.Model,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	quota := computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}

	metadata := model.AppendCacheWriteTokensMetadata(nil, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)

	// Use centralized detailed billing function with explicit trace ID
	quotaDelta := quota - preConsumedQuota - incrementalCharged
	// If requestId somehow empty, try derive from ctx (best-effort)
	if requestId == "" {
		if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
			requestId = ginCtx.GetString(ctxkey.RequestId)
		}
	}
	billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
		Ctx:                    ctx,
		TokenId:                meta.TokenId,
		QuotaDelta:             quotaDelta,
		TotalQuota:             quota,
		UserId:                 meta.UserId,
		ChannelId:              meta.ChannelId,
		PromptTokens:           computeResult.PromptTokens,
		CompletionTokens:       computeResult.CompletionTokens,
		ModelRatio:             computeResult.UsedModelRatio,
		GroupRatio:             groupRatio,
		ModelName:              request.Model,
		TokenName:              meta.TokenName,
		IsStream:               meta.IsStream,
		StartTime:              meta.StartTime,
		SystemPromptReset:      false,
		CompletionRatio:        computeResult.UsedCompletionRatio,
		ToolsCost:              usage.ToolsCost,
		CachedPromptTokens:     computeResult.CachedPromptTokens,
		CachedCompletionTokens: computeResult.CachedCompletionTokens,
		CacheWrite5mTokens:     usage.CacheWrite5mTokens,
		CacheWrite1hTokens:     usage.CacheWrite1hTokens,
		Metadata:               metadata,
		RequestId:              requestId,
		TraceId:                traceId,
	})

	// Log with context if available
	gmw.GetLogger(ctx).Debug("Claude Messages quota with trace ID",
		zap.Int64("pre_consumed", preConsumedQuota),
		zap.Int64("actual", quota),
		zap.Int64("difference", quotaDelta),
	)
	return quota
}
