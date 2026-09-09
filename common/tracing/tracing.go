package tracing

import (
	"context"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
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
// Resolution order:
//  1. When the context contains an embedded gin.Context (gmw.BackgroundCtx pattern),
//     the span-scoped gin-middlewares TraceID is returned.
//  2. Otherwise it reads the trace id snapshotted under gutils.TracingKey. This is the
//     relayctx.Detach pattern: a c-free background context carries the trace id BY
//     VALUE as a string under gutils.TracingKey (no embedded gin, no OTel span), so it
//     would otherwise be lost here.
//  3. When neither is available, it falls back to the OpenTelemetry trace id.
func GetTraceIDFromContext(ctx context.Context) string {
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		return GetTraceID(ginCtx)
	}
	if v, ok := ctx.Value(gutils.TracingKey).(string); ok && v != "" {
		return v
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

// traceDisabled reports whether trace recording is switched off entirely.
//
// Parameters: none.
//
// Return values:
//   - bool: true when TRACE_SINK resolves to exactly "none", in which case no
//     recorder is created and no legacy synchronous write is issued either.
func traceDisabled() bool {
	return len(config.TraceSinks) == 1 && config.TraceSinks[0] == config.TraceSinkNone
}

// syncWriteMode reports whether the legacy per-mutation synchronous write path
// is selected.
//
// Parameters: none.
//
// Return values:
//   - bool: true when TRACE_WRITE_MODE is "sync".
func syncWriteMode() bool {
	return config.TraceWriteMode == config.TraceWriteModeSync
}

// spanFromGin returns the OpenTelemetry span attached to a request, if any.
//
// Parameters:
//   - c: the gin context; nil or a request-less context yields a non-recording
//     span.
//
// Return values:
//   - oteltrace.Span: the active span, never nil.
func spanFromGin(c *gin.Context) oteltrace.Span {
	if c == nil || c.Request == nil {
		return oteltrace.SpanFromContext(context.Background())
	}
	return oteltrace.SpanFromContext(c.Request.Context())
}

// detachedRequestContext returns a context carrying the request's span and
// logger but not its cancellation, so trace bookkeeping survives a client
// disconnect exactly as the pre-proposal path did.
//
// Parameters:
//   - c: the gin context; nil yields context.Background().
//
// Return values:
//   - context.Context: the detached context.
func detachedRequestContext(c *gin.Context) context.Context {
	if c == nil {
		return context.Background()
	}
	ctx := gmw.Ctx(c)
	if ctx == nil {
		if c.Request != nil {
			ctx = c.Request.Context()
		} else {
			return context.Background()
		}
	}
	return context.WithoutCancel(ctx)
}

// PathExcluded reports whether a request path is on the never-trace list.
//
// Without this every static SPA asset, health probe, and Prometheus scrape
// creates a trace row: TracingMiddleware is registered globally, before any
// route grouping.
//
// Parameters:
//   - path: the request path, without query string.
//
// Return values:
//   - bool: true when the path matches a configured prefix.
func PathExcluded(path string) bool {
	for _, prefix := range config.TraceExcludedPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// requestExcluded reports whether a request must not be traced at all.
//
// Parameters:
//   - c: the gin context; a nil or request-less context is treated as excluded.
//
// Return values:
//   - bool: true when tracing is disabled globally or the path is excluded.
func requestExcluded(c *gin.Context) bool {
	if traceDisabled() {
		return true
	}
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return true
	}
	return PathExcluded(c.Request.URL.Path)
}

// RecordTraceStart begins recording a request's trace.
//
// In the default batched write mode this only allocates an in-memory Recorder
// and annotates the active span; it issues no database statement. In the legacy
// sync mode it creates the row immediately, preserving pre-proposal behavior.
//
// Parameters:
//   - c: the gin context of the request that just arrived.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceStart(c *gin.Context) {
	if requestExcluded(c) {
		noteRequestExcluded(c)
		return
	}

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

	if syncWriteMode() {
		ctx := gmw.SetLogger(gmw.Ctx(c), lg)
		if _, err := model.CreateTrace(ctx, traceID, url, method, bodySize); err != nil {
			lg.Error("failed to create trace record", zap.Error(err))
		}
		return
	}

	// The sync path annotates the span inside model.CreateTrace; the batched
	// path has no model call at request start, so it annotates here. Doing it
	// in exactly one branch keeps the events from being emitted twice.
	if span := spanFromGin(c); span.IsRecording() {
		sanitizedURL, _ := model.SanitizeTraceURL(url)
		span.SetAttributes(
			attribute.String("one_api.trace_id", traceID),
			attribute.String("one_api.url", sanitizedURL),
			attribute.String("one_api.method", method),
			attribute.Int64("one_api.body_size", bodySize),
		)
		span.AddEvent(model.TimestampRequestReceived)
	}

	rec := NewRecorder(traceID, url, method, bodySize)
	if rec == nil {
		// Active-recorder admission was denied (TRACE_MAX_ACTIVE_RECORDERS);
		// NewRecorder already counted the drop. The request proceeds untraced
		// rather than failing, and every later hook resolves to a nil recorder.
		return
	}
	bindRecorder(c, rec)
}

// RecordTraceTimestamp records a lifecycle timestamp for the current request.
//
// Parameters:
//   - c: the gin context of the request.
//   - timestampKey: one of the model.Timestamp* constants.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceTimestamp(c *gin.Context, timestampKey string) {
	if requestExcluded(c) {
		return
	}

	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(zap.String("timestamp_key", timestampKey))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping timestamp update")
		return
	}

	if rec := recorderFromGin(c); rec != nil {
		if span := spanFromGin(c); span.IsRecording() {
			span.AddEvent(timestampKey)
		}
		if !rec.Mark(timestampKey) {
			lg.Warn("unknown timestamp key", zap.String("trace_id", traceID))
		}
		return
	}

	if !syncWriteMode() {
		// Batched mode with no recorder: the request did not pass through
		// TracingMiddleware (synthetic contexts used by channel testing do
		// this). The pre-proposal path issued a SELECT that could only miss.
		return
	}

	if err := model.UpdateTraceTimestamp(c, traceID, timestampKey); err != nil {
		lg.Error("failed to update trace timestamp", zap.Error(err))
	}
}

// RecordTraceExternalCall appends an external call entry to the trace timeline.
//
// Parameters:
//   - c: the gin context of the request.
//   - call: the external call entry to append.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceExternalCall(c *gin.Context, call model.TraceExternalCall) {
	if requestExcluded(c) {
		return
	}

	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping external call record")
		return
	}

	if rec := recorderFromGin(c); rec != nil {
		rec.AppendExternalCall(call)
		return
	}

	if !syncWriteMode() {
		return
	}

	if err := model.AppendTraceExternalCall(c, traceID, call); err != nil {
		lg.Error("failed to append trace external call", zap.Error(err))
	}
}

// RecordTraceStatus records the HTTP status code for the current request.
//
// Parameters:
//   - c: the gin context of the request.
//   - status: the HTTP status code.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceStatus(c *gin.Context, status int) {
	if requestExcluded(c) {
		return
	}

	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c).With(zap.Int("status", status))
	if traceID == "" {
		lg.Warn("empty trace ID, skipping status update")
		return
	}

	if rec := recorderFromGin(c); rec != nil {
		if span := spanFromGin(c); span.IsRecording() {
			span.SetAttributes(attribute.Int("one_api.status", status))
		}
		rec.SetStatus(status)
		return
	}

	if !syncWriteMode() {
		return
	}

	ctx := gmw.SetLogger(gmw.Ctx(c), lg)
	if err := model.UpdateTraceStatus(ctx, traceID, status); err != nil {
		lg.Error("failed to update trace status", zap.Error(err))
	}
}

// ForceTraceSample pins the current request's trace so the sampler always keeps
// it, regardless of TRACE_SAMPLE_RATE.
//
// Parameters:
//   - c: the gin context of the request.
//
// Return values: none.
func ForceTraceSample(c *gin.Context) {
	recorderFromGin(c).ForceSample()
}

// RecordTraceEnd marks the completion of a request, applies the sampling
// decision, and hands the finished trace to the configured sinks.
//
// This is the only point at which the batched path touches a sink, and it is
// where the head-plus-tail sampling decision is made: status and duration are
// both known here, so error and slow-request retention needs no extra buffering.
//
// Parameters:
//   - c: the gin context of the request that just finished.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceEnd(c *gin.Context) {
	status := 0
	if c != nil && c.Writer != nil {
		status = c.Writer.Status()
	}
	recordTraceEnd(c, status)
}

// RecordTraceEndWithStatus completes a request trace with an explicit final
// status when the caller knows the response writer has not yet been updated.
// This is used by middleware that observes a panic before Gin's outer recovery
// handler writes its 500 response.
//
// Parameters:
//   - c: the gin context of the request that just finished.
//   - status: the final HTTP status code to record; values below 1 become 200.
//
// Return values: none; failures are logged because tracing is best-effort.
func RecordTraceEndWithStatus(c *gin.Context, status int) {
	recordTraceEnd(c, status)
}

// recordTraceEnd completes a request trace after its final status has been
// resolved by the public caller.
//
// Parameters:
//   - c: the gin context of the request that just finished.
//   - status: the final HTTP status code to record.
//
// Return values: none; failures are logged because tracing is best-effort.
func recordTraceEnd(c *gin.Context, status int) {
	if requestExcluded(c) {
		noteRequestExcluded(c)
		return
	}

	traceID := GetTraceID(c)
	lg := gmw.GetLogger(c)
	if traceID == "" {
		lg.Warn("empty trace ID, skipping trace end recording")
		return
	}

	if status < 1 {
		status = 200 // Default to 200 if no status was set
	}

	// Record the final timestamp and status code. Both resolve to the in-memory
	// recorder in batched mode and to the legacy UPDATE statements in sync mode.
	RecordTraceTimestamp(c, model.TimestampRequestCompleted)
	RecordTraceStatus(c, status)

	rec := recorderFromGin(c)
	if rec == nil {
		return
	}

	in, durationMs, ok := rec.Finish()
	if !ok {
		// Already finished: never emit a second row for one request.
		return
	}

	// The status recorded above is what the client actually received, which for
	// a failed stream is 200: once headers are flushed nothing can change it.
	// The semantic failure is therefore passed separately, so the
	// always-sample-errors rule still retains the trace. Time-to-first-token is
	// passed alongside the total lifetime so the slow rule is not dominated by
	// long-lived streams.
	ttftMs, ttftKnown := timeToFirstTokenMs(in)
	if !SampleDecisionFor(SampleInput{
		Status:     in.Status,
		DurationMs: durationMs,
		TTFTMs:     ttftMs,
		TTFTKnown:  ttftKnown,
		Forced:     rec.Forced(),
		Failure:    TraceFailure(c),
	}) {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeSampledOut, 1)
		return
	}

	row, truncated, err := model.NewTraceRow(in)
	if err != nil {
		lg.Error("failed to build trace record", zap.Error(err), zap.String("trace_id", traceID))
		return
	}
	if truncated {
		lg.Warn("trace url truncated to max length",
			zap.String("trace_id", traceID),
			zap.Int("original_length", len(in.URL)),
			zap.Int("truncated_length", len(row.URL)))
	}

	if err := Sink().Submit(detachedRequestContext(c), row); err != nil {
		lg.Warn("failed to submit trace record", zap.Error(err), zap.String("trace_id", traceID))
	}
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
