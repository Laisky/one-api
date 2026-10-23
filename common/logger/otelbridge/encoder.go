package otelbridge

// zapcore field encoders for the OTLP application-log bridge.
//
// Derived from go.opentelemetry.io/contrib/bridges/otelzap (Copyright The
// OpenTelemetry Authors, SPDX-License-Identifier: Apache-2.0), ported to the
// github.com/Laisky/zap fork. See convert.go for why the upstream package
// cannot be imported.
//
// These encoders turn a zap field into OpenTelemetry attributes. zap.Namespace
// opens a nested attribute map rather than a dotted key prefix, and
// zap.Object/zap.Array become nested map/slice attribute values. Encoding
// failures reject the whole OTLP record rather than silently dropping content.

import (
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
)

var (
	_ zapcore.ObjectEncoder = (*objectEncoder)(nil)
	_ zapcore.ArrayEncoder  = (*arrayEncoder)(nil)
)

// namespace is one level of a zap.Namespace chain, holding the attributes added
// while that namespace was open.
type namespace struct {
	name  string
	attrs []attribute.KeyValue
	next  *namespace
}

// objectEncoder implements zapcore.ObjectEncoder by accumulating OpenTelemetry
// attributes, honoring nested namespaces opened with zap.Namespace.
type objectEncoder struct {
	// root points at the outermost namespace, whose attrs are the result.
	root *namespace
	// cur points at the namespace currently being written to.
	cur            *namespace
	failure        *error
	depth          int
	namespaceDepth int
}

// newObjectEncoder returns an object encoder sized for n fields.
//
// Parameters:
//   - n: the expected number of attributes, used only to size the backing slice.
//
// Return values:
//   - *objectEncoder: an encoder whose root namespace is empty.
func newObjectEncoder(n int) *objectEncoder {
	root := &namespace{attrs: make([]attribute.KeyValue, 0, n)}
	var failure error
	return &objectEncoder{root: root, cur: root, failure: &failure}
}

// calculate collapses the namespace chain into nested map attributes.
//
// It must run before the root namespace's attributes are read, because a
// namespace's attributes are only attached to its parent here.
//
// Parameters:
//   - o: the namespace to collapse, normally the encoder's root.
//
// Return values: none.
func (m *objectEncoder) calculate(o *namespace) {
	var chain []*namespace
	for node := o; node != nil; node = node.next {
		chain = append(chain, node)
	}
	for i := len(chain) - 2; i >= 0; i-- {
		chain[i].attrs = append(chain[i].attrs, attribute.Map(chain[i+1].name, chain[i+1].attrs...))
	}
}

// AddArray implements zapcore.ObjectEncoder by encoding v as a slice attribute.
//
// Parameters:
//   - key: the attribute key.
//   - v: the array marshaler supplied at the log site.
//
// Return values:
//   - error: whatever v.MarshalLogArray reported.
func (m *objectEncoder) AddArray(key string, v zapcore.ArrayMarshaler) error {
	arr := newArrayEncoder()
	arr.failure = m.failure
	arr.depth = m.depth + 1
	if arr.depth > maxLogEncodingDepth {
		*m.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	err := v.MarshalLogArray(arr)
	if err != nil {
		*m.failure = ErrLogEncoding
	}
	m.cur.attrs = append(m.cur.attrs, attribute.Slice(key, arr.elems...))
	return errors.WithStack(err)
}

// AddObject implements zapcore.ObjectEncoder by encoding v as a map attribute.
//
// Parameters:
//   - k: the attribute key.
//   - v: the object marshaler supplied at the log site.
//
// Return values:
//   - error: whatever v.MarshalLogObject reported.
func (m *objectEncoder) AddObject(k string, v zapcore.ObjectMarshaler) error {
	// Capacity 2 mirrors zap's own console encoder, which assumes small objects.
	inner := newObjectEncoder(2)
	inner.failure = m.failure
	inner.depth = m.depth + 1
	if inner.depth > maxLogEncodingDepth {
		*m.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	err := v.MarshalLogObject(inner)
	if err != nil {
		*m.failure = ErrLogEncoding
	}
	inner.calculate(inner.root)
	m.cur.attrs = append(m.cur.attrs, attribute.Map(k, inner.root.attrs...))
	return errors.WithStack(err)
}

// AddBinary implements zapcore.ObjectEncoder for raw byte fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the bytes to record.
//
// Return values: none.
func (m *objectEncoder) AddBinary(k string, v []byte) {
	m.cur.attrs = append(m.cur.attrs, attribute.ByteSlice(k, v))
}

// AddByteString implements zapcore.ObjectEncoder for UTF-8 byte fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the bytes to record as a string.
//
// Return values: none.
func (m *objectEncoder) AddByteString(k string, v []byte) {
	m.cur.attrs = append(m.cur.attrs, attribute.String(k, string(v)))
}

// AddBool implements zapcore.ObjectEncoder for boolean fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddBool(k string, v bool) {
	m.cur.attrs = append(m.cur.attrs, attribute.Bool(k, v))
}

// AddDuration implements zapcore.ObjectEncoder, recording durations as
// nanoseconds so no unit information is lost in transport.
//
// Parameters:
//   - k: the attribute key.
//   - v: the duration to record.
//
// Return values: none.
func (m *objectEncoder) AddDuration(k string, v time.Duration) {
	m.AddInt64(k, v.Nanoseconds())
}

// AddComplex128 implements zapcore.ObjectEncoder, recording the value as a map
// of its real and imaginary parts.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddComplex128(k string, v complex128) {
	m.cur.attrs = append(m.cur.attrs, attribute.KeyValue{
		Key:   attribute.Key(k),
		Value: complexValue(v),
	})
}

// AddFloat64 implements zapcore.ObjectEncoder for 64-bit float fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddFloat64(k string, v float64) {
	m.cur.attrs = append(m.cur.attrs, attribute.Float64(k, v))
}

// AddInt64 implements zapcore.ObjectEncoder for 64-bit integer fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddInt64(k string, v int64) {
	m.cur.attrs = append(m.cur.attrs, attribute.Int64(k, v))
}

// AddInt implements zapcore.ObjectEncoder for platform-sized integer fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddInt(k string, v int) {
	m.cur.attrs = append(m.cur.attrs, attribute.Int(k, v))
}

// AddString implements zapcore.ObjectEncoder for string fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddString(k, v string) {
	m.cur.attrs = append(m.cur.attrs, attribute.String(k, v))
}

// AddUint64 implements zapcore.ObjectEncoder for unsigned 64-bit fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUint64(k string, v uint64) {
	m.cur.attrs = append(m.cur.attrs, attribute.KeyValue{
		Key:   attribute.Key(k),
		Value: convertUintValue(v),
	})
}

// AddReflected implements zapcore.ObjectEncoder for values with no dedicated
// zap type, converting them with the shared reflection path.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to convert.
//
// Return values:
//   - error: fixed failure when the declared JSON value cannot be encoded.
func (m *objectEncoder) AddReflected(k string, v any) error {
	value, err := reflectedLogValue(v)
	if err != nil {
		*m.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	m.cur.attrs = append(m.cur.attrs, attribute.KeyValue{Key: attribute.Key(k), Value: value})
	return nil
}

// OpenNamespace implements zapcore.ObjectEncoder by opening a nested attribute
// map that every subsequent field is added to.
//
// Parameters:
//   - k: the namespace name, which becomes the nested map's key.
//
// Return values: none.
func (m *objectEncoder) OpenNamespace(k string) {
	// Fail at the boundary, not after allocating an arbitrarily long
	// namespace chain and invoking later application field serializers.
	if m.depth+m.namespaceDepth >= maxLogEncodingDepth {
		*m.failure = ErrLogEncoding
		return
	}
	m.namespaceDepth++
	next := &namespace{name: k, attrs: make([]attribute.KeyValue, 0, 5)}
	m.cur.next = next
	m.cur = next
}

// AddComplex64 implements zapcore.ObjectEncoder for 64-bit complex fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddComplex64(k string, v complex64) {
	m.AddComplex128(k, complex128(v))
}

// AddTime implements zapcore.ObjectEncoder, preserving exact signed Unix
// nanoseconds when representable and RFC3339Nano text otherwise.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddTime(k string, v time.Time) {
	m.cur.attrs = append(m.cur.attrs, attribute.KeyValue{Key: attribute.Key(k), Value: convertTimeValue(v)})
}

// AddFloat32 implements zapcore.ObjectEncoder for 32-bit float fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddFloat32(k string, v float32) { m.AddFloat64(k, float64(v)) }

// AddInt32 implements zapcore.ObjectEncoder for 32-bit integer fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddInt32(k string, v int32) { m.AddInt64(k, int64(v)) }

// AddInt16 implements zapcore.ObjectEncoder for 16-bit integer fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddInt16(k string, v int16) { m.AddInt64(k, int64(v)) }

// AddInt8 implements zapcore.ObjectEncoder for 8-bit integer fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddInt8(k string, v int8) { m.AddInt64(k, int64(v)) }

// AddUint implements zapcore.ObjectEncoder for platform-sized unsigned fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUint(k string, v uint) { m.AddUint64(k, uint64(v)) }

// AddUint32 implements zapcore.ObjectEncoder for 32-bit unsigned fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUint32(k string, v uint32) { m.AddInt64(k, int64(v)) }

// AddUint16 implements zapcore.ObjectEncoder for 16-bit unsigned fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUint16(k string, v uint16) { m.AddInt64(k, int64(v)) }

// AddUint8 implements zapcore.ObjectEncoder for 8-bit unsigned fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUint8(k string, v uint8) { m.AddInt64(k, int64(v)) }

// AddUintptr implements zapcore.ObjectEncoder for pointer-sized unsigned fields.
//
// Parameters:
//   - k: the attribute key.
//   - v: the value to record.
//
// Return values: none.
func (m *objectEncoder) AddUintptr(k string, v uintptr) { m.AddUint64(k, uint64(v)) }

// arrayEncoder implements zapcore.ArrayEncoder by accumulating attribute values.
type arrayEncoder struct {
	elems   []attribute.Value
	failure *error
	depth   int
}

// newArrayEncoder returns an empty array encoder.
//
// Parameters: none.
//
// Return values:
//   - *arrayEncoder: an encoder holding no elements.
func newArrayEncoder() *arrayEncoder {
	// Capacity 2 mirrors zap's own console encoder.
	var failure error
	return &arrayEncoder{elems: make([]attribute.Value, 0, 2), failure: &failure}
}

// AppendArray implements zapcore.ArrayEncoder for nested arrays.
//
// Parameters:
//   - v: the array marshaler to encode.
//
// Return values:
//   - error: whatever v.MarshalLogArray reported.
func (a *arrayEncoder) AppendArray(v zapcore.ArrayMarshaler) error {
	inner := newArrayEncoder()
	inner.failure = a.failure
	inner.depth = a.depth + 1
	if inner.depth > maxLogEncodingDepth {
		*a.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	err := v.MarshalLogArray(inner)
	if err != nil {
		*a.failure = ErrLogEncoding
	}
	a.elems = append(a.elems, attribute.SliceValue(inner.elems...))
	return errors.WithStack(err)
}

// AppendObject implements zapcore.ArrayEncoder for nested objects.
//
// Parameters:
//   - v: the object marshaler to encode.
//
// Return values:
//   - error: whatever v.MarshalLogObject reported.
func (a *arrayEncoder) AppendObject(v zapcore.ObjectMarshaler) error {
	inner := newObjectEncoder(2)
	inner.failure = a.failure
	inner.depth = a.depth + 1
	if inner.depth > maxLogEncodingDepth {
		*a.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	err := v.MarshalLogObject(inner)
	if err != nil {
		*a.failure = ErrLogEncoding
	}
	inner.calculate(inner.root)
	a.elems = append(a.elems, attribute.MapValue(inner.root.attrs...))
	return errors.WithStack(err)
}

// AppendReflected implements zapcore.ArrayEncoder for values with no dedicated
// zap type.
//
// Parameters:
//   - v: the value to convert.
//
// Return values:
//   - error: fixed failure when the declared JSON value cannot be encoded.
func (a *arrayEncoder) AppendReflected(v any) error {
	value, err := reflectedLogValue(v)
	if err != nil {
		*a.failure = ErrLogEncoding
		return ErrLogEncoding
	}
	a.elems = append(a.elems, value)
	return nil
}

// AppendByteString implements zapcore.ArrayEncoder for UTF-8 byte slices.
//
// Parameters:
//   - v: the bytes to append as a string.
//
// Return values: none.
func (a *arrayEncoder) AppendByteString(v []byte) {
	a.elems = append(a.elems, attribute.StringValue(string(v)))
}

// AppendBool implements zapcore.ArrayEncoder for boolean elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendBool(v bool) {
	a.elems = append(a.elems, attribute.BoolValue(v))
}

// AppendFloat64 implements zapcore.ArrayEncoder for 64-bit float elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendFloat64(v float64) {
	a.elems = append(a.elems, attribute.Float64Value(v))
}

// AppendInt implements zapcore.ArrayEncoder for platform-sized integers.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendInt(v int) {
	a.elems = append(a.elems, attribute.IntValue(v))
}

// AppendInt64 implements zapcore.ArrayEncoder for 64-bit integer elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendInt64(v int64) {
	a.elems = append(a.elems, attribute.Int64Value(v))
}

// AppendString implements zapcore.ArrayEncoder for string elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendString(v string) {
	a.elems = append(a.elems, attribute.StringValue(v))
}

// AppendComplex128 implements zapcore.ArrayEncoder for complex elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendComplex128(v complex128) {
	a.elems = append(a.elems, complexValue(v))
}

// AppendUint64 implements zapcore.ArrayEncoder for unsigned 64-bit elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUint64(v uint64) {
	a.elems = append(a.elems, convertUintValue(v))
}

// AppendFloat32 implements zapcore.ArrayEncoder for 32-bit float elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendFloat32(v float32) { a.AppendFloat64(float64(v)) }

// AppendComplex64 implements zapcore.ArrayEncoder for 64-bit complex elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendComplex64(v complex64) { a.AppendComplex128(complex128(v)) }

// AppendDuration implements zapcore.ArrayEncoder, recording nanoseconds.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendDuration(v time.Duration) { a.AppendInt64(v.Nanoseconds()) }

// AppendInt32 implements zapcore.ArrayEncoder for 32-bit integer elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendInt32(v int32) { a.AppendInt64(int64(v)) }

// AppendInt16 implements zapcore.ArrayEncoder for 16-bit integer elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendInt16(v int16) { a.AppendInt64(int64(v)) }

// AppendInt8 implements zapcore.ArrayEncoder for 8-bit integer elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendInt8(v int8) { a.AppendInt64(int64(v)) }

// AppendTime implements zapcore.ArrayEncoder, preserving a time's exact
// signed nanoseconds when possible and RFC3339Nano text otherwise.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendTime(v time.Time) {
	a.elems = append(a.elems, convertTimeValue(v))
}

// AppendUint implements zapcore.ArrayEncoder for platform-sized unsigned values.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUint(v uint) { a.AppendUint64(uint64(v)) }

// AppendUint32 implements zapcore.ArrayEncoder for 32-bit unsigned elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUint32(v uint32) { a.AppendInt64(int64(v)) }

// AppendUint16 implements zapcore.ArrayEncoder for 16-bit unsigned elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUint16(v uint16) { a.AppendInt64(int64(v)) }

// AppendUint8 implements zapcore.ArrayEncoder for 8-bit unsigned elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUint8(v uint8) { a.AppendInt64(int64(v)) }

// AppendUintptr implements zapcore.ArrayEncoder for pointer-sized elements.
//
// Parameters:
//   - v: the value to append.
//
// Return values: none.
func (a *arrayEncoder) AppendUintptr(v uintptr) { a.AppendUint64(uint64(v)) }
