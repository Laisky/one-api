package otelbridge

// Value conversion for the OTLP application-log bridge.
//
// Derived from go.opentelemetry.io/contrib/bridges/otelzap (Copyright The
// OpenTelemetry Authors, SPDX-License-Identifier: Apache-2.0). That package
// cannot be imported here: it is written against go.uber.org/zap, while this
// repository uses the github.com/Laisky/zap fork, whose zapcore.Core carries an
// extra Fields() method and whose Field type is a distinct Go type. The
// conversion semantics are kept identical on purpose, so a record produced here
// is indistinguishable from one produced by the upstream bridge and any
// collector pipeline or dashboard written against it keeps working.

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// maxReflectDepth bounds how deep convertValue will follow nested slices, maps,
// pointers and interfaces.
//
// The upstream conversion recurses without a depth limit, which is safe for the
// values a normal log call carries but is not a property this repository can
// rely on: a relay adaptor can log a decoded upstream payload, and a self-
// referential structure reached through an interface would recurse until the
// goroutine stack is exhausted. Logging must never be able to kill the process
// it is describing, so depth is bounded and the overflow is reported in-band as
// a string rather than by panicking.
const maxReflectDepth = 8

// convertValue converts an arbitrary Go value into an OpenTelemetry attribute
// value, handling the common types without reflection.
//
// Parameters:
//   - v: the value to convert; nil and unsupported types are handled.
//
// Return values:
//   - attribute.Value: the converted value; an empty value for nil, or a
//     descriptive string for a type with no attribute representation.
func convertValue(v any) attribute.Value {
	return convertValueDepth(v, 0)
}

// convertValueDepth converts a value while tracking recursion depth.
//
// Parameters:
//   - v: the value to convert.
//   - depth: how many levels of nesting have already been followed.
//
// Return values:
//   - attribute.Value: the converted value, or a truncation marker once depth
//     reaches maxReflectDepth.
func convertValueDepth(v any, depth int) attribute.Value {
	// Handling the most common types without reflect is a small performance win
	// on a path that runs for every field of every exported log line.
	switch val := v.(type) {
	case bool:
		return attribute.BoolValue(val)
	case string:
		return attribute.StringValue(val)
	case int:
		return attribute.Int64Value(int64(val))
	case int8:
		return attribute.Int64Value(int64(val))
	case int16:
		return attribute.Int64Value(int64(val))
	case int32:
		return attribute.Int64Value(int64(val))
	case int64:
		return attribute.Int64Value(val)
	case uint:
		return convertUintValue(uint64(val))
	case uint8:
		return attribute.Int64Value(int64(val))
	case uint16:
		return attribute.Int64Value(int64(val))
	case uint32:
		return attribute.Int64Value(int64(val))
	case uint64:
		return convertUintValue(val)
	case uintptr:
		return convertUintValue(uint64(val))
	case float32:
		return attribute.Float64Value(float64(val))
	case float64:
		return attribute.Float64Value(val)
	case time.Duration:
		return attribute.Int64Value(val.Nanoseconds())
	case complex64:
		return complexValue(complex128(val))
	case complex128:
		return complexValue(val)
	case time.Time:
		return attribute.Int64Value(val.UnixNano())
	case []byte:
		return attribute.ByteSliceValue(val)
	case error:
		return attribute.StringValue(val.Error())
	case attribute.Value:
		return val
	}

	if depth >= maxReflectDepth {
		return attribute.StringValue("truncated: value nested deeper than " +
			strconv.Itoa(maxReflectDepth) + " levels")
	}

	t := reflect.TypeOf(v)
	if t == nil {
		return attribute.Value{}
	}
	val := reflect.ValueOf(v)
	switch t.Kind() {
	case reflect.Struct:
		return attribute.StringValue(fmt.Sprintf("%+v", v))
	case reflect.Slice, reflect.Array:
		items := make([]attribute.Value, 0, val.Len())
		for i := range val.Len() {
			items = append(items, convertValueDepth(val.Index(i).Interface(), depth+1))
		}
		return attribute.SliceValue(items...)
	case reflect.Map:
		kvs := make([]attribute.KeyValue, 0, val.Len())
		for _, k := range val.MapKeys() {
			var key string
			switch k.Kind() {
			case reflect.String:
				key = k.String()
			default:
				key = fmt.Sprintf("%+v", k.Interface())
			}
			kvs = append(kvs, attribute.KeyValue{
				Key:   attribute.Key(key),
				Value: convertValueDepth(val.MapIndex(k).Interface(), depth+1),
			})
		}
		return attribute.MapValue(kvs...)
	case reflect.Pointer, reflect.Interface:
		if val.IsNil() {
			return attribute.Value{}
		}
		return convertValueDepth(val.Elem().Interface(), depth+1)
	default:
		// Never panic on an unexpected type. An attribute carrying an "unhandled:"
		// prefix is a question an operator can ask; a panic inside the logger is a
		// crash in the component whose job is to explain crashes.
		return attribute.StringValue(fmt.Sprintf("unhandled: (%s) %+v", t, v))
	}
}

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
