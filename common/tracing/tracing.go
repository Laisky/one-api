package tracing

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// EventRequestReceived records when the gateway accepts an inbound request.
	EventRequestReceived = "request.received"
	// EventRelayStart records when relay handling begins.
	EventRelayStart = "relay.start"
	// EventUpstreamRequestSent records when the upstream request is dispatched.
	EventUpstreamRequestSent = "upstream.request.sent"
	// EventFirstUpstreamByte records when the first upstream response byte or event is available.
	EventFirstUpstreamByte = "upstream.first_byte"
	// EventUpstreamComplete records when upstream response handling completes.
	EventUpstreamComplete = "upstream.complete"
	// EventResponseComplete records when the client response is complete.
	EventResponseComplete = "response.complete"
	// EventError records a relay or middleware error.
	EventError = "error"
)

const (
	// TimestampRequestForwarded preserves the old timestamp key for upstream dispatch events.
	TimestampRequestForwarded = "request_forwarded"
	// TimestampFirstUpstreamResponse preserves the old timestamp key for first upstream response events.
	TimestampFirstUpstreamResponse = "first_upstream_response"
	// TimestampFirstClientResponse preserves the old timestamp key for first client response events.
	TimestampFirstClientResponse = "first_client_response"
	// TimestampUpstreamCompleted preserves the old timestamp key for upstream completion events.
	TimestampUpstreamCompleted = "upstream_completed"
	// TimestampRequestCompleted preserves the old timestamp key for request completion events.
	TimestampRequestCompleted = "request_completed"
)

const (
	// OpenTelemetryTraceIDKey caches a generated fallback OpenTelemetry trace ID on gin context.
	OpenTelemetryTraceIDKey = "open_telemetry_trace_id"
	// GenAIOperationNameAttr is the standard GenAI operation attribute.
	GenAIOperationNameAttr = "gen_ai.operation.name"
	// GenAIProviderNameAttr is the standard GenAI provider attribute.
	GenAIProviderNameAttr = "gen_ai.provider.name"
	// GenAIRequestModelAttr is the standard requested model attribute.
	GenAIRequestModelAttr = "gen_ai.request.model"
	// GenAIResponseModelAttr is the standard response model attribute.
	GenAIResponseModelAttr = "gen_ai.response.model"
	// GenAIUsageInputTokensAttr is the standard input token usage attribute.
	GenAIUsageInputTokensAttr = "gen_ai.usage.input_tokens"
	// GenAIUsageOutputTokensAttr is the standard output token usage attribute.
	GenAIUsageOutputTokensAttr = "gen_ai.usage.output_tokens"
	// OneAPIChannelIDAttr identifies the selected OneAPI channel.
	OneAPIChannelIDAttr = "oneapi.channel_id"
	// OneAPIStreamAttr records whether the request is streamed.
	OneAPIStreamAttr = "oneapi.stream"
	// OneAPIUpstreamAddressAttr records the selected upstream address.
	OneAPIUpstreamAddressAttr = "oneapi.upstream_address"
	// OneAPIUpstreamURLAttr records the sanitized upstream URL.
	OneAPIUpstreamURLAttr = "oneapi.upstream_url"
)

var excludedGenAIInputDetailAttrs = map[string]struct{}{
	"gen_ai.input.messages":      {},
	"gen_ai.output.messages":     {},
	"gen_ai.system_instructions": {},
	"gen_ai.prompt":              {},
	"gen_ai.prompt_template":     {},
}

// otelSpanContextFromContext extracts a valid OpenTelemetry span context.
func otelSpanContextFromContext(ctx context.Context) oteltrace.SpanContext {
	if ctx == nil {
		return oteltrace.SpanContext{}
	}

	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx
	}

	return oteltrace.SpanContext{}
}

// otelTraceIDFromContext extracts the OpenTelemetry trace ID from a context when available.
func otelTraceIDFromContext(ctx context.Context) string {
	spanCtx := otelSpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

// otelSpanIDFromContext extracts the OpenTelemetry span ID from a context when available.
func otelSpanIDFromContext(ctx context.Context) string {
	spanCtx := otelSpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.SpanID().String()
}

// GetTraceID extracts the OpenTelemetry trace ID from a gin context.
func GetTraceID(c *gin.Context) string {
	return GetOpenTelemetryTraceID(c)
}

// GetTraceIDFromContext extracts the OpenTelemetry trace ID from a context.
func GetTraceIDFromContext(ctx context.Context) string {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		return GetOpenTelemetryTraceID(ginCtx)
	}
	return otelTraceIDFromContext(ctx)
}

// GetOpenTelemetryTraceID extracts the OpenTelemetry trace id from gin context when available.
//
// This is used when callers need a stable distributed trace id (not span-scoped), e.g.
// generating OpenAI-style response IDs.
func GetOpenTelemetryTraceID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if c.Request != nil {
		if traceID := otelTraceIDFromContext(c.Request.Context()); traceID != "" {
			return traceID
		}
	}
	if traceID := otelTraceIDFromContext(gmw.Ctx(c)); traceID != "" {
		return traceID
	}
	if cached := c.GetString(OpenTelemetryTraceIDKey); cached != "" {
		return cached
	}

	traceID, err := GenerateOpenTelemetryTraceID()
	if err != nil {
		return ""
	}
	c.Set(OpenTelemetryTraceIDKey, traceID)
	return traceID
}

// GetOpenTelemetryTraceIDFromContext extracts the OpenTelemetry trace id from a standard context.
//
// Returns empty string when no OpenTelemetry span context is available.
func GetOpenTelemetryTraceIDFromContext(ctx context.Context) string {
	if traceID := otelTraceIDFromContext(ctx); traceID != "" {
		return traceID
	}

	traceID, err := GenerateOpenTelemetryTraceID()
	if err != nil {
		return ""
	}
	return traceID
}

// GenerateOpenTelemetryTraceID creates a valid non-zero OpenTelemetry trace ID string.
func GenerateOpenTelemetryTraceID() (string, error) {
	var traceID oteltrace.TraceID
	if _, err := io.ReadFull(rand.Reader, traceID[:]); err != nil {
		return "", errors.Wrap(err, "read random trace id bytes")
	}
	return traceID.String(), nil
}

// GetOpenTelemetrySpanID extracts the OpenTelemetry span id from gin context when available.
func GetOpenTelemetrySpanID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if c.Request != nil {
		if spanID := otelSpanIDFromContext(c.Request.Context()); spanID != "" {
			return spanID
		}
	}
	return otelSpanIDFromContext(gmw.Ctx(c))
}

// GetOpenTelemetrySpanIDFromContext extracts the OpenTelemetry span id from a standard context.
func GetOpenTelemetrySpanIDFromContext(ctx context.Context) string {
	return otelSpanIDFromContext(ctx)
}

// IsExcludedGenAIInputDetailAttribute reports whether attr would expose prompt or message content.
func IsExcludedGenAIInputDetailAttribute(attr string) bool {
	_, ok := excludedGenAIInputDetailAttrs[attr]
	return ok
}

// GenAIClientSpanName returns the standard GenAI client span name from known values.
func GenAIClientSpanName(operationName, model string) string {
	if operationName == "" {
		return model
	}
	if model == "" {
		return operationName
	}
	return operationName + " " + model
}

// AddSpanEvent records an OpenTelemetry span event when a recording span exists.
func AddSpanEvent(ctx context.Context, eventName string, attrs ...attribute.KeyValue) {
	span := oteltrace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent(eventName, oteltrace.WithAttributes(attrs...))
}

// SetSpanAttributes records attributes on the current OpenTelemetry span when it is recording.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := oteltrace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attrs...)
}

// RecordTraceEvent records a storage-free tracing event without emitting operational logs.
func RecordTraceEvent(c *gin.Context, eventName string, fields ...zap.Field) {
	_ = fields
	RecordTraceEventAttrs(c, eventName)
}

// RecordTraceEventAttrs records a storage-free tracing event with OpenTelemetry attributes.
func RecordTraceEventAttrs(c *gin.Context, eventName string, attrs ...attribute.KeyValue) {
	AddSpanEvent(gmw.Ctx(c), eventName, attrs...)
}

// RecordTraceStart records the request start event without database persistence.
func RecordTraceStart(c *gin.Context) {
	status := http.StatusOK
	if c.Writer != nil && c.Writer.Status() > 0 {
		status = c.Writer.Status()
	}
	method := c.Request.Method
	path := ""
	if c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	RecordTraceEventAttrs(c, EventRequestReceived,
		attribute.String("http.request.method", method),
		attribute.String("url.path", path),
		attribute.Int("http.response.status_code", status),
	)
}

// RecordTraceTimestamp maps legacy timestamp names to storage-free OTel/log events.
func RecordTraceTimestamp(c *gin.Context, timestampKey string) {
	eventName := timestampKey
	switch timestampKey {
	case TimestampRequestForwarded:
		eventName = EventUpstreamRequestSent
	case TimestampFirstUpstreamResponse:
		eventName = EventFirstUpstreamByte
	case TimestampFirstClientResponse:
		eventName = EventRelayStart
	case TimestampUpstreamCompleted:
		eventName = EventUpstreamComplete
	case TimestampRequestCompleted:
		eventName = EventResponseComplete
	}
	RecordTraceEventAttrs(c, eventName, attribute.String("oneapi.timestamp_key", timestampKey))
}

// RecordExternalCall records an external dependency call as an OpenTelemetry event.
func RecordExternalCall(c *gin.Context, attrs ...attribute.KeyValue) {
	RecordTraceEventAttrs(c, "external_call", attrs...)
}

// RecordTraceStatus records the final HTTP status without database persistence.
func RecordTraceStatus(c *gin.Context, status int) {
	RecordTraceEventAttrs(c, EventResponseComplete, attribute.Int("http.response.status_code", status))
}

// RecordTraceEnd records the request completion event without database persistence.
func RecordTraceEnd(c *gin.Context) {
	status := c.Writer.Status()
	if status == 0 {
		status = http.StatusOK
	}
	RecordTraceStatus(c, status)
}

// WithTraceID adds standard OpenTelemetry trace ID fields to structured logging fields.
func WithTraceID(c *gin.Context, fields ...zap.Field) []zap.Field {
	traceID := GetOpenTelemetryTraceID(c)
	if traceID == "" {
		return fields
	}

	traceField := zap.String("trace_id", traceID)
	return append([]zap.Field{traceField}, fields...)
}

// WithTraceIDFromContext adds standard OpenTelemetry trace ID fields from context.
func WithTraceIDFromContext(ctx context.Context, fields ...zap.Field) []zap.Field {
	traceID := GetOpenTelemetryTraceIDFromContext(ctx)
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
	return "chatcmpl-oneapi-" + traceID
}
