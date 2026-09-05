package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// sdktraceStartAttributes wraps attributes as a span start option, mirroring
// how otelgin attaches request attributes when it starts the server span.
func sdktraceStartAttributes(attrs ...attribute.KeyValue) trace.SpanStartOption {
	return trace.WithAttributes(attrs...)
}
