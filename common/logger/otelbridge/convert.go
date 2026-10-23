package otelbridge

// Native scalar conversions for the forked-Zap OTLP bridge. Complex values
// retain real/imaginary parts and unsigned overflow retains exact decimal text.
// JSON reflection lives in reflected.go and never uses fmt fallback strings.

import (
	"math"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// complexValue renders a complex number as a two-key map of its real and
// imaginary parts, which is the only lossless attribute representation
// available.
//
// Parameters:
//   - v: the complex value to render.
//
// Return values:
//   - attribute.Value: a map value with keys "r" and "i".
func complexValue(v complex128) attribute.Value {
	return attribute.MapValue(
		attribute.Float64("r", real(v)),
		attribute.Float64("i", imag(v)),
	)
}

// convertUintValue converts an unsigned integer into an attribute value,
// falling back to a decimal string when the value cannot be represented as a
// signed 64-bit integer.
//
// Parameters:
//   - v: the unsigned value to convert.
//
// Return values:
//   - attribute.Value: an int64 value when representable, otherwise a string.
func convertUintValue(v uint64) attribute.Value {
	if v > math.MaxInt64 {
		return attribute.StringValue(strconv.FormatUint(v, 10))
	}
	return attribute.Int64Value(int64(v))
}

// convertTimeValue preserves signed Unix nanoseconds when exactly representable.
// For earlier/later valid application time fields, OTLP has no native time
// scalar, so use RFC3339Nano text instead of rejecting unrelated log fields.
func convertTimeValue(v time.Time) attribute.Value {
	if ns, err := logTimeNanos(v); err == nil {
		return attribute.Int64Value(ns)
	}
	// RFC3339 supports only minute-resolution zone offsets. Preserve the
	// instant for historical zones with second-resolution offsets instead of
	// silently shifting it when the text is parsed by a downstream sink.
	if _, offset := v.Zone(); offset%60 != 0 {
		v = v.UTC()
	}
	return attribute.StringValue(v.Format(time.RFC3339Nano))
}
