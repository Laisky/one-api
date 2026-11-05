package otelbridge

import (
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/attribute"
)

// ErrLogEncoding is the fixed fail-closed diagnostic for rejected log records.
// The bridge returns this marker unchanged at its rejection boundary, without
// embedding application field values or forwarding serializer errors. Its stack
// identifies marker initialization, not the particular rejected field.
var ErrLogEncoding = errors.New("application log cannot be encoded faithfully")

// maxLogEncodingDepth is a stack/work resource bound, not an OTLP protocol rule.
// Exceeding it rejects the whole record visibly instead of replacing a subtree.
const maxLogEncodingDepth = 256

// ValidateLogValue checks values without rewriting text, scalars, EMPTY, maps or
// lists. Native OTLP doubles include NaN/Inf; JSON reflection has its own contract.
func ValidateLogValue(value attribute.Value) error { return validateLogValue(value, 0) }

// ValidateLogAttributes is the strict check: a repeated key is an error. It is
// for collections that must already hold each key once -- resource and scope
// attribute.Set values, which have canonical map semantics, and records after
// NormalizeLogRecordAttributes has resolved collisions.
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

// NormalizeLogRecordAttributes validates record attributes exactly as the
// strict ValidateLogAttributes does, plus the pinned SDK's rule that a
// top-level key is nonempty (empty keys inside maps remain valid), except that
// a repeated key is resolved instead of rejected.
//
// Zap permits the same key more than once, and the process and request loggers
// both bind "host", so collisions are routine rather than malformed input. OTLP
// requires every attribute collection and nested map to hold a key once, and
// the OpenTelemetry common specification resolves a key set twice by letting
// the later value overwrite the earlier one. This applies that rule at every
// depth: the first occurrence keeps its position and takes the last value,
// which is exactly what the SDK's own deduplication produces
// (TestNormalizeMatchesSDKDeduplication pins the equivalence). It is done here
// rather than by the SDK because the SDK deduplicates one attribute at a time,
// rebuilding a key index for each, which measurably slows every record.
//
// Parameters:
//   - attrs: the record attributes in the order they were added.
//
// Return values:
//   - []attribute.KeyValue: attrs itself when no key repeats at any depth;
//     otherwise a new slice holding each key once.
//   - bool: whether any key repeated, at any depth.
//   - error: ErrLogEncoding when a key or value cannot be encoded faithfully.
func NormalizeLogRecordAttributes(attrs []attribute.KeyValue) ([]attribute.KeyValue, bool, error) {
	for _, kv := range attrs {
		if len(kv.Key) == 0 {
			return nil, false, ErrLogEncoding
		}
	}
	var n keyNormalizer
	out, _, err := n.attributes(attrs, 0)
	if err != nil {
		return nil, false, err
	}
	return out, n.repeated, nil
}

// NormalizeLogValue validates a value exactly as ValidateLogValue does, except
// that a key repeated inside a map is resolved with the same last-value-wins
// rule as NormalizeLogRecordAttributes. It is meant for a native record body.
//
// Parameters:
//   - value: the value to normalize.
//
// Return values:
//   - attribute.Value: value itself when no map key repeats; otherwise the
//     normalized copy.
//   - bool: whether any map key repeated.
//   - error: ErrLogEncoding when the value cannot be encoded faithfully.
func NormalizeLogValue(value attribute.Value) (attribute.Value, bool, error) {
	var n keyNormalizer
	out, _, err := n.value(value, 0)
	if err != nil {
		return attribute.Value{}, false, err
	}
	return out, n.repeated, nil
}

// keyNormalizer walks a value tree with the strict validator's depth budget,
// resolving repeated map keys and remembering whether it had to.
type keyNormalizer struct {
	repeated bool
}

// attributes normalizes one attribute collection or map.
//
// Parameters:
//   - attrs: the collection.
//   - depth: the current nesting depth, counted as validateLogAttributes does.
//
// Return values:
//   - []attribute.KeyValue: attrs itself when nothing changed; otherwise a copy.
//   - bool: whether anything changed.
//   - error: ErrLogEncoding for an invalid key, value, or excessive depth.
func (n *keyNormalizer) attributes(attrs []attribute.KeyValue, depth int) ([]attribute.KeyValue, bool, error) {
	if depth > maxLogEncodingDepth {
		return nil, false, ErrLogEncoding
	}
	// out stays nil, and attrs is returned unchanged, until a nested value
	// changes or a key repeats. seen maps a key to its position in the result,
	// which is the same position in attrs for as long as out is nil.
	var out []attribute.KeyValue
	seen := make(map[attribute.Key]int, len(attrs))
	for i, kv := range attrs {
		if !utf8.ValidString(string(kv.Key)) {
			return nil, false, ErrLogEncoding
		}
		value, changed, err := n.value(kv.Value, depth+1)
		if err != nil {
			return nil, false, err
		}
		at, repeated := seen[kv.Key]
		if (changed || repeated) && out == nil {
			out = append(make([]attribute.KeyValue, 0, len(attrs)), attrs[:i]...)
		}
		kv.Value = value
		switch {
		case repeated:
			n.repeated = true
			out[at] = kv
		case out != nil:
			seen[kv.Key] = len(out)
			out = append(out, kv)
		default:
			seen[kv.Key] = i
		}
	}
	if out == nil {
		return attrs, false, nil
	}
	return out, true, nil
}

// value normalizes v, recursing into lists and maps.
//
// Parameters:
//   - v: the value.
//   - depth: the current nesting depth, counted as validateLogValue does.
//
// Return values:
//   - attribute.Value: v itself when nothing changed; otherwise a copy.
//   - bool: whether the value changed.
//   - error: ErrLogEncoding when v cannot be encoded faithfully.
func (n *keyNormalizer) value(v attribute.Value, depth int) (attribute.Value, bool, error) {
	if depth > maxLogEncodingDepth {
		return v, false, ErrLogEncoding
	}
	switch v.Type() {
	case attribute.SLICE:
		items := v.AsSlice()
		var out []attribute.Value
		for i, item := range items {
			normalized, changed, err := n.value(item, depth+1)
			if err != nil {
				return v, false, err
			}
			if changed && out == nil {
				out = append(make([]attribute.Value, 0, len(items)), items[:i]...)
			}
			if out != nil {
				out = append(out, normalized)
			}
		}
		if out == nil {
			return v, false, nil
		}
		return attribute.SliceValue(out...), true, nil
	case attribute.MAP:
		normalized, changed, err := n.attributes(v.AsMap(), depth+1)
		if err != nil || !changed {
			return v, false, err
		}
		return attribute.MapValue(normalized...), true, nil
	default:
		return v, false, validateLogValue(v, depth)
	}
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

// logTimeNanos checks the established signed-nanosecond attribute projection.
// A zero Go time is a real year-one attribute, not an unset value here.
func logTimeNanos(value time.Time) (int64, error) {
	ns := value.UnixNano()
	if !time.Unix(0, ns).Equal(value) {
		return 0, ErrLogEncoding
	}
	return ns, nil
}

// ValidateLogTimestamp checks the pinned exporter's signed UnixNano projection
// before its uint64 conversion. Zero means unset; negative timestamps cannot be
// represented by that export path. This is narrower than OTLP's uint64 range.
func ValidateLogTimestamp(value time.Time) error {
	if value.IsZero() {
		return nil
	}
	ns, err := logTimeNanos(value)
	if err != nil || ns < 0 {
		return ErrLogEncoding
	}
	return nil
}
