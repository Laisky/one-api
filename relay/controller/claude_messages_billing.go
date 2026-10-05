package controller

import (
	"context"
	"net/http"

	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/channeltype"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// preConsumeClaudeMessagesQuota reserves quota for a Claude Messages request.
// Jina requests reject unbounded tool calls and reserve their complete prepared
// allowance; every other paid request also reserves its complete quote.
func preConsumeClaudeMessagesQuota(c *gin.Context, request *ClaudeMessagesRequest, promptTokens int, ratio float64, completionRatio float64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	if meta.ChannelType == channeltype.Jina {
		if len(request.Tools) > 0 {
			return 0, openai.ErrorWrapper(errors.New("Jina OCR tool calls have no bounded billing contract"), "unbounded_jina_request", http.StatusBadRequest)
		}
		converted, err := (&jina.Adaptor{}).ConvertClaudeRequest(c, request)
		if err != nil {
			return 0, openai.ErrorWrapper(err, "invalid_jina_request", http.StatusBadRequest)
		}
		chat, ok := converted.(*relaymodel.GeneralOpenAIRequest)
		if !ok {
			return 0, openai.ErrorWrapper(errors.New("invalid Jina OCR conversion"), "invalid_jina_request", http.StatusBadRequest)
		}
		promptUsage, apiErr := prepareJinaBudget(c, meta, chat)
		if apiErr != nil {
			return 0, apiErr
		}
		meta.PromptTokens = promptUsage.PromptTokens
		return preConsumeJinaQuota(c, meta)
	}
	baseQuota, err := checkedClaudeQuotaEstimate(promptTokens, request.MaxTokens, ratio, completionRatio)
	if err != nil {
		return 0, openai.ErrorWrapper(err, "invalid_claude_quota_quote", http.StatusBadRequest)
	}
	if ratio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}
	// Admitted paid server tools reserve their bounded allowance with the tokens.
	baseQuota = withClaudeToolAllowance(c, baseQuota)

	return reservePaidRequestQuota(c, meta, baseQuota, "claude_messages_preconsume")
}

// postConsumeClaudeMessagesQuotaWithTraceID calculates and records the final
// Claude Messages charge with an explicit trace ID. Missing, zero, or estimated
// usage retains the reservation and incremental charges; Jina usage uses exact
// settlement pricing. It returns the total quota submitted for settlement.
func postConsumeClaudeMessagesQuotaWithTraceID(ctx context.Context, requestId string, traceId string, usage *relaymodel.Usage, meta *metalib.Meta, request *ClaudeMessagesRequest, ratio float64, preConsumedQuota int64, incrementalCharged int64, modelRatio float64, channelModelRatio map[string]float64, groupRatio float64, channelModelConfigs map[string]model.ModelConfigLocal, channelCompletionRatio map[string]float64) int64 {
	if usage == nil {
		usage = &relaymodel.Usage{BillingEstimateReason: "missing_usage_retained_reservation"}
	}

	pricingAdaptor := resolvePricingAdaptor(meta)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              request.Model,
		ModelRatio:             modelRatio,
		ChannelModelRatio:      channelModelRatio,
		GroupRatio:             groupRatio,
		ChannelModelConfigs:    channelModelConfigs,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
		RequestTime:            meta.StartTime,
	})
	retainUnpricedUsage(ctx, usage, computeResult)

	quota := exactJinaUsageQuota(ctx, meta, usage, computeResult.TotalQuota, preConsumedQuota+incrementalCharged,
		computeResult.UsedModelRatio, computeResult.UsedCompletionRatio, groupRatio)
	if !hasBillableUsage(usage) {
		quota = preConsumedQuota + incrementalCharged
		usage.BillingEstimateReason = "missing_or_zero_usage_retained_reservation"
	}
	if usage.BillingEstimateReason != "" {
		quota = max(quota, preConsumedQuota+incrementalCharged)
	}

	metadata := billingEstimateMetadata(model.AppendCacheWriteTokensMetadata(nil, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens), usage.BillingEstimateReason)

	// Use centralized detailed billing function with explicit trace ID
	quotaDelta := quota - preConsumedQuota - incrementalCharged
	// Resolve identifiers from the detached billing snapshot (or, for a synchronous
	// caller, from the embedded gin context). NEVER read them off a live *gin.Context
	// here: this runs inside a post-billing goroutine and gin recycles c.
	billingID := billingIdentityFromContext(ctx)
	provisionalLogId := billingID.provisionalLogID
	if requestId == "" {
		requestId = billingID.requestID
	}
	// For Claude models, upstream reports non-cached input tokens as PromptTokens
	// and cached tokens separately. Sum them so the log shows the total prompt tokens.
	logPromptTokens := computeResult.PromptTokens + computeResult.CachedPromptTokens

	billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
		Ctx:                ctx,
		TokenId:            meta.TokenId,
		QuotaDelta:         quotaDelta,
		TotalQuota:         quota,
		UserId:             meta.UserId,
		UserUUID:           meta.UserUUID,
		ChannelId:          meta.ChannelId,
		ChannelUUID:        meta.ChannelUUID,
		PromptTokens:       logPromptTokens,
		CompletionTokens:   computeResult.CompletionTokens,
		ModelRatio:         computeResult.UsedModelRatio,
		GroupRatio:         groupRatio,
		OriginModelName:    meta.OriginModelName,
		ModelName:          request.Model,
		TokenUUID:          meta.TokenUUID,
		TokenName:          meta.TokenName,
		IsStream:           meta.IsStream,
		StartTime:          meta.StartTime,
		SystemPromptReset:  false,
		CompletionRatio:    computeResult.UsedCompletionRatio,
		ToolsCost:          usage.ToolsCost,
		CachedPromptTokens: computeResult.CachedPromptTokens,
		CacheWrite5mTokens: usage.CacheWrite5mTokens,
		CacheWrite1hTokens: usage.CacheWrite1hTokens,
		Metadata:           metadata,
		RequestId:          requestId,
		TraceId:            traceId,
		ProvisionalLogId:   provisionalLogId,
		UserAPIFormat:      resolveUserAPIFormat(meta.Mode),
		UpstreamAPIFormat:  apitype.String(meta.APIType),
		UpstreamEndpoint:   meta.UpstreamRequestURL,
	})

	// Log with context if available
	gmw.GetLogger(ctx).Debug("Claude Messages quota with trace ID",
		zap.Int64("pre_consumed", preConsumedQuota),
		zap.Int64("actual", quota),
		zap.Int64("difference", quotaDelta),
	)
	return quota
}
