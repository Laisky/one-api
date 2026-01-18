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

// otelTraceIDFromContext extracts the OpenTelemetry trace ID from a context when available.
func otelTraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String()
	}

	return ""
}

// GetTraceID extracts the per-request TraceID from gin context using gin-middlewares.
//
// This TraceID is intended to be unique per incoming HTTP request. It may be derived
// from the OpenTelemetry span context, but it includes span-level information (e.g.
// span id) so it remains unique even when multiple requests share the same distributed
// OpenTelemetry trace id.
func GetTraceID(c *gin.Context) string {
	traceID, err := gmw.TraceID(c)
	if err != nil {
		gmw.GetLogger(c).Warn("failed to get trace ID from gin-middlewares", zap.Error(err))
		// Fallback to empty string - this should not happen in normal operation
		return ""
	}
	return traceID.String()
}

// GetTraceIDFromContext extracts the per-request TraceID from a standard context.
//
// When the context contains an embedded gin.Context (gmw.BackgroundCtx pattern), the
// gin-middlewares TraceID is returned.
//
// When no gin.Context is available, it falls back to the OpenTelemetry trace id.
func GetTraceIDFromContext(ctx context.Context) string {
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		return GetTraceID(ginCtx)
	}
	if traceID := otelTraceIDFromContext(ctx); traceID != "" {
		return traceID
	}
	logger.FromContext(ctx).Warn("failed to get gin context from standard context for trace ID extraction")
	return ""
}

// GetOpenTelemetryTraceID extracts the OpenTelemetry trace id from gin context when available.
//
// This is used when callers need a stable distributed trace id (not span-scoped), e.g.
// generating OpenAI-style response IDs.
func GetOpenTelemetryTraceID(c *gin.Context) string {
	return otelTraceIDFromContext(gmw.Ctx(c))
}

// GetOpenTelemetryTraceIDFromContext extracts the OpenTelemetry trace id from a standard context.
//
// Returns empty string when no OpenTelemetry span context is available.
func GetOpenTelemetryTraceIDFromContext(ctx context.Context) string {
	return otelTraceIDFromContext(ctx)
}

// RecordTraceStart creates a new trace record when a request starts
func RecordTraceStart(c *gin.Context) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping trace record creation")
		return
	}

	otelTraceID := GetOpenTelemetryTraceID(c)
	if otelTraceID != "" {
		lg.Debug("resolved trace identifiers",
			zap.String("trace_id", traceID),
			zap.String("otel_trace_id", otelTraceID),
			zap.String("url", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)
	}

	url := c.Request.URL.String()
	method := c.Request.Method
	bodySize := max(c.Request.ContentLength, 0)

	ctx := gmw.SetLogger(gmw.Ctx(c), lg)
	_, err := model.CreateTrace(ctx, traceID, url, method, bodySize)
	if err != nil {
		lg.Error("failed to create trace record",
			zap.Error(err))
	}
}

// RecordTraceTimestamp updates a specific timestamp in the trace record
func RecordTraceTimestamp(c *gin.Context, timestampKey string) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(zap.String("timestamp_key", timestampKey))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping timestamp update")
		return
	}

	err := model.UpdateTraceTimestamp(c, traceID, timestampKey)
	if err != nil {
		lg.Error("failed to update trace timestamp", zap.Error(err))
	}
}

// RecordTraceExternalCall appends an external call entry to the trace timeline.
func RecordTraceExternalCall(c *gin.Context, call model.TraceExternalCall) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping external call record")
		return
	}
	if err := model.AppendTraceExternalCall(c, traceID, call); err != nil {
		lg.Error("failed to append trace external call", zap.Error(err))
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
	lg := gmw.GetLogger(c).With(zap.Int("status", status))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping status update")
		return
	}

	ctx := gmw.SetLogger(gmw.Ctx(c), lg)
	err := model.UpdateTraceStatus(ctx, traceID, status)
	if err != nil {
		lg.Error("failed to update trace status", zap.Error(err))
	}
}

// RecordTraceEnd marks the completion of a request and records final timestamp
func RecordTraceEnd(c *gin.Context) {
	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c)
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
	_ = gmw.SetLogger(gmw.Ctx(c), lg) // attach logger for symmetry; RecordTraceStatus will fetch its own context
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
	traceID := GetOpenTelemetryTraceID(c)
	if traceID == "" {
		traceID = GetTraceID(c)
	}
	return "chatcmpl-oneapi-" + traceID
}

// GenerateChatCompletionIDFromContext generates a chat completion ID from standard context.
// This is useful when only context.Context is available (not gin.Context).
//
// Format: chatcmpl-oneapi-{trace-id}
//
// Returns: Chat completion ID string with "chatcmpl-oneapi-" prefix
func GenerateChatCompletionIDFromContext(ctx context.Context) string {
	traceID := GetOpenTelemetryTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = GetTraceIDFromContext(ctx)
	}
	return "chatcmpl-oneapi-" + traceID
}
