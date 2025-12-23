package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
)

// Trace represents a request tracing record with key timestamps
type Trace struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TraceId    string `json:"trace_id" gorm:"type:varchar(64);uniqueIndex;not null"` // TraceID from gin-middlewares
	URL        string `json:"url" gorm:"type:text;not null"`                         // Request URL
	Method     string `json:"method" gorm:"type:varchar(16);not null"`               // HTTP method
	BodySize   int64  `json:"body_size" gorm:"bigint;default:0"`                     // Request body size in bytes
	Status     int    `json:"status" gorm:"default:0"`                               // HTTP status code
	Timestamps string `json:"timestamps" gorm:"type:text"`                           // JSON object with timestamps
	CreatedAt  int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli;index"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// TraceTimestamps represents the structure of timestamps stored in the Trace.Timestamps field
type TraceTimestamps struct {
	RequestReceived       *int64 `json:"request_received,omitempty"`        // When request was received
	RequestForwarded      *int64 `json:"request_forwarded,omitempty"`       // When request was forwarded to upstream
	FirstUpstreamResponse *int64 `json:"first_upstream_response,omitempty"` // When first response received from upstream
	FirstClientResponse   *int64 `json:"first_client_response,omitempty"`   // When first response sent to client
	UpstreamCompleted     *int64 `json:"upstream_completed,omitempty"`      // When upstream response completed (for streaming)
	RequestCompleted      *int64 `json:"request_completed,omitempty"`       // When entire request completed
}

// Timestamp constants for consistent key naming
const (
	TimestampRequestReceived       = "request_received"
	TimestampRequestForwarded      = "request_forwarded"
	TimestampFirstUpstreamResponse = "first_upstream_response"
	TimestampFirstClientResponse   = "first_client_response"
	TimestampUpstreamCompleted     = "upstream_completed"
	TimestampRequestCompleted      = "request_completed"
)

// maxTraceURLLength guards against unbounded storage of user-provided URLs.
// Modern browsers typically cap URLs at ~2000 characters; we allow double that to
// accommodate reverse proxies injecting metadata while still preventing runaway growth.
const maxTraceURLLength = 4096

// CreateTrace creates a new trace record with initial data
func CreateTrace(ctx context.Context, traceId, url, method string, bodySize int64) (*Trace, error) {
	lg := gmw.GetLogger(ctx)
	now := time.Now().UnixMilli()

	timestamps := &TraceTimestamps{
		RequestReceived: &now,
	}

	urlToStore, truncated := enforceTraceURLLimit(url)
	if truncated {
		lg.Warn("trace url truncated to max length",
			zap.String("trace_id", traceId),
			zap.Int("original_length", len(url)),
			zap.Int("truncated_length", len(urlToStore)))
	}

	timestampsJSON, err := json.Marshal(timestamps)
	if err != nil {
		lg.Error("failed to marshal trace timestamps",
			zap.Error(err),
			zap.String("trace_id", traceId))
		return nil, errors.Wrapf(err, "failed to marshal trace timestamps for trace_id: %s", traceId)
	}

	traceRecord := &Trace{
		TraceId:    traceId,
		URL:        urlToStore,
		Method:     method,
		BodySize:   bodySize,
		Timestamps: string(timestampsJSON),
	}

	// Integrate with OpenTelemetry
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("one_api.trace_id", traceId),
			attribute.String("one_api.url", urlToStore),
			attribute.String("one_api.method", method),
			attribute.Int64("one_api.body_size", bodySize),
		)
		span.AddEvent(TimestampRequestReceived)
	}

	db := traceDBWithContext(ctx)

	if err := db.Create(traceRecord).Error; err != nil {
		lg.Error("failed to create trace record",
			zap.Error(err),
			zap.String("trace_id", traceId))
		return nil, errors.Wrapf(err, "failed to create trace record for trace_id: %s", traceId)
	}

	lg.Debug("created trace record",
		zap.String("trace_id", traceId),
		zap.String("url", urlToStore),
		zap.String("method", method))

	return traceRecord, nil
}

// UpdateTraceTimestamp updates a specific timestamp in the trace record
func UpdateTraceTimestamp(ctx *gin.Context, traceId, timestampKey string) error {
	lg := gmw.GetLogger(ctx)
	db := traceDBWithGin(ctx)
	var traceRecord Trace
	if err := db.Where("trace_id = ?", traceId).First(&traceRecord).Error; err != nil {
		// For some internal flows (e.g., channel test helper using a synthetic gin.Context),
		// trace IDs may not correspond to a persisted request trace. Treat not found as best-effort.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Debug("trace record not found for timestamp update (best-effort, skipping)",
				zap.String("trace_id", traceId),
				zap.String("timestamp_key", timestampKey))
			return nil
		}
		lg.Error("failed to query trace record for timestamp update",
			zap.Error(err),
			zap.String("trace_id", traceId),
			zap.String("timestamp_key", timestampKey))
		return errors.Wrapf(err, "failed to query trace record for timestamp update, trace_id: %s, key: %s", traceId, timestampKey)
	}

	var timestamps TraceTimestamps
	if err := json.Unmarshal([]byte(traceRecord.Timestamps), &timestamps); err != nil {
		lg.Error("failed to unmarshal trace timestamps",
			zap.Error(err),
			zap.String("trace_id", traceId))
		return errors.Wrapf(err, "failed to unmarshal trace timestamps for trace_id: %s", traceId)
	}

	now := time.Now().UnixMilli()

	// Integrate with OpenTelemetry
	if ctx != nil && ctx.Request != nil {
		span := trace.SpanFromContext(ctx.Request.Context())
		if span.IsRecording() {
			span.AddEvent(timestampKey)
		}
	}

	// Update the specific timestamp
	switch timestampKey {
	case TimestampRequestForwarded:
		timestamps.RequestForwarded = &now
	case TimestampFirstUpstreamResponse:
		timestamps.FirstUpstreamResponse = &now
	case TimestampFirstClientResponse:
		timestamps.FirstClientResponse = &now
	case TimestampUpstreamCompleted:
		timestamps.UpstreamCompleted = &now
	case TimestampRequestCompleted:
		timestamps.RequestCompleted = &now
	default:
		lg.Warn("unknown timestamp key",
			zap.String("trace_id", traceId),
			zap.String("timestamp_key", timestampKey))
		return nil
	}

	timestampsJSON, err := json.Marshal(timestamps)
	if err != nil {
		lg.Error("failed to marshal updated trace timestamps",
			zap.Error(err),
			zap.String("trace_id", traceId))
		return errors.Wrapf(err, "failed to marshal updated trace timestamps for trace_id: %s", traceId)
	}

	if err := db.Model(&traceRecord).Update("timestamps", string(timestampsJSON)).Error; err != nil {
		lg.Error("failed to update trace timestamp",
			zap.Error(err),
			zap.String("trace_id", traceId),
			zap.String("timestamp_key", timestampKey))
		return errors.Wrapf(err, "failed to update trace timestamp for trace_id: %s, key: %s", traceId, timestampKey)
	}

	lg.Debug("updated trace timestamp",
		zap.String("trace_id", traceId),
		zap.String("timestamp_key", timestampKey))

	return nil
}

// UpdateTraceStatus updates the HTTP status code for a trace
func UpdateTraceStatus(ctx context.Context, traceId string, status int) error {
	lg := gmw.GetLogger(ctx)

	// Integrate with OpenTelemetry
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attribute.Int("one_api.status", status))
	}

	// Use RowsAffected to determine if the record exists; treat 0 as best-effort no-op.
	db := traceDBWithContext(ctx)
	tx := db.Model(&Trace{}).Where("trace_id = ?", traceId).Update("status", status)
	if tx.Error != nil {
		lg.Error("failed to update trace status",
			zap.Error(tx.Error),
			zap.String("trace_id", traceId),
			zap.Int("status", status))
		return errors.Wrapf(tx.Error, "failed to update trace status for trace_id: %s", traceId)
	}
	if tx.RowsAffected == 0 {
		lg.Debug("trace record not found for status update (best-effort, skipping)",
			zap.String("trace_id", traceId),
			zap.Int("status", status))
		return nil
	}

	lg.Debug("updated trace status",
		zap.String("trace_id", traceId),
		zap.Int("status", status))

	return nil
}

// GetTraceByTraceId retrieves a trace record by trace ID using the provided context.
func GetTraceByTraceId(ctx context.Context, traceId string) (*Trace, error) {
	dbCtx := ctx
	if dbCtx == nil {
		dbCtx = context.Background()
	}
	var traceRecord Trace
	db := traceDBWithContext(dbCtx)
	if err := db.Where("trace_id = ?", traceId).First(&traceRecord).Error; err != nil {
		return nil, errors.Wrapf(err, "failed to get trace by trace_id: %s", traceId)
	}
	return &traceRecord, nil
}

// traceDBWithGin returns a gorm session suitable for trace operations. When running on
// PostgreSQL we must disable prepared statements for these queries because schema
// migrations that alter JSON/TEXT columns can invalidate cached plans. Using
// PrepareStmt=false ensures the driver issues simple protocol queries and avoids the
// "cached plan must not change result type" error.
func traceDBWithGin(ctx *gin.Context) *gorm.DB {
	var base *gorm.DB
	if ctx != nil && ctx.Request != nil {
		requestCtx := gmw.Ctx(ctx)
		if requestCtx != nil {
			requestCtx = context.WithoutCancel(requestCtx)
			base = DB.WithContext(requestCtx)
		} else {
			base = DB
		}
	} else {
		base = DB
	}
	return applyTraceDBSession(base)
}

// traceDBWithContext mirrors traceDBWithGin but accepts a standard context for callers
// outside the Gin execution flow.
func traceDBWithContext(ctx context.Context) *gorm.DB {
	if ctx != nil {
		detachedCtx := context.WithoutCancel(ctx)
		return applyTraceDBSession(DB.WithContext(detachedCtx))
	}
	return applyTraceDBSession(DB)
}

func applyTraceDBSession(db *gorm.DB) *gorm.DB {
	if !common.UsingPostgreSQL.Load() || db == nil {
		return db
	}

	session := db.Session(&gorm.Session{NewDB: true})
	if session.Config != nil {
		cfgCopy := *session.Config
		cfgCopy.PrepareStmt = false
		session.Config = &cfgCopy
	}
	return session
}

// GetTraceTimestamps parses and returns the timestamps from a trace record
func (t *Trace) GetTraceTimestamps() (*TraceTimestamps, error) {
	var timestamps TraceTimestamps
	if err := json.Unmarshal([]byte(t.Timestamps), &timestamps); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal trace timestamps for trace_id: %s", t.TraceId)
	}
	return &timestamps, nil
}

// enforceTraceURLLimit truncates URLs longer than maxTraceURLLength while preserving UTF-8 boundaries.
func enforceTraceURLLimit(raw string) (string, bool) {
	if len(raw) <= maxTraceURLLength {
		return raw, false
	}

	runes := []rune(raw)
	if len(runes) <= maxTraceURLLength {
		return raw[:maxTraceURLLength], true
	}

	return string(runes[:maxTraceURLLength]), true
}
