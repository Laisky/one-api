package controller

import (
	"context"
	"math"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// postConsumeResponseAPIQuotaDetailed lets tests capture the billing detail without DB writes.
var postConsumeResponseAPIQuotaDetailed = billing.PostConsumeQuotaDetailed

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
	syncUserQuotaCacheAfterPreConsume(ctx, meta.UserId, baseQuota, "response_api_preconsume")

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
	channelModelRatio map[string]float64,
	groupRatio float64,
	channelModelConfigs map[string]model.ModelConfigLocal,
	channelCompletionRatio map[string]float64) (quota int64) {

	// !! NO-USAGE / ZERO-USAGE GUARD !!
	//
	// Some upstreams do not report token usage at all (a transport that only
	// passes bytes through, a truncated stream, the OpenAI WebSocket Response
	// API). Two things must NOT happen when that occurs:
	//
	//  1. Reconciling with zero usage yields quotaDelta = 0 - preConsumedQuota,
	//     which REFUNDS the pre-consumed amount and makes the request free.
	//  2. Returning early skips settlement entirely, which strands the
	//     provisional log row at LogTypeProvisional. That row is filtered out of
	//     every Logs-page query, so the user is charged for a request nobody can
	//     see, and the user/channel usage aggregates silently under-report.
	//
	// So an unmeasurable request settles at the pre-consumed estimate — a zero
	// delta, keeping the money already debited — and still flows through the one
	// settlement call below, which reconciles the provisional row into a visible
	// consume log.
	settledAtEstimate := false
	if usage == nil {
		// The upstream returned success without any usage. That is an adaptor
		// defect, not a client error: report it loudly instead of silently
		// dropping the charge from the ledger.
		gmw.GetLogger(ctx).Error("response api post-billing received no usage; settling at the pre-consumed estimate",
			zap.Int64("pre_consumed_quota", preConsumedQuota),
			zap.String("model", responseAPIRequest.Model),
		)
		usage = &relaymodel.Usage{}
		settledAtEstimate = true
	} else if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		gmw.GetLogger(ctx).Warn("response api post-billing received zero usage; settling at the pre-consumed estimate",
			zap.Int64("pre_consumed_quota", preConsumedQuota),
			zap.String("model", responseAPIRequest.Model),
		)
		settledAtEstimate = true
	}

	pricingAdaptor := resolvePricingAdaptor(meta)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              responseAPIRequest.Model,
		ModelRatio:             modelRatio,
		ChannelModelRatio:      channelModelRatio,
		GroupRatio:             groupRatio,
		ChannelModelConfigs:    channelModelConfigs,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
		RequestTime:            meta.StartTime,
	})

	quota = computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}
	if settledAtEstimate {
		// Keep exactly what was already debited: a zero delta charges nothing
		// extra and, crucially, refunds nothing.
		quota = preConsumedQuota
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
		usedCompletionRatio = pricing.ResolveCompletionRatioAt(responseAPIRequest.Model, channelModelConfigs, channelCompletionRatio, pricingAdaptor, meta.StartTime)
	}

	// Resolve request-scoped identifiers from the detached billing snapshot (or, for a
	// synchronous caller, from the embedded gin context). NEVER read them off a live
	// *gin.Context here: this runs inside a post-billing goroutine and gin recycles c.
	billingID := billingIdentityFromContext(ctx)
	requestId := billingID.requestID
	provisionalLogId := billingID.provisionalLogID
	traceId := billingID.traceID
	if meta.TokenId > 0 && meta.UserId > 0 && meta.ChannelId > 0 {
		toolSummary := billingID.toolSummary
		metadata := model.AppendCacheWriteTokensMetadata(nil, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)
		if settledAtEstimate {
			if metadata == nil {
				metadata = model.LogMetadata{}
			}
			metadata[model.LogMetadataKeyEstimatedCharge] = true
		}

		postConsumeResponseAPIQuotaDetailed(billing.QuotaConsumeDetail{
			Ctx:                ctx,
			TokenId:            meta.TokenId,
			QuotaDelta:         quotaDelta,
			TotalQuota:         quota,
			UserId:             meta.UserId,
			UserUUID:           meta.UserUUID,
			ChannelId:          meta.ChannelId,
			ChannelUUID:        meta.ChannelUUID,
			PromptTokens:       promptTokens,
			CompletionTokens:   completionTokens,
			ModelRatio:         usedModelRatio,
			GroupRatio:         groupRatio,
			OriginModelName:    meta.OriginModelName,
			ModelName:          responseAPIRequest.Model,
			TokenUUID:          meta.TokenUUID,
			TokenName:          meta.TokenName,
			IsStream:           meta.IsStream,
			StartTime:          meta.StartTime,
			SystemPromptReset:  false,
			CompletionRatio:    usedCompletionRatio,
			ToolsCost:          usage.ToolsCost,
			CachedPromptTokens: cachedPrompt,
			CacheWrite5mTokens: usage.CacheWrite5mTokens,
			CacheWrite1hTokens: usage.CacheWrite1hTokens,
			Metadata:           metadata,
			RequestId:          requestId,
			TraceId:            traceId,
			ProvisionalLogId:   provisionalLogId,
			UserAPIFormat:      resolveUserAPIFormat(meta.Mode),
			UpstreamAPIFormat:  apitype.String(meta.APIType),
			UpstreamEndpoint:   meta.UpstreamRequestURL,
			ToolUsageSummary:   toolSummary,
		})
	} else {
		// Should not happen; log for investigation
		lg := gmw.GetLogger(ctx)
		lg.Error("postConsumeResponseAPIQuota missing essential meta information",
			zap.Int("meta_token_id", meta.TokenId),
			zap.Int("meta_user_id", meta.UserId),
			zap.Int("meta_channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}
