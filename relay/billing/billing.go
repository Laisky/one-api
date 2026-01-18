package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
)

// PostConsumeQuotaWithLog is the unified billing entry that consumes quota, updates caches,
// records a consume log, and updates user/channel aggregates.
// Caller must provide a pre-filled log entry (including RequestId/TraceId if desired).
func PostConsumeQuotaWithLog(ctx context.Context, tokenId int, quotaDelta int64, totalQuota int64, logEntry *model.Log) {
	billingStartTime := time.Now()
	billingSuccess := true
	lg := logger.FromContext(ctx)

	if ctx == nil || logEntry == nil {
		lg.Error("PostConsumeQuotaWithLog: invalid args", zap.Bool("ctx_nil", ctx == nil), zap.Bool("log_nil", logEntry == nil))
		metrics.GlobalRecorder.RecordBillingError("validation_error", "post_consume_with_log", 0, 0, "")
		return
	}
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
	model.RecordConsumeLog(ctx, logEntry)

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
	Ctx                    context.Context
	TokenId                int
	QuotaDelta             int64
	TotalQuota             int64
	UserId                 int
	ChannelId              int
	PromptTokens           int
	CompletionTokens       int
	ModelRatio             float64
	GroupRatio             float64
	ModelName              string
	TokenName              string
	IsStream               bool
	StartTime              time.Time
	SystemPromptReset      bool
	CompletionRatio        float64
	ToolsCost              int64
	CachedPromptTokens     int
	CachedCompletionTokens int
	CacheWrite5mTokens     int
	CacheWrite1hTokens     int
	Metadata               model.LogMetadata
	// Explicit IDs propagated from gin.Context
	RequestId string
	TraceId   string
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
		logContent = fmt.Sprintf("model rate %.2f, group rate %.2f, completion rate %.2f, cached_prompt %d, cached_completion %d, cache_write_5m %d, cache_write_1h %d",
			detail.ModelRatio, detail.GroupRatio, detail.CompletionRatio, detail.CachedPromptTokens, detail.CachedCompletionTokens, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	} else {
		logContent = fmt.Sprintf("model rate %.2f, group rate %.2f, completion rate %.2f, tools cost %d, cached_prompt %d, cached_completion %d, cache_write_5m %d, cache_write_1h %d",
			detail.ModelRatio, detail.GroupRatio, detail.CompletionRatio, detail.ToolsCost, detail.CachedPromptTokens, detail.CachedCompletionTokens, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	}
	entry := &model.Log{
		UserId:                 detail.UserId,
		ChannelId:              detail.ChannelId,
		PromptTokens:           detail.PromptTokens,
		CompletionTokens:       detail.CompletionTokens,
		ModelName:              detail.ModelName,
		TokenName:              detail.TokenName,
		Content:                logContent,
		IsStream:               detail.IsStream,
		ElapsedTime:            helper.CalcElapsedTime(detail.StartTime),
		SystemPromptReset:      detail.SystemPromptReset,
		CachedPromptTokens:     detail.CachedPromptTokens,
		CachedCompletionTokens: detail.CachedCompletionTokens,
		RequestId:              detail.RequestId,
		TraceId:                detail.TraceId,
	}

	metadata := model.CloneLogMetadata(detail.Metadata)
	metadata = model.AppendCacheWriteTokensMetadata(metadata, detail.CacheWrite5mTokens, detail.CacheWrite1hTokens)
	if len(metadata) > 0 {
		entry.Metadata = metadata
	}

	PostConsumeQuotaWithLog(detail.Ctx, detail.TokenId, detail.QuotaDelta, detail.TotalQuota, entry)
}

// Removed PostConsumeQuotaDetailedWithTraceID; use QuotaConsumeDetail.TraceId instead
