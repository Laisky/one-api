package otelbridge

import (
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/attribute"
)

// ErrLogEncoding is a fixed diagnostic: it never embeds application field values.
var ErrLogEncoding = errors.New("application log cannot be encoded faithfully")

// maxLogEncodingDepth is a stack/work resource bound, not an OTLP protocol rule.
// Exceeding it rejects the whole record visibly instead of replacing a subtree.
const maxLogEncodingDepth = 256

// ValidateLogValue checks values without rewriting text, scalars, EMPTY, maps or
// lists. Native OTLP doubles include NaN/Inf; JSON reflection has its own contract.
func ValidateLogValue(value attribute.Value) error { return validateLogValue(value, 0) }

// ValidateLogAttributes rejects duplicate/invalid keys before SDK deduplication.
// Resource/scope attribute.Set values have already chosen canonical map semantics.
func ValidateLogAttributes(attrs []attribute.KeyValue) error {
	return validateLogAttributes(attrs, 0)
}

// validateLogAttributes checks protocol map keys and all nested values.
func validateLogAttributes(attrs []attribute.KeyValue, depth int) error {
	if depth > maxLogEncodingDepth {
		return ErrLogEncoding
	}
	seen := make(map[attribute.Key]struct{}, len(attrs))
	for _, kv := range attrs {
		if !utf8.ValidString(string(kv.Key)) {
			return ErrLogEncoding
		}
		if _, exists := seen[kv.Key]; exists {
			return ErrLogEncoding
		}
		seen[kv.Key] = struct{}{}
		if err := validateLogValue(kv.Value, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateLogValue walks native AnyValue-compatible values with bounded depth.
func validateLogValue(v attribute.Value, depth int) error {
	if depth > maxLogEncodingDepth {
		return ErrLogEncoding
	}
	switch v.Type() {
	case attribute.EMPTY, attribute.BOOL, attribute.INT64, attribute.FLOAT64, attribute.BOOLSLICE, attribute.INT64SLICE, attribute.FLOAT64SLICE, attribute.BYTESLICE:
		return nil
	case attribute.STRING:
		if !utf8.ValidString(v.AsString()) {
			return ErrLogEncoding
		}
	case attribute.STRINGSLICE:
		for _, s := range v.AsStringSlice() {
			if !utf8.ValidString(s) {
				return ErrLogEncoding
			}
		}
	case attribute.SLICE:
		for _, item := range v.AsSlice() {
			if err := validateLogValue(item, depth+1); err != nil {
				return err
			}
		}
	case attribute.MAP:
		return validateLogAttributes(v.AsMap(), depth+1)
	default:
		return ErrLogEncoding
	}
	return nil
}

// appendNamespaced appends fields at the persistent Zap Namespace cursor. It
// preserves already-materialized With values without invoking their encoders again.
func appendNamespaced(base []attribute.KeyValue, path []string, extra []attribute.KeyValue) []attribute.KeyValue {
	out := append([]attribute.KeyValue(nil), base...)
	if len(path) == 0 {
		return append(out, extra...)
	}
	for i := len(out) - 1; i >= 0; i-- {
		if string(out[i].Key) == path[0] && out[i].Value.Type() == attribute.MAP {
			out[i].Value = attribute.MapValue(appendNamespaced(out[i].Value.AsMap(), path[1:], extra)...)
			return out
		}
	}
	return append(out, attribute.Map(path[0], appendNamespaced(nil, path[1:], extra)...))
}

// hasLogKey reports whether generated metadata would collide with an app key.
func hasLogKey(attrs []attribute.KeyValue, key attribute.Key) bool {
	for _, kv := range attrs {
		if kv.Key == key {
			return true
		}
	}
	return false
}

// ValidateLogRecordAttributes additionally checks the pinned SDK's nonempty-key
// contract. Empty JSON/body map keys remain valid; this is not a blanket map rule.
func ValidateLogRecordAttributes(attrs []attribute.KeyValue) error {
	for _, kv := range attrs {
		if len(kv.Key) == 0 {
			return ErrLogEncoding
		}
	}
	return ValidateLogAttributes(attrs)
}
