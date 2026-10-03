package telemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// privacySpanExporter removes untrusted text before the OTLP exporter sees it.
// It preserves identity, timing, status codes, and allowlisted operational data.
type privacySpanExporter struct{ sdktrace.SpanExporter }

// ExportSpans exports private views without mutating spans retained by SQL or
// diagnostic consumers. Shutdown is delegated to the embedded exporter.
func (e privacySpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	private := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		private[i] = privateSpan{ReadOnlySpan: span}
	}
	return e.SpanExporter.ExportSpans(ctx, private)
}

// privateSpan overrides every textual span surface, including links and scope.
type privateSpan struct{ sdktrace.ReadOnlySpan }

// Name returns a fixed event class; arbitrary span names may contain payloads.
func (privateSpan) Name() string { return "one-api request" }

// SpanContext keeps correlation IDs and flags without caller-supplied tracestate.
func (s privateSpan) SpanContext() oteltrace.SpanContext {
	return s.ReadOnlySpan.SpanContext().WithTraceState(oteltrace.TraceState{})
}

// Parent keeps the parent ID without caller-supplied tracestate.
func (s privateSpan) Parent() oteltrace.SpanContext {
	return s.ReadOnlySpan.Parent().WithTraceState(oteltrace.TraceState{})
}

// Attributes returns only typed operational fields and enumerated strings.
func (s privateSpan) Attributes() []attribute.KeyValue {
	return privateSpanAttributes(s.ReadOnlySpan.Attributes())
}

// Status preserves the status code while removing arbitrary exception text.
func (s privateSpan) Status() sdktrace.Status {
	return sdktrace.Status{Code: s.ReadOnlySpan.Status().Code}
}

// InstrumentationScope returns a fixed scope without untrusted scope attributes.
func (privateSpan) InstrumentationScope() instrumentation.Scope {
	return instrumentation.Scope{Name: "github.com/Laisky/one-api"}
}

// InstrumentationLibrary returns the same fixed scope for legacy exporters.
func (s privateSpan) InstrumentationLibrary() instrumentation.Library {
	return s.InstrumentationScope()
}

// Resource removes unknown resource attributes from all exported spans.
func (s privateSpan) Resource() *sdkresource.Resource {
	return privateResource(s.ReadOnlySpan.Resource())
}

// Links preserves linked IDs while removing arbitrary link text and tracestate.
func (s privateSpan) Links() []sdktrace.Link {
	links := s.ReadOnlySpan.Links()
	private := make([]sdktrace.Link, len(links))
	for i, link := range links {
		private[i] = sdktrace.Link{SpanContext: link.SpanContext.WithTraceState(oteltrace.TraceState{})}
	}
	return private
}

// Events keeps the known request timeline and numeric external-call diagnostics.
// Tool names, server labels, exception text, and unknown events are omitted.
func (s privateSpan) Events() []sdktrace.Event {
	var events []sdktrace.Event
	for _, event := range s.ReadOnlySpan.Events() {
		switch event.Name {
		case "request_received", "request_forwarded", "first_upstream_response",
			"first_client_response", "upstream_completed", "request_completed", "one_api.external_call":
			events = append(events, sdktrace.Event{Name: event.Name, Time: event.Time,
				Attributes: privateSpanAttributes(event.Attributes)})
		}
	}
	return events
}

// privateSpanAttributes applies a key-and-type allowlist; unknown strings,
// nested values, URLs, request paths, and user agents are never exported.
func privateSpanAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	var kept []attribute.KeyValue
	for _, kv := range attrs {
		key := string(kv.Key)
		switch key {
		case "one_api.body_size", "http.request.body.size", "http.response.body.size", "duration_ms":
			if kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 0 {
				kept = append(kept, kv)
			}
		case "one_api.status", "http.response.status_code", "http.status_code":
			if kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 100 && kv.Value.AsInt64() <= 599 {
				kept = append(kept, kv)
			}
		case "server_id":
			if kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 0 && kv.Value.AsInt64() <= 1<<31-1 {
				kept = append(kept, kv)
			}
		case "is_error":
			if kv.Value.Type() == attribute.BOOL {
				kept = append(kept, kv)
			}
		case "one_api.method", "http.request.method", "http.method":
			if kv.Value.Type() == attribute.STRING && isPrivateMethod(kv.Value.AsString()) {
				kept = append(kept, kv)
			}
		case "one_api.failure":
			if kv.Value.Type() == attribute.STRING {
				switch kv.Value.AsString() {
				case "none", "panic", "timeout", "client_canceled", "upstream":
					kept = append(kept, kv)
				}
			}
		case "source":
			if kv.Value.Type() == attribute.STRING && (kv.Value.AsString() == "mcp" || kv.Value.AsString() == "external") {
				kept = append(kept, kv)
			}
		}
	}
	return kept
}

// isPrivateMethod accepts only known HTTP methods, never the arbitrary wire verb.
func isPrivateMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		return true
	}
	return false
}

// privateResource keeps configured service/host correlation and SDK metadata.
// Configured identities are trusted deployment inputs, not request fields.
// Arbitrary OTEL_RESOURCE_ATTRIBUTES keys cannot expand the export surface.
func privateResource(res *sdkresource.Resource) *sdkresource.Resource {
	if res == nil {
		return sdkresource.Empty()
	}
	var kept []attribute.KeyValue
	for _, kv := range res.Attributes() {
		switch string(kv.Key) {
		case "service.name", "service.version", "host.name", "deployment.environment",
			"deployment.environment.name", "telemetry.sdk.name", "telemetry.sdk.language",
			"telemetry.sdk.version", "process.runtime.name", "process.runtime.version":
			if kv.Value.Type() == attribute.STRING && len(kv.Value.AsString()) <= 128 &&
				!strings.ContainsAny(kv.Value.AsString(), "\r\n") {
				kept = append(kept, kv)
			}
		case "process.pid":
			if kv.Value.Type() == attribute.INT64 {
				kept = append(kept, kv)
			}
		}
	}
	return sdkresource.NewWithAttributes("", kept...)
}
