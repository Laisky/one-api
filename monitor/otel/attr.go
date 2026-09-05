package otel

import (
	"go.opentelemetry.io/otel/attribute"

	"github.com/Laisky/one-api/common/metrics"
)

// strAttr builds a string attribute whose value is guaranteed to be valid
// UTF-8 (see metrics.SanitizeLabelValue).
//
// Why: the OTLP exporter marshals attributes with protobuf, which rejects any
// string field that is not valid UTF-8 and fails the whole export batch. With
// cumulative metric aggregators the poisoned attribute set is retained, so a
// single request path such as "/\xc0" would make every later metric export
// fail until restart. Every string attribute in this package must go through
// strAttr.
func strAttr(key, value string) attribute.KeyValue {
	return attribute.String(key, metrics.SanitizeLabelValue(value))
}
