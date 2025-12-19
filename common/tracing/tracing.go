package tracing

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
)

// traceIDFromContext extracts the OpenTelemetry trace ID from a context when available.
func traceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String()
	}

	return ""
}

// GetTraceID extracts the TraceID from gin context using gin-middlewares
func GetTraceID(c *gin.Context) string {
	if traceID := traceIDFromContext(gmw.Ctx(c)); traceID != "" {
		return traceID
	}

	traceID, err := gmw.TraceID(c)
	if err != nil {
		gmw.GetLogger(c).Warn("failed to get trace ID from gin-middlewares", zap.Error(err))
		// Fallback to empty string - this should not happen in normal operation
		return ""
	}
	return traceID.String()
}

// GetTraceIDFromContext extracts TraceID from standard context
// This is useful when we only have context.Context and not gin.Context
func GetTraceIDFromContext(ctx context.Context) string {
	if traceID := traceIDFromContext(ctx); traceID != "" {
		return traceID
	}

	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		return GetTraceID(ginCtx)
	}
	logger.Logger.Warn("failed to get gin context from standard context for trace ID extraction")
	return ""
}

// RecordTraceStart creates a new trace record when a request starts
func RecordTraceStart(c *gin.Context) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(zap.String("trace_id", traceID))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping trace record creation")
		return
	}

	url := c.Request.URL.String()
	method := c.Request.Method
	bodySize := max(c.Request.ContentLength, 0)

	ctx := gmw.Ctx(c)
	// propagate tagged logger downstream
	ctx = gmw.SetLogger(ctx, lg)
	_, err := model.CreateTrace(ctx, traceID, url, method, bodySize)
	if err != nil {
		lg.Error("failed to create trace record",
			zap.Error(err))
	}
}

// RecordTraceTimestamp updates a specific timestamp in the trace record
func RecordTraceTimestamp(c *gin.Context, timestampKey string) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(
		zap.String("trace_id", traceID),
		zap.String("timestamp_key", timestampKey),
	)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping timestamp update")
		return
	}

	err := model.UpdateTraceTimestamp(c, traceID, timestampKey)
	if err != nil {
		lg.Error("failed to update trace timestamp", zap.Error(err))
	}
}

// RecordTraceTimestampFromContext updates a timestamp using standard context
// func RecordTraceTimestampFromContext(ctx context.Context, timestampKey string) {
// 	traceID := GetTraceIDFromContext(ctx)
// 	if traceID == "" {
// 		logger.Logger.Warn("empty trace ID from context, skipping timestamp update",
// 			zap.String("timestamp_key", timestampKey))
// 		return
// 	}

// 	// Best-effort update; model handles not-found quietly.
// 	if err := model.UpdateTraceTimestamp(ctx, traceID, timestampKey); err != nil {
// 		logger.Logger.Error("failed to update trace timestamp from context",
// 			zap.Error(err),
// 			zap.String("trace_id", traceID),
// 			zap.String("timestamp_key", timestampKey))
// 	}
// }

// RecordTraceStatus updates the HTTP status code for a trace
func RecordTraceStatus(c *gin.Context, status int) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(
		zap.String("trace_id", traceID),
		zap.Int("status", status),
	)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping status update")
		return
	}

	ctx := gmw.Ctx(c)
	// propagate tagged logger downstream
	ctx = gmw.SetLogger(ctx, lg)
	err := model.UpdateTraceStatus(ctx, traceID, status)
	if err != nil {
		lg.Error("failed to update trace status", zap.Error(err))
	}
}

// RecordTraceEnd marks the completion of a request and records final timestamp
func RecordTraceEnd(c *gin.Context) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(zap.String("trace_id", traceID))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping trace end recording")
		return
	}

	// Record the final timestamp
	RecordTraceTimestamp(c, model.TimestampRequestCompleted)

	// Record the final status code
	status := c.Writer.Status()
	if status == 0 {
		status = 200 // Default to 200 if no status was set
	}
	// attach logger to context for downstream status update
	ctx := gmw.Ctx(c)
	ctx = gmw.SetLogger(ctx, lg)
	_ = ctx // keep for symmetry; RecordTraceStatus will fetch its own context
	RecordTraceStatus(c, status)
}

// WithTraceID adds trace ID to structured logging fields
func WithTraceID(c *gin.Context, fields ...zap.Field) []zap.Field {
	traceID := GetTraceID(c)
	if traceID == "" {
		return fields
	}

	traceField := zap.String("trace_id", traceID)
	return append([]zap.Field{traceField}, fields...)
}

// WithTraceIDFromContext adds trace ID to structured logging fields from context
func WithTraceIDFromContext(ctx context.Context, fields ...zap.Field) []zap.Field {
	traceID := GetTraceIDFromContext(ctx)
	if traceID == "" {
		return fields
	}

	traceField := zap.String("trace_id", traceID)
	return append([]zap.Field{traceField}, fields...)
}

// GenerateChatCompletionID generates a chat completion ID from the trace ID.
// This function creates a consistent ID format across all adaptors, enabling
// request tracing through Prometheus, logging, and external systems.
//
// Format: chatcmpl-oneapi-{trace-id}
//
// For streaming responses, use the same ID for all chunks in the stream.
// For non-streaming responses, use this ID for the single response.
//
// Returns: Chat completion ID string with "chatcmpl-oneapi-" prefix
func GenerateChatCompletionID(c *gin.Context) string {
	traceID := GetTraceID(c)
	return "chatcmpl-oneapi-" + traceID
}

// GenerateChatCompletionIDFromContext generates a chat completion ID from standard context.
// This is useful when only context.Context is available (not gin.Context).
//
// Format: chatcmpl-oneapi-{trace-id}
//
// Returns: Chat completion ID string with "chatcmpl-oneapi-" prefix
func GenerateChatCompletionIDFromContext(ctx context.Context) string {
	traceID := GetTraceIDFromContext(ctx)
	return "chatcmpl-oneapi-" + traceID
}
