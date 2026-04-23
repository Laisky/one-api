package model

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/dto"
)

// Log represents a persisted usage or management entry emitted by the billing pipeline.
type Log struct {
	Id        int    `json:"id"`
	UserId    int    `json:"user_id" gorm:"index;index:idx_user_token,priority:1"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_created_at_type"`
	Type      int    `json:"type" gorm:"index:idx_created_at_type"`
	Content   string `json:"content" gorm:"type:text"`
	Username  string `json:"username" gorm:"index:index_username_model_name,priority:2;default:''"`
	TokenName string `json:"token_name" gorm:"index;index:idx_user_token,priority:2;default:''"`
	ModelName string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	// OriginModelName records the model name as requested by the client before any mapping.
	// When a channel has model mapping configured (e.g., "my-model" -> "gpt-4"),
	// this field preserves the original model name requested ("my-model") while ModelName
	// holds the mapped model used for billing ("gpt-4").
	OriginModelName   string `json:"origin_model_name" gorm:"index;default:''"`
	Quota             int    `json:"quota" gorm:"default:0;index"`             // Added index for sorting
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0;index"`     // Added index for sorting
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0;index"` // Added index for sorting
	ChannelId         int    `json:"channel" gorm:"index"`
	RequestId         string `json:"request_id" gorm:"default:''"`
	TraceId           string `json:"trace_id" gorm:"type:varchar(64);index;default:''"` // TraceID from gin-middlewares
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
	ElapsedTime       int64  `json:"elapsed_time" gorm:"default:0;index"` // Added index for sorting (unit is ms)
	IsStream          bool   `json:"is_stream" gorm:"default:false"`
	SystemPromptReset bool   `json:"system_prompt_reset" gorm:"default:false"`
	// Cached prompt tokens for cost transparency.
	CachedPromptTokens int `json:"cached_prompt_tokens" gorm:"default:0;index"`
	// Metadata holds provider-specific attributes serialized as JSON (e.g., cache write tokens).
	Metadata LogMetadata `json:"metadata,omitempty" gorm:"type:text"`
}

// LogMetadata stores structured provider-specific attributes associated with a log entry.
// It is serialized as JSON in the underlying database column to avoid schema churn when
// new adaptor-specific fields appear.
type LogMetadata map[string]any

// logSortFields enumerates whitelisted columns for log sorting.
var logSortFields = map[string]string{
	"id":                "id",
	"created_time":      "created_at",
	"created_at":        "created_at",
	"prompt_tokens":     "prompt_tokens",
	"completion_tokens": "completion_tokens",
	"quota":             "quota",
	"elapsed_time":      "elapsed_time",
}

const (
	// LogMetadataKeyCacheWriteTokens groups cache write token counts recorded for billing transparency.
	LogMetadataKeyCacheWriteTokens = "cache_write_tokens"
	// LogMetadataKeyCacheWrite5m records the count of 5-minute window cache write tokens.
	LogMetadataKeyCacheWrite5m = "ephemeral_5m"
	// LogMetadataKeyCacheWrite1h records the count of 1-hour window cache write tokens.
	LogMetadataKeyCacheWrite1h = "ephemeral_1h"
	// LogMetadataKeyToolUsage stores structured metadata about built-in tool usage and charges.
	LogMetadataKeyToolUsage = "tool_usage"
	// LogMetadataKeyProvisional marks a consume log entry as provisional (pre-consumed, awaiting reconciliation).
	// Post-billing removes this flag when the log is reconciled with actual usage.
	LogMetadataKeyProvisional = "provisional"
	// LogMetadataKeyUserAPIFormat records the API format the end-user requested (e.g. "chat", "response_api").
	LogMetadataKeyUserAPIFormat = "user_api_format"
	// LogMetadataKeyUpstreamAPIFormat records the upstream provider's API format (e.g. "openai", "anthropic").
	LogMetadataKeyUpstreamAPIFormat = "upstream_api_format"
	// LogMetadataKeyUpstreamEndpoint records the final URL sent to the upstream provider.
	LogMetadataKeyUpstreamEndpoint = "upstream_endpoint"
)

// ToolUsageEntry captures per-tool usage metadata for logging.
type ToolUsageEntry struct {
	Tool     string
	Source   string
	ServerID int
	Count    int
	Cost     int64
}

// ToolUsageSummary captures built-in tool invocation details for billing and logging purposes.
type ToolUsageSummary struct {
	TotalCost  int64            // Aggregated quota consumed by tools
	Counts     map[string]int   // Invocation counts per tool
	CostByTool map[string]int64 // Quota charged per tool
	Entries    []ToolUsageEntry // Optional detailed entries
}

// Value converts LogMetadata to a driver-compatible JSON representation.
func (m LogMetadata) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}

	payload, err := json.Marshal(map[string]any(m))
	if err != nil {
		return nil, errors.Wrap(err, "marshal log metadata")
	}
	if string(payload) == "null" {
		return nil, nil
	}
	return string(payload), nil
}

// Scan populates LogMetadata from a database value.
func (m *LogMetadata) Scan(value any) error {
	if m == nil {
		return errors.New("log metadata scan: nil receiver")
	}
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.Errorf("log metadata scan: unsupported type %T", value)
	}

	if len(data) == 0 {
		*m = nil
		return nil
	}

	decoded := make(map[string]any)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal log metadata")
	}
	if len(decoded) == 0 {
		*m = nil
		return nil
	}

	*m = LogMetadata(decoded)
	return nil
}

// CloneLogMetadata returns a shallow copy of the provided metadata map.
func CloneLogMetadata(src LogMetadata) LogMetadata {
	if len(src) == 0 {
		return nil
	}

	clone := LogMetadata{}
	maps.Copy(clone, src)
	return clone
}

// AppendCacheWriteTokensMetadata appends cache write token counts into the metadata map.
func AppendCacheWriteTokensMetadata(metadata LogMetadata, cacheWrite5m, cacheWrite1h int) LogMetadata {
	if cacheWrite5m == 0 && cacheWrite1h == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = LogMetadata{}
	}

	existing, _ := metadata[LogMetadataKeyCacheWriteTokens].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	if cacheWrite5m != 0 {
		existing[LogMetadataKeyCacheWrite5m] = cacheWrite5m
	}
	if cacheWrite1h != 0 {
		existing[LogMetadataKeyCacheWrite1h] = cacheWrite1h
	}
	if len(existing) == 0 {
		return metadata
	}

	metadata[LogMetadataKeyCacheWriteTokens] = existing
	return metadata
}

// AppendToolUsageMetadata attaches tool invocation details to the metadata map when present.
func AppendToolUsageMetadata(metadata LogMetadata, summary *ToolUsageSummary) LogMetadata {
	if summary == nil {
		return metadata
	}
	if summary.TotalCost == 0 && len(summary.Counts) == 0 && len(summary.CostByTool) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = LogMetadata{}
	}

	entry := make(map[string]any, 3)
	if summary.TotalCost != 0 {
		entry["total_cost"] = summary.TotalCost
	}
	if len(summary.Counts) > 0 {
		countsCopy := make(map[string]int, len(summary.Counts))
		maps.Copy(countsCopy, summary.Counts)
		entry["counts"] = countsCopy
	}
	if len(summary.CostByTool) > 0 {
		costCopy := make(map[string]int64, len(summary.CostByTool))
		maps.Copy(costCopy, summary.CostByTool)
		entry["cost_by_tool"] = costCopy
	}
	if len(summary.Entries) > 0 {
		entries := make([]map[string]any, 0, len(summary.Entries))
		for _, item := range summary.Entries {
			entries = append(entries, map[string]any{
				"tool":      item.Tool,
				"source":    item.Source,
				"server_id": item.ServerID,
				"count":     item.Count,
				"cost":      item.Cost,
			})
		}
		entry["entries"] = entries
	}

	metadata[LogMetadataKeyToolUsage] = entry
	return metadata
}

const (
	// LogTypeUnknown denotes an unspecified log category and should only appear in migration edge cases.
	LogTypeUnknown = iota
	// LogTypeTopup captures quota recharge operations initiated by administrators or redemption codes.
	LogTypeTopup
	// LogTypeConsume records quota deductions generated by upstream model invocations.
	LogTypeConsume
	// LogTypeManage tracks administrative changes to user profiles, quotas, or security settings.
	LogTypeManage
	// LogTypeSystem is reserved for automated system events such as welcome bonuses.
	LogTypeSystem
	// LogTypeTest stores synthetic traffic generated by channel testing utilities.
	LogTypeTest
	// LogTypeProvisional marks a pre-consumed quota log entry that is awaiting
	// post-billing reconciliation. Once reconciled, the type is changed to LogTypeConsume.
	LogTypeProvisional
)

const manageLogRedactedPlaceholder = "[REDACTED]"

var manageLogRedactionKeywords = []string{"password", "secret", "credential"}

func ensureLogContent(log *Log) {
	if log == nil {
		return
	}

	if strings.TrimSpace(log.Content) != "" {
		return
	}

	switch log.Type {
	case LogTypeTopup:
		log.Content = buildTopupContent(log)
	case LogTypeConsume, LogTypeProvisional:
		log.Content = buildConsumeContent(log)
	case LogTypeManage:
		log.Content = buildManageFallbackContent(log)
	case LogTypeSystem:
		log.Content = buildSystemContent(log)
	case LogTypeTest:
		log.Content = buildTestContent(log)
	default:
		log.Content = buildGenericContent(log)
	}
}

func buildManageLogContent(field string, previous any, next any, note string) string {
	cleanField := strings.TrimSpace(field)
	if cleanField == "" {
		cleanField = "unspecified_field"
	}

	prevValue := sanitizeManageValue(cleanField, previous)
	nextValue := sanitizeManageValue(cleanField, next)
	content := fmt.Sprintf("Field %s changed from %s to %s", cleanField, prevValue, nextValue)
	if trimmedNote := strings.TrimSpace(note); trimmedNote != "" {
		content = fmt.Sprintf("%s (%s)", content, trimmedNote)
	}
	return content
}

func sanitizeManageValue(field string, value any) string {
	lowered := strings.ToLower(field)
	for _, keyword := range manageLogRedactionKeywords {
		if strings.Contains(lowered, keyword) {
			return manageLogRedactedPlaceholder
		}
	}

	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return "<empty>"
	}
	return text
}

func buildManageFallbackContent(log *Log) string {
	details := []string{}
	if log.UserId != 0 {
		details = append(details, fmt.Sprintf("user_id=%d", log.UserId))
	}
	if log.RequestId != "" {
		details = append(details, fmt.Sprintf("request_id=%s", log.RequestId))
	}
	if log.TraceId != "" {
		details = append(details, fmt.Sprintf("trace_id=%s", log.TraceId))
	}
	if len(details) == 0 {
		return "Management action recorded."
	}
	return fmt.Sprintf("Management action recorded (%s).", strings.Join(details, ", "))
}

func buildTopupContent(log *Log) string {
	details := []string{}
	if log.Quota != 0 {
		details = append(details, fmt.Sprintf("amount=%s", common.LogQuota(int64(log.Quota))))
	}
	if log.TokenName != "" {
		details = append(details, fmt.Sprintf("token=%s", log.TokenName))
	}
	if log.ChannelId != 0 {
		details = append(details, fmt.Sprintf("channel_id=%d", log.ChannelId))
	}
	if len(details) == 0 {
		return "Top-up event recorded."
	}
	return fmt.Sprintf("Top-up event recorded: %s.", strings.Join(details, ", "))
}

func buildConsumeContent(log *Log) string {
	details := []string{}
	if log.ModelName != "" {
		details = append(details, fmt.Sprintf("model=%s", log.ModelName))
	}
	if log.ChannelId != 0 {
		details = append(details, fmt.Sprintf("channel_id=%d", log.ChannelId))
	}
	if log.Quota != 0 {
		details = append(details, fmt.Sprintf("quota=%s", common.LogQuota(int64(log.Quota))))
	}
	if log.PromptTokens != 0 || log.CompletionTokens != 0 {
		details = append(details, fmt.Sprintf("tokens=%d prompt/%d completion", log.PromptTokens, log.CompletionTokens))
	}
	if len(details) == 0 {
		return "Model invocation recorded."
	}
	return fmt.Sprintf("Model invocation recorded: %s.", strings.Join(details, ", "))
}

func buildSystemContent(log *Log) string {
	details := []string{}
	if log.Username != "" {
		details = append(details, fmt.Sprintf("username=%s", log.Username))
	} else if log.UserId != 0 {
		details = append(details, fmt.Sprintf("user_id=%d", log.UserId))
	}
	if log.Quota != 0 {
		details = append(details, fmt.Sprintf("quota=%s", common.LogQuota(int64(log.Quota))))
	}
	if log.ModelName != "" {
		details = append(details, fmt.Sprintf("model=%s", log.ModelName))
	}
	if len(details) == 0 {
		return "System event recorded."
	}
	return fmt.Sprintf("System event recorded: %s.", strings.Join(details, ", "))
}

func buildTestContent(log *Log) string {
	details := []string{}
	if log.ModelName != "" {
		details = append(details, fmt.Sprintf("model=%s", log.ModelName))
	}
	if log.ChannelId != 0 {
		details = append(details, fmt.Sprintf("channel_id=%d", log.ChannelId))
	}
	if log.ElapsedTime != 0 {
		details = append(details, fmt.Sprintf("elapsed=%dms", log.ElapsedTime))
	}
	if log.PromptTokens != 0 || log.CompletionTokens != 0 {
		details = append(details, fmt.Sprintf("tokens=%d prompt/%d completion", log.PromptTokens, log.CompletionTokens))
	}
	if log.Quota != 0 {
		details = append(details, fmt.Sprintf("quota=%s", common.LogQuota(int64(log.Quota))))
	}
	if len(details) == 0 {
		return "Channel test executed."
	}
	return fmt.Sprintf("Channel test executed: %s.", strings.Join(details, ", "))
}

func buildGenericContent(log *Log) string {
	details := []string{fmt.Sprintf("type=%d", log.Type)}
	if log.UserId != 0 {
		details = append(details, fmt.Sprintf("user_id=%d", log.UserId))
	}
	if log.RequestId != "" {
		details = append(details, fmt.Sprintf("request_id=%s", log.RequestId))
	}
	if log.TraceId != "" {
		details = append(details, fmt.Sprintf("trace_id=%s", log.TraceId))
	}
	return fmt.Sprintf("Log entry recorded (%s).", strings.Join(details, ", "))
}

// excludeProvisionalScope adds a WHERE condition that filters out provisional
// (pre-consumed, not yet reconciled) log entries so they don't appear in user-facing queries.
func excludeProvisionalScope(tx *gorm.DB) *gorm.DB {
	return tx.Where("type != ?", LogTypeProvisional)
}

// GetLogOrderClause converts frontend sort preferences into a SQL ORDER clause.
func GetLogOrderClause(sortBy string, sortOrder string) string {
	return ValidateOrderClause(sortBy, sortOrder, logSortFields, "id desc")
}

// BUG: Session‑related variables like RequestId and TraceId are kept in `gin.Context`.
// However, logging can happen after the request’s Gin context has been closed,
// so `recordLogHelper` receives a standard `context.Context` rather than
// the original `gin.Context`. Consequently, many context values are lost.
// We need a systematic audit of every function that attempts to fetch values
// from `context.Context` and change the design to pass those values explicitly
// as parameters, rather than trying to read them from a generic `context.Context`.
func recordLogHelper(ctx context.Context, log *Log) {
	lg := logger.FromContext(ctx)
	// IDs must be pre-populated by the caller from gin.Context
	ensureLogContent(log)

	err := LOG_DB.Create(log).Error
	if err != nil {
		// For billing logs (consume type), this is critical as it means we sent upstream request but failed to log it
		if log.Type == LogTypeConsume {
			lg.Error("failed to record billing log - audit trail incomplete",
				zap.Error(err),
				zap.Int("userId", log.UserId),
				zap.Int("channelId", log.ChannelId),
				zap.String("model", log.ModelName),
				zap.Int("quota", log.Quota),
				zap.String("requestId", log.RequestId),
				zap.String("note", "billing completed successfully but log recording failed"))
		} else {
			lg.Error("failed to record log", zap.Error(err))
		}

		return
	}

	lg.Info("record log",
		zap.Int("user_id", log.UserId),
		zap.String("username", log.Username),
		zap.Int64("created_at", log.CreatedAt),
		zap.Int("type", log.Type),
		zap.String("content", log.Content),
		zap.String("request_id", log.RequestId),
		zap.String("trace_id", log.TraceId),
		zap.Int("quota", log.Quota),
		zap.Int("prompt_tokens", log.PromptTokens),
		zap.Int("completion_tokens", log.CompletionTokens),
	)
}

// recordLogHelperWithTraceID removed: callers must set IDs directly on log

// RecordLog persists a generic log entry for the provided user and type.
func RecordLog(ctx context.Context, userId int, logType int, content string) {
	if logType == LogTypeConsume && !config.IsLogConsumeEnabled() {
		return
	}
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	recordLogHelper(ctx, log)
}

// RecordLogWithIDs records a generic log with explicit requestId/traceId.
func RecordLogWithIDs(ctx context.Context, userId int, logType int, content string, requestId string, traceId string) {
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      logType,
		Content:   content,
		RequestId: requestId,
		TraceId:   traceId,
	}
	recordLogHelper(ctx, log)
}

// RecordManageLog captures administrative modifications including the affected field and value changes.
func RecordManageLog(ctx context.Context, userId int, field string, previous any, next any, note string) {
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      LogTypeManage,
		Content:   buildManageLogContent(field, previous, next, note),
	}
	recordLogHelper(ctx, log)
}

// RecordTopupLog writes a quota recharge entry with the provided description and amount.
func RecordTopupLog(ctx context.Context, userId int, content string, quota int) {
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Quota:     quota,
	}
	recordLogHelper(ctx, log)
}

// RecordTopupLogWithIDs records a topup log with explicit requestId/traceId.
func RecordTopupLogWithIDs(ctx context.Context, userId int, content string, quota int, requestId string, traceId string) {
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Quota:     quota,
		RequestId: requestId,
		TraceId:   traceId,
	}
	recordLogHelper(ctx, log)
}

// RecordConsumeLog stores a model consumption log and populates audit fields automatically.
func RecordConsumeLog(ctx context.Context, log *Log) {
	if !config.IsLogConsumeEnabled() {
		return
	}
	log.Username = GetUsernameById(log.UserId)
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeConsume
	recordLogHelper(ctx, log)
}

// RecordProvisionalConsumeLog creates a consume log entry at pre-consume time
// with an estimated quota amount. The entry is marked as provisional via metadata
// so that post-billing can later reconcile it with the actual amount.
//
// Returns the log ID so that post-billing can update the record.
// If the log cannot be created (e.g., logging disabled), returns 0.
func RecordProvisionalConsumeLog(ctx context.Context, log *Log, estimatedQuota int64) int {
	if estimatedQuota <= 0 {
		return 0
	}
	if !config.IsLogConsumeEnabled() {
		return 0
	}

	log.Username = GetUsernameById(log.UserId)
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeProvisional
	log.Quota = int(estimatedQuota)

	// Mark as provisional in metadata so it's visible in audit
	if log.Metadata == nil {
		log.Metadata = make(LogMetadata)
	}
	log.Metadata[LogMetadataKeyProvisional] = true

	// Append "[provisional]" to content for visibility
	if log.Content == "" {
		log.Content = "[provisional] pre-consumed quota, awaiting reconciliation"
	} else {
		log.Content = "[provisional] " + log.Content
	}

	ensureLogContent(log)
	lg := logger.FromContext(ctx)
	err := LOG_DB.Create(log).Error
	if err != nil {
		lg.Error("failed to record provisional billing log",
			zap.Error(err),
			zap.Int("userId", log.UserId),
			zap.Int("channelId", log.ChannelId),
			zap.String("model", log.ModelName),
			zap.Int("quota", log.Quota),
			zap.String("requestId", log.RequestId),
		)
		return 0
	}

	lg.Debug("recorded provisional consume log",
		zap.Int("log_id", log.Id),
		zap.Int("user_id", log.UserId),
		zap.Int64("estimated_quota", estimatedQuota),
		zap.String("model", log.ModelName),
		zap.String("request_id", log.RequestId),
	)

	return log.Id
}

// ConsumeLogReconcileDetail captures the finalized fields written back into a
// provisional consume log during post-billing reconciliation.
type ConsumeLogReconcileDetail struct {
	FinalQuota         int64
	Content            string
	PromptTokens       int
	CompletionTokens   int
	CachedPromptTokens int
	ElapsedTime        int64
	Metadata           LogMetadata
}

// ReconcileConsumeLog updates a provisional consume log entry with the final
// billing details after post-billing completes. This removes the provisional
// marker and updates quota, content, tokens, and elapsed_time.
func ReconcileConsumeLog(ctx context.Context, logID int, finalQuota int64, content string, promptTokens int, completionTokens int, elapsedTime int64, metadata LogMetadata) error {
	return ReconcileConsumeLogDetailed(ctx, logID, ConsumeLogReconcileDetail{
		FinalQuota:       finalQuota,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ElapsedTime:      elapsedTime,
		Metadata:         metadata,
	})
}

// ReconcileConsumeLogDetailed updates a provisional consume log entry with the
// finalized billing data, including cached token columns used by the log UI.
func ReconcileConsumeLogDetailed(ctx context.Context, logID int, detail ConsumeLogReconcileDetail) error {
	if logID <= 0 {
		return nil // No provisional log to reconcile (e.g., logging disabled)
	}

	lg := logger.FromContext(ctx)

	// Log context state for diagnostics — helps identify context cancellation issues
	if ctx.Err() != nil {
		lg.Debug("ReconcileConsumeLog called with errored context",
			zap.Error(ctx.Err()),
			zap.Int("log_id", logID),
		)
	}

	updates := map[string]any{
		"type":                 LogTypeConsume,
		"quota":                int(detail.FinalQuota),
		"content":              detail.Content,
		"prompt_tokens":        detail.PromptTokens,
		"completion_tokens":    detail.CompletionTokens,
		"cached_prompt_tokens": detail.CachedPromptTokens,
		"elapsed_time":         detail.ElapsedTime,
	}

	// Remove the provisional flag from metadata
	metadata := detail.Metadata
	if metadata == nil {
		metadata = make(LogMetadata)
	}
	delete(metadata, LogMetadataKeyProvisional)
	metadataJSON, err := metadata.Value()
	if err != nil {
		lg.Warn("failed to serialize metadata for log reconciliation", zap.Error(err))
	} else {
		updates["metadata"] = metadataJSON
	}

	if err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("id = ?", logID).
		Updates(updates).Error; err != nil {
		lg.Error("CRITICAL: failed to reconcile provisional consume log",
			zap.Error(err),
			zap.Int("log_id", logID),
			zap.Int64("final_quota", detail.FinalQuota),
			zap.NamedError("ctx_err", ctx.Err()),
		)
		return errors.Wrapf(err, "failed to reconcile consume log: id=%d", logID)
	}

	lg.Debug("reconciled provisional consume log",
		zap.Int("log_id", logID),
		zap.Int64("final_quota", detail.FinalQuota),
		zap.Int("prompt_tokens", detail.PromptTokens),
		zap.Int("completion_tokens", detail.CompletionTokens),
		zap.Int("cached_prompt_tokens", detail.CachedPromptTokens),
	)
	return nil
}

// RecordConsumeLogWithTraceID removed: pass IDs directly and call RecordConsumeLog

// RecordTestLog persists a synthetic channel test log entry.
func RecordTestLog(ctx context.Context, log *Log) {
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeTest
	recordLogHelper(ctx, log)
}

// RecordTestLogWithIDs records a test log with explicit requestId/traceId.
func RecordTestLogWithIDs(ctx context.Context, log *Log, requestId string, traceId string) {
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeTest
	log.RequestId = requestId
	log.TraceId = traceId
	recordLogHelper(ctx, log)
}

// UpdateConsumeLogByID performs a partial update on an existing consume log entry.
// Parameters:
//   - ctx: request context used for cancellation propagation.
//   - logID: identifier of the log row to update.
//   - updates: column/value pairs to apply. When empty, the function is a no-op.
//
// Returns an error if the update fails.
var allowedConsumeLogUpdateFields = map[string]struct{}{
	"quota":                {},
	"content":              {},
	"elapsed_time":         {},
	"prompt_tokens":        {},
	"completion_tokens":    {},
	"cached_prompt_tokens": {},
	"metadata":             {},
}

func UpdateConsumeLogByID(ctx context.Context, logID int, updates map[string]any) error {
	if logID <= 0 {
		return errors.Errorf("log id must be positive: %d", logID)
	}
	if len(updates) == 0 {
		return nil
	}

	for field := range updates {
		if _, ok := allowedConsumeLogUpdateFields[field]; !ok {
			return errors.Errorf("unsupported consume log update field: %s", field)
		}
	}

	if err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("id = ?", logID).
		Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "failed to update consume log: id=%d", logID)
	}
	return nil
}

// GetAllLogs retrieves logs filtered by type, time, model, username, token, and channel with pagination support.
func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, sortBy string, sortOrder string) (logs []*Log, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("type = ?", logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	tx = excludeProvisionalScope(tx)

	// Apply sorting with timeout for sorting queries
	orderClause := GetLogOrderClause(sortBy, sortOrder)
	if sortBy != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = tx.WithContext(ctx).Order(orderClause).Limit(num).Offset(startIdx).Find(&logs).Error
	} else {
		err = tx.Order(orderClause).Limit(num).Offset(startIdx).Find(&logs).Error
	}
	if err != nil {
		return nil, errors.Wrap(err, "get all logs")
	}
	return logs, nil
}

// GetAllLogsCount returns the total number of logs matching the supplied filters.
func GetAllLogsCount(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int) (count int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("type = ?", logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	tx = excludeProvisionalScope(tx)

	err = tx.Model(&Log{}).Count(&count).Error
	if err != nil {
		return 0, errors.Wrap(err, "count all logs")
	}
	return count, nil
}

// GetUserLogs lists logs belonging to a specific user with optional filtering and ordering.
func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, sortBy string, sortOrder string) (logs []*Log, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("user_id = ? and type = ?", userId, logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	tx = excludeProvisionalScope(tx)

	// Apply sorting with timeout for sorting queries
	orderClause := GetLogOrderClause(sortBy, sortOrder)
	if sortBy != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = tx.WithContext(ctx).Order(orderClause).Limit(num).Offset(startIdx).Find(&logs).Error
	} else {
		err = tx.Order(orderClause).Limit(num).Offset(startIdx).Find(&logs).Error
	}
	if err != nil {
		return nil, errors.Wrapf(err, "get user %d logs", userId)
	}
	return logs, nil
}

// GetUserLogsCount provides the number of logs for a user that satisfy the given filters.
func GetUserLogsCount(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string) (count int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("user_id = ? and type = ?", userId, logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	tx = excludeProvisionalScope(tx)

	err = tx.Model(&Log{}).Count(&count).Error
	if err != nil {
		return 0, errors.Wrapf(err, "count user %d logs", userId)
	}
	return count, nil
}

// SearchAllLogs performs a keyword search across all log entries with pagination.
func SearchAllLogs(keyword string, startIdx int, num int, sortBy string, sortOrder string) (logs []*Log, total int64, err error) {
	db := excludeProvisionalScope(LOG_DB.Model(&Log{}))
	if keyword != "" {
		db = db.Where("content LIKE ?", "%"+keyword+"%")
	}
	orderClause := GetLogOrderClause(sortBy, sortOrder)
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "search all logs")
	}
	return logs, total, nil
}

// SearchUserLogs searches logs owned by a specific user using a keyword filter.
func SearchUserLogs(userId int, keyword string, startIdx int, num int, sortBy string, sortOrder string) (logs []*Log, total int64, err error) {
	db := excludeProvisionalScope(LOG_DB.Model(&Log{}).Where("user_id = ?", userId))
	if keyword != "" {
		db = db.Where("content LIKE ?", "%"+keyword+"%")
	}
	orderClause := GetLogOrderClause(sortBy, sortOrder)
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, errors.Wrapf(err, "search user %d logs", userId)
	}
	return logs, total, nil
}

// SumUsedQuota aggregates quota consumption over matching logs, scoped by model, user, token, or channel.
func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int) (quota int64) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL.Load() {
		ifnull = "COALESCE"
	}
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf("%s(sum(quota),0)", ifnull))
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&quota)
	return quota
}

// SumUsedToken returns the total number of prompt and completion tokens consumed within the filter scope.
func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL.Load() {
		ifnull = "COALESCE"
	}
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf("%s(sum(prompt_tokens),0) + %s(sum(completion_tokens),0)", ifnull, ifnull))
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

// DeleteOldLog removes log entries older than the provided timestamp and returns the number deleted.
func DeleteOldLog(targetTimestamp int64) (int64, error) {
	result := LOG_DB.Where("created_at < ?", targetTimestamp).Delete(&Log{})
	return result.RowsAffected, result.Error
}

// GetLogById retrieves a log entry by its ID
func GetLogById(id int) (*Log, error) {
	var log Log
	if err := LOG_DB.Where("id = ?", id).First(&log).Error; err != nil {
		return nil, errors.Wrapf(err, "get log by id %d", id)
	}
	return &log, nil
}

// dayAggregationSelect returns the SQL expression that normalizes log timestamps
// into YYYY-MM-DD strings, accounting for the configured database engine.
func dayAggregationSelect() string {
	if common.UsingPostgreSQL.Load() {
		return "TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD') as day"
	}

	if common.UsingSQLite.Load() {
		return "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch')) as day"
	}

	return "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d') as day"
}

// SearchLogsByDayAndModel returns per-day, per-model aggregates for logs in the
// half-open timestamp range [start, endExclusive). `start` and `endExclusive`
// are Unix seconds.
func SearchLogsByDayAndModel(userId, start, endExclusive int) (LogStatistics []*dto.LogStatistic, err error) {
	groupSelect := dayAggregationSelect()

	// If userId is 0, query all users (site-wide statistics)
	var query string
	var args []any

	// We switch to explicit >= start AND < endExclusive to avoid relying on BETWEEN inclusive semantics.
	if userId == 0 {
		query = `
			SELECT ` + groupSelect + `,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND created_at >= ? AND created_at < ?
			GROUP BY day, model_name
			ORDER BY day, model_name
		`
		args = []any{start, endExclusive}
	} else {
		query = `
			SELECT ` + groupSelect + `,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND user_id= ?
			AND created_at >= ? AND created_at < ?
			GROUP BY day, model_name
			ORDER BY day, model_name
		`
		args = []any{userId, start, endExclusive}
	}

	err = LOG_DB.Raw(query, args...).Scan(&LogStatistics).Error
	if err != nil {
		return nil, errors.Wrap(err, "search logs by day and model")
	}
	return LogStatistics, nil
}

// SearchLogsByDayAndUser returns per-day, per-user aggregates for logs within
// the half-open timestamp range [start, endExclusive).
func SearchLogsByDayAndUser(userId, start, endExclusive int) ([]*dto.LogStatisticByUser, error) {
	groupSelect := dayAggregationSelect()

	var query string
	var args []any

	if userId == 0 {
		query = `
			SELECT ` + groupSelect + `,
			username, user_id,
			count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND created_at >= ? AND created_at < ?
			GROUP BY day, username, user_id
			ORDER BY day, username
		`
		args = []any{start, endExclusive}
	} else {
		query = `
			SELECT ` + groupSelect + `,
			username, user_id,
			count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND user_id = ?
			AND created_at >= ? AND created_at < ?
			GROUP BY day, username, user_id
			ORDER BY day, username
		`
		args = []any{userId, start, endExclusive}
	}

	var stats []*dto.LogStatisticByUser
	err := LOG_DB.Raw(query, args...).Scan(&stats).Error
	if err != nil {
		return nil, errors.Wrap(err, "search logs by day and user")
	}
	return stats, nil
}

// SearchLogsByDayAndToken returns per-day, per-token aggregates (scoped by
// username to disambiguate tokens with identical names) for the half-open
// range [start, endExclusive).
func SearchLogsByDayAndToken(userId, start, endExclusive int) ([]*dto.LogStatisticByToken, error) {
	groupSelect := dayAggregationSelect()

	var query string
	var args []any

	if userId == 0 {
		query = `
			SELECT ` + groupSelect + `,
			COALESCE(token_name, '') as token_name,
			username, user_id,
			count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND created_at >= ? AND created_at < ?
			GROUP BY day, token_name, username, user_id
			ORDER BY day, username, token_name
		`
		args = []any{start, endExclusive}
	} else {
		query = `
			SELECT ` + groupSelect + `,
			COALESCE(token_name, '') as token_name,
			username, user_id,
			count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens,
			sum(cached_prompt_tokens) as cached_prompt_tokens,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
			sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota
			FROM logs
			WHERE type=2
			AND user_id = ?
			AND created_at >= ? AND created_at < ?
			GROUP BY day, token_name, username, user_id
			ORDER BY day, username, token_name
		`
		args = []any{userId, start, endExclusive}
	}

	var stats []*dto.LogStatisticByToken
	err := LOG_DB.Raw(query, args...).Scan(&stats).Error
	if err != nil {
		return nil, errors.Wrap(err, "search logs by day and token")
	}
	return stats, nil
}
