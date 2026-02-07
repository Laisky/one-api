package controller

import (
	"context"
	"math"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

// preConsumeResponseAPIQuota pre-consumes quota for Response API requests
func preConsumeResponseAPIQuota(
	c *gin.Context,
	responseAPIRequest *openai.ResponseAPIRequest,
	promptTokens int,
	inputRatio float64,
	outputRatio float64,
	background bool,
	meta *metalib.Meta,
) (int64, *relaymodel.ErrorWithStatusCode) {
	ctx := gmw.Ctx(c)
	baseQuota := calculateResponseAPIPreconsumeQuota(promptTokens, responseAPIRequest.MaxOutputTokens, inputRatio, outputRatio, background)

	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	if !tokenQuotaUnlimited && tokenQuota > 0 && tokenQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("token quota is not enough"), "insufficient_token_quota", http.StatusForbidden)
	}

	err = model.PreConsumeTokenQuota(ctx, c.GetInt(ctxkey.TokenId), baseQuota)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}

	return baseQuota, nil
}

// calculateResponseAPIPreconsumeQuota calculates the estimated quota to pre-consume for Response API requests
func calculateResponseAPIPreconsumeQuota(promptTokens int, maxOutputTokens *int, inputRatio float64, outputRatio float64, background bool) int64 {
	promptQuota := float64(promptTokens) * inputRatio
	completionQuota := 0.0
	if maxOutputTokens != nil {
		completionQuota = float64(*maxOutputTokens) * outputRatio
	}

	baseQuota := int64(promptQuota + completionQuota)
	if inputRatio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}

	if background && outputRatio > 0 {
		backgroundQuota := int64(math.Ceil(float64(config.PreconsumeTokenForBackgroundRequest) * outputRatio))
		if backgroundQuota <= 0 {
			backgroundQuota = 1
		}
		if baseQuota < backgroundQuota {
			baseQuota = backgroundQuota
		}
	}

	return baseQuota
}

// postConsumeResponseAPIQuota calculates final quota consumption for Response API requests
// Following DRY principle by reusing the centralized billing.PostConsumeQuota function
func postConsumeResponseAPIQuota(ctx context.Context,
	usage *relaymodel.Usage,
	meta *metalib.Meta,
	responseAPIRequest *openai.ResponseAPIRequest,
	preConsumedQuota int64,
	modelRatio float64,
	groupRatio float64,
	channelCompletionRatio map[string]float64) (quota int64) {

	if usage == nil {
		// No gin context here; cannot use request-scoped logger
		// Keep silent here to avoid global logger; caller should ensure usage
		return
	}

	pricingAdaptor := resolvePricingAdaptor(meta)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              responseAPIRequest.Model,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	quota = computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}

	// Use centralized detailed billing function to follow DRY principle
	quotaDelta := quota - preConsumedQuota
	cachedPrompt := computeResult.CachedPromptTokens
	promptTokens := computeResult.PromptTokens
	completionTokens := computeResult.CompletionTokens
	usedModelRatio := computeResult.UsedModelRatio
	if usedModelRatio == 0 {
		usedModelRatio = modelRatio
	}
	usedCompletionRatio := computeResult.UsedCompletionRatio
	if usedCompletionRatio == 0 {
		usedCompletionRatio = pricing.GetCompletionRatioWithThreeLayers(responseAPIRequest.Model, channelCompletionRatio, pricingAdaptor)
	}

	// Derive RequestId/TraceId from std context if possible
	var requestId string
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
	}
	traceId := tracing.GetTraceIDFromContext(ctx)
	if meta.TokenId > 0 && meta.UserId > 0 && meta.ChannelId > 0 {
		var toolSummary *model.ToolUsageSummary
		if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
			if raw, exists := ginCtx.Get(ctxkey.ToolInvocationSummary); exists {
				if summary, ok := raw.(*model.ToolUsageSummary); ok {
					toolSummary = summary
				}
			}
		}
		metadata := model.AppendToolUsageMetadata(nil, toolSummary)
		metadata = model.AppendCacheWriteTokensMetadata(metadata, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)

		billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
			Ctx:                    ctx,
			TokenId:                meta.TokenId,
			QuotaDelta:             quotaDelta,
			TotalQuota:             quota,
			UserId:                 meta.UserId,
			ChannelId:              meta.ChannelId,
			PromptTokens:           promptTokens,
			CompletionTokens:       completionTokens,
			ModelRatio:             usedModelRatio,
			GroupRatio:             groupRatio,
			ModelName:              responseAPIRequest.Model,
			TokenName:              meta.TokenName,
			IsStream:               meta.IsStream,
			StartTime:              meta.StartTime,
			SystemPromptReset:      false,
			CompletionRatio:        usedCompletionRatio,
			ToolsCost:              usage.ToolsCost,
			CachedPromptTokens:     cachedPrompt,
			CachedCompletionTokens: 0,
			CacheWrite5mTokens:     usage.CacheWrite5mTokens,
			CacheWrite1hTokens:     usage.CacheWrite1hTokens,
			Metadata:               metadata,
			RequestId:              requestId,
			TraceId:                traceId,
		})
	} else {
		// Should not happen; log for investigation
		lg := gmw.GetLogger(ctx)
		lg.Error("postConsumeResponseAPIQuota missing essential meta information",
			zap.Int("token_id", meta.TokenId),
			zap.Int("user_id", meta.UserId),
			zap.Int("channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}
