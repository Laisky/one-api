package telemetry

import (
	"context"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Laisky/one-api/common/metrics"
)

// utf8AttributeSanitizer is a trace SpanProcessor that rewrites any string (or
// string slice) span attribute that is not valid UTF-8 as soon as the span
// starts.
//
// Why: otelgin copies request-derived values (url.path, user_agent.original,
// server.address, ...) onto the server span at start time, and URL.Path is
// percent-decoded, so GET /%c0 yields the attribute value "/\xc0". The OTLP
// exporter marshals spans with protobuf, which rejects invalid UTF-8 in any
// string field, and the batch span processor then drops the entire batch (up
// to hundreds of unrelated spans) for that one request. Sanitizing at OnStart
// covers every start-time attribute, which is where request-controlled bytes
// enter; ReadOnlySpan is sealed by the SDK so spans cannot be rewritten at
// export time.
type utf8AttributeSanitizer struct{}

var _ sdktrace.SpanProcessor = utf8AttributeSanitizer{}

// newUTF8AttributeSanitizer returns the span processor to register ahead of
// the exporting batcher.
func newUTF8AttributeSanitizer() sdktrace.SpanProcessor {
	return utf8AttributeSanitizer{}
}

// OnStart rewrites invalid UTF-8 string attributes in place on the span.
func (utf8AttributeSanitizer) OnStart(_ context.Context, s sdktrace.ReadWriteSpan) {
	fixed := sanitizeAttributes(s.Attributes())
	if len(fixed) > 0 {
		s.SetAttributes(fixed...)
	}
}

// OnEnd is a no-op: ended spans are read-only.
func (utf8AttributeSanitizer) OnEnd(sdktrace.ReadOnlySpan) {}

// Shutdown is a no-op: the processor holds no resources.
func (utf8AttributeSanitizer) Shutdown(context.Context) error { return nil }

// ForceFlush is a no-op: the processor buffers nothing.
func (utf8AttributeSanitizer) ForceFlush(context.Context) error { return nil }

// sanitizeAttributes returns replacement attributes for every entry of attrs
// whose string value is not valid UTF-8. It returns nil when nothing needs to
// change so the common case allocates nothing.
func sanitizeAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	var fixed []attribute.KeyValue
	for _, kv := range attrs {
		switch kv.Value.Type() {
		case attribute.STRING:
			if v := kv.Value.AsString(); !utf8.ValidString(v) {
				fixed = append(fixed, attribute.String(string(kv.Key), metrics.SanitizeLabelValue(v)))
			}
		case attribute.STRINGSLICE:
			values := kv.Value.AsStringSlice()
			changed := false
			for i, v := range values {
				if !utf8.ValidString(v) {
					values[i] = metrics.SanitizeLabelValue(v)
					changed = true
				}
			}
			if changed {
				fixed = append(fixed, attribute.StringSlice(string(kv.Key), values))
			}
		default:
			// Non-string attribute values cannot carry invalid UTF-8.
		}
	}
	return fixed
}
