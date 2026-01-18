package controller

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// preConsumeClaudeMessagesQuota pre-consumes quota for Claude Messages API requests.
func preConsumeClaudeMessagesQuota(c *gin.Context, request *ClaudeMessagesRequest, promptTokens int, ratio float64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	// Use similar logic to ChatCompletion pre-consumption
	ctx := gmw.Ctx(c)
	preConsumedTokens := int64(promptTokens)
	if request.MaxTokens > 0 {
		preConsumedTokens += int64(request.MaxTokens)
	}

	baseQuota := int64(float64(preConsumedTokens) * ratio)
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
		zap.Int("tokens", int(preConsumedTokens)),
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

	// Use three-layer pricing system for completion ratio
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(request.Model, channelCompletionRatio, pricingAdaptor)
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	// Calculate base quota
	baseQuota := int64(math.Ceil((float64(promptTokens) + float64(completionTokens)*completionRatio) * ratio))

	// No structured output surcharge
	quota := baseQuota + usage.ToolsCost
	if ratio != 0 && quota <= 0 {
		quota = 1
	}

	totalTokens := promptTokens + completionTokens
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
	}

	// Extract cache token counts from usage details
	cachedPromptTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedPromptTokens = usage.PromptTokensDetails.CachedTokens
	}
	cachedCompletionTokens := 0
	if usage.CompletionTokensDetails != nil {
		cachedCompletionTokens = usage.CompletionTokensDetails.CachedTokens
	}

	cacheWrite5mTokens := usage.CacheWrite5mTokens
	cacheWrite1hTokens := usage.CacheWrite1hTokens
	metadata := model.AppendCacheWriteTokensMetadata(nil, cacheWrite5mTokens, cacheWrite1hTokens)

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
		PromptTokens:           promptTokens,
		CompletionTokens:       completionTokens,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ModelName:              request.Model,
		TokenName:              meta.TokenName,
		IsStream:               meta.IsStream,
		StartTime:              meta.StartTime,
		SystemPromptReset:      false,
		CompletionRatio:        completionRatio,
		ToolsCost:              usage.ToolsCost,
		CachedPromptTokens:     cachedPromptTokens,
		CachedCompletionTokens: cachedCompletionTokens,
		CacheWrite5mTokens:     cacheWrite5mTokens,
		CacheWrite1hTokens:     cacheWrite1hTokens,
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
