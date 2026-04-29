package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// billingOpsTimeout is the maximum time allowed for all database operations
// within a single PostConsumeQuotaWithLog call. This is independent of the
// caller's context timeout and acts as a safety net against indefinite hangs
// after the parent context is detached from cancellation.
const billingOpsTimeout = 60 * time.Second

// postConsumeQuotaWithLogFn lets tests capture the generated log entry without hitting the DB.
var postConsumeQuotaWithLogFn = PostConsumeQuotaWithLog

// PostConsumeQuotaWithLog is the unified billing entry that consumes quota, updates caches,
// records a consume log, and updates user/channel aggregates.
// Caller must provide a pre-filled log entry (including RequestId/TraceId if desired).
func PostConsumeQuotaWithLog(ctx context.Context, tokenId int, quotaDelta int64, totalQuota int64, logEntry *model.Log, provisionalLogId ...int) {
	if ctx == nil || logEntry == nil {
		lg := logger.FromContext(ctx)
		lg.Error("PostConsumeQuotaWithLog: invalid args", zap.Bool("ctx_nil", ctx == nil), zap.Bool("log_nil", logEntry == nil))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_with_log", 0, 0, "")
		return
	}

	// Billing operations are critical and must complete even after the HTTP
	// request context is canceled (e.g., client disconnect, handler return).
	// Detach from parent cancellation while preserving context values (logger,
	// trace ID) and apply a dedicated timeout.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), billingOpsTimeout)
	defer cancel()

	billingStartTime := time.Now()
	billingSuccess := true
	lg := logger.FromContext(ctx)
	if tokenId <= 0 {
		lg.Error("PostConsumeQuotaWithLog: invalid tokenId", zap.Int("token_id", tokenId))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_with_log", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		return
	}
	if logEntry.UserId <= 0 || logEntry.ChannelId <= 0 {
		lg.Error("PostConsumeQuotaWithLog: invalid user/channel", zap.Int("user_id", logEntry.UserId), zap.Int("channel_id", logEntry.ChannelId))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_with_log", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		return
	}
	if logEntry.ModelName == "" {
		lg.Error("PostConsumeQuotaWithLog: modelName is empty")
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_with_log", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		return
	}

	// Consume remaining quota
	if err := model.PostConsumeTokenQuota(ctx, tokenId, quotaDelta); err != nil {
		lg.Error("CRITICAL: upstream request was sent but billing failed - unbilled request detected",
			zap.Error(err),
			zap.Int("tokenId", tokenId),
			zap.Int("userId", logEntry.UserId),
			zap.Int("channelId", logEntry.ChannelId),
			zap.String("model", logEntry.ModelName),
			zap.Int64("quotaDelta", quotaDelta),
			zap.Int64("totalQuota", totalQuota))
		metrics.GlobalRecorder.RecordBillingError("database_error", "post_consume_token_quota_with_log", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		billingSuccess = false
	}
	if err := model.CacheUpdateUserQuota(ctx, logEntry.UserId); err != nil {
		lg.Warn("user quota cache update failed - billing completed successfully",
			zap.Error(err),
			zap.Int("userId", logEntry.UserId),
			zap.Int("channelId", logEntry.ChannelId),
			zap.String("model", logEntry.ModelName),
			zap.Int64("totalQuota", totalQuota),
			zap.String("note", "database billing succeeded, cache will be refreshed on next request"))
		metrics.GlobalRecorder.RecordBillingError("cache_error", "update_user_quota_cache", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		billingSuccess = false
	}

	// Force quota onto log entry for consistency
	logEntry.Quota = int(totalQuota)

	// If a provisional log entry was created at pre-consume time, reconcile it
	// with the final billing data instead of creating a new log entry.
	var provLogID int
	if len(provisionalLogId) > 0 {
		provLogID = provisionalLogId[0]
	}
	if provLogID > 0 {
		if err := model.ReconcileConsumeLogDetailed(ctx, provLogID, model.ConsumeLogReconcileDetail{
			FinalQuota:         totalQuota,
			Content:            logEntry.Content,
			PromptTokens:       logEntry.PromptTokens,
			CompletionTokens:   logEntry.CompletionTokens,
			CachedPromptTokens: logEntry.CachedPromptTokens,
			ElapsedTime:        logEntry.ElapsedTime,
			Metadata:           logEntry.Metadata,
		}); err != nil {
			lg.Error("failed to reconcile provisional log, falling back to new log entry",
				zap.Error(err), zap.Int("provisional_log_id", provLogID))
			model.RecordConsumeLog(ctx, logEntry)
		}
	} else {
		model.RecordConsumeLog(ctx, logEntry)
	}

	// Update aggregates only when there is actual consumption.
	// Zero totalQuota is allowed (e.g., free groups or zero ratios) and should not be treated as an error.
	if totalQuota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(logEntry.UserId, totalQuota)
		model.UpdateChannelUsedQuota(logEntry.ChannelId, totalQuota)
	} else if totalQuota < 0 {
		// Negative consumption should never happen; flag as error for diagnostics.
		lg.Error("invalid negative totalQuota consumed",
			zap.Int64("total_quota", totalQuota),
			zap.Int("user_id", logEntry.UserId),
			zap.Int("channel_id", logEntry.ChannelId),
			zap.String("model_name", logEntry.ModelName))
		metrics.GlobalRecorder.RecordBillingError("calculation_error", "post_consume_with_log", logEntry.UserId, logEntry.ChannelId, logEntry.ModelName)
		billingSuccess = false
	} // totalQuota == 0: do nothing (free request)

	metrics.GlobalRecorder.RecordBillingOperation(billingStartTime, "post_consume_with_log", billingSuccess, logEntry.UserId, logEntry.ChannelId, logEntry.ModelName, float64(totalQuota))
}

func ReturnPreConsumedQuota(ctx context.Context, preConsumedQuota int64, tokenId int) {
	if preConsumedQuota == 0 {
		return
	}
	lg := logger.FromContext(ctx)
	// Return pre-consumed quota synchronously; callers should wrap this in a lifecycle-managed goroutine
	// if they do not want to block the handler. This ensures graceful drain can account for it.
	if err := model.PostConsumeTokenQuota(ctx, tokenId, -preConsumedQuota); err != nil {
		lg.Warn("failed to return pre-consumed quota - cleanup operation failed",
			zap.Error(err),
			zap.Int("tokenId", tokenId),
			zap.Int64("preConsumedQuota", preConsumedQuota),
			zap.String("note", "main billing already completed successfully"))
	}
}

// (Legacy wrapper PostConsumeQuota removed) callers must build model.Log and call PostConsumeQuotaWithLog.

// QuotaConsumeDetail encapsulates all parameters for detailed quota consumption billing
type QuotaConsumeDetail struct {
	Ctx              context.Context
	TokenId          int
	QuotaDelta       int64
	TotalQuota       int64
	UserId           int
	ChannelId        int
	PromptTokens     int
	CompletionTokens int
	ModelRatio       float64
	GroupRatio       float64
	// OriginModelName is the model name as requested by the client before mapping.
	// ModelName is the mapped model used for billing.
	OriginModelName    string
	ModelName          string
	TokenName          string
	IsStream           bool
	StartTime          time.Time
	SystemPromptReset  bool
	CompletionRatio    float64
	ToolsCost          int64
	CachedPromptTokens int
	CacheWrite5mTokens int
	CacheWrite1hTokens int
	Metadata           model.LogMetadata
	// Explicit IDs propagated from gin.Context
	RequestId string
	TraceId   string
	// ProvisionalLogId is the database ID of the provisional log entry created at
	// pre-consume time. When non-zero, post-billing reconciles this entry instead
	// of creating a new one.
	ProvisionalLogId int
	// UserAPIFormat is the API format the end-user requested (e.g. "chat",
	// "response_api", "claude_messages"). Derived from relaymode.String(meta.Mode).
	UserAPIFormat string
	// UpstreamAPIFormat is the upstream provider's API format (e.g. "openai",
	// "anthropic"). Derived from apitype.String(meta.APIType).
	UpstreamAPIFormat string
	// UpstreamEndpoint is the final URL sent to the upstream provider, captured
	// from meta.UpstreamRequestURL after the adaptor resolves it.
	UpstreamEndpoint string
	// ToolUsageSummary describes built-in tool invocations performed during
	// the request. When non-nil and non-empty, PostConsumeQuotaDetailed emits
	// one LogTypeTool row per invocation so the dashboard tool charts can
	// aggregate strictly on type. The originating consume log row is unchanged.
	ToolUsageSummary *model.ToolUsageSummary
}

// PostConsumeQuotaDetailed handles detailed billing for ChatCompletion and Response API requests
// This function properly logs individual prompt and completion tokens with additional metadata
// SAFETY: This function validates all inputs to prevent billing errors
func PostConsumeQuotaDetailed(detail QuotaConsumeDetail) {

	// Input validation for safety
	lg := logger.FromContext(detail.Ctx)
	if detail.Ctx == nil {
		lg.Error("PostConsumeQuotaDetailed: context is nil")
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}
	if detail.TokenId <= 0 {
		lg.Error("PostConsumeQuotaDetailed: invalid tokenId", zap.Int("token_id", detail.TokenId))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}
	if detail.UserId <= 0 {
		lg.Error("PostConsumeQuotaDetailed: invalid userId", zap.Int("user_id", detail.UserId))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}
	if detail.ChannelId <= 0 {
		lg.Error("PostConsumeQuotaDetailed: invalid channelId", zap.Int("channel_id", detail.ChannelId))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}
	if detail.PromptTokens < 0 || detail.CompletionTokens < 0 {
		lg.Error("PostConsumeQuotaDetailed: negative token counts",
			zap.Int("prompt_tokens", detail.PromptTokens),
			zap.Int("completion_tokens", detail.CompletionTokens))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}
	if detail.ModelName == "" {
		lg.Error("PostConsumeQuotaDetailed: modelName is empty")
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_detailed", detail.UserId, detail.ChannelId, detail.ModelName)
		return
	}

	var logContent string
	if detail.ToolsCost == 0 {
		logContent = fmt.Sprintf("model rate %.2f, group rate %.2f, completion rate %.2f, cached_prompt %d, cache_write_5m %d, cache_write_1h %d",
			detail.ModelRatio, detail.GroupRatio, detail.CompletionRatio, detail.CachedPromptTokens, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	} else {
		logContent = fmt.Sprintf("model rate %.2f, group rate %.2f, completion rate %.2f, tools cost %d, cached_prompt %d, cache_write_5m %d, cache_write_1h %d",
			detail.ModelRatio, detail.GroupRatio, detail.CompletionRatio, detail.ToolsCost, detail.CachedPromptTokens, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	}
	entry := &model.Log{
		UserId:             detail.UserId,
		ChannelId:          detail.ChannelId,
		PromptTokens:       detail.PromptTokens,
		CompletionTokens:   detail.CompletionTokens,
		ModelName:          detail.ModelName,
		OriginModelName:    detail.OriginModelName,
		TokenName:          detail.TokenName,
		Content:            logContent,
		IsStream:           detail.IsStream,
		ElapsedTime:        helper.CalcElapsedTime(detail.StartTime),
		SystemPromptReset:  detail.SystemPromptReset,
		CachedPromptTokens: detail.CachedPromptTokens,
		RequestId:          detail.RequestId,
		TraceId:            detail.TraceId,
	}

	metadata := model.CloneLogMetadata(detail.Metadata)
	metadata = model.AppendCacheWriteTokensMetadata(metadata, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	if detail.UserAPIFormat != "" {
		if metadata == nil {
			metadata = model.LogMetadata{}
		}
		metadata[model.LogMetadataKeyUserAPIFormat] = detail.UserAPIFormat
	}
	if detail.UpstreamAPIFormat != "" {
		if metadata == nil {
			metadata = model.LogMetadata{}
		}
		metadata[model.LogMetadataKeyUpstreamAPIFormat] = detail.UpstreamAPIFormat
	}
	if detail.UpstreamEndpoint != "" {
		if metadata == nil {
			metadata = model.LogMetadata{}
		}
		metadata[model.LogMetadataKeyUpstreamEndpoint] = detail.UpstreamEndpoint
	}
	if len(metadata) > 0 {
		entry.Metadata = metadata
	}

	lg.Debug("prepared detailed consume log",
		zap.Int("user_id", detail.UserId),
		zap.Int("channel_id", detail.ChannelId),
		zap.String("model", detail.ModelName),
		zap.Int64("quota_delta", detail.QuotaDelta),
		zap.Int64("total_quota", detail.TotalQuota),
		zap.Int("prompt_tokens", detail.PromptTokens),
		zap.Int("completion_tokens", detail.CompletionTokens),
		zap.Int("cached_prompt_tokens", detail.CachedPromptTokens),
		zap.Int("cache_write_5m_tokens", detail.CacheWrite5mTokens),
		zap.Int("cache_write_1h_tokens", detail.CacheWrite1hTokens),
		zap.Int("provisional_log_id", detail.ProvisionalLogId),
		zap.String("request_id", detail.RequestId),
		zap.String("trace_id", detail.TraceId),
	)

	postConsumeQuotaWithLogFn(detail.Ctx, detail.TokenId, detail.QuotaDelta, detail.TotalQuota, entry, detail.ProvisionalLogId)

	// Emit tool invocation log rows so dashboard tool charts (which aggregate
	// strictly on type=LogTypeTool) reflect built-in tool usage. The originating
	// model consume row remains as-is; tool rows are siblings of it.
	if detail.ToolUsageSummary != nil {
		model.RecordToolLogs(detail.Ctx, entry, detail.ToolUsageSummary)
	}
}

// Removed PostConsumeQuotaDetailedWithTraceID; use QuotaConsumeDetail.TraceId instead
