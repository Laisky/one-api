package otelbridge

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// fidelityObject exercises the application's explicit ObjectMarshaler contract.
type fidelityObject struct{ fail bool }

func (o fidelityObject) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("phase", "对象\nready")
	e.AddInt("count", 3)
	if o.fail {
		return errors.New("synthetic object encoding failure")
	}
	return nil
}

// TestLogFidelityBridgeNamespaceAcrossWith checks persistent Zap namespace state
// without reevaluating bound fields at each later log call.
func TestLogFidelityBridgeNamespaceAcrossWith(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.With(zap.Namespace("worker"), zap.Int("bound", 7)).With(zap.String("stage", "init")).
		Info("worker completed", zap.String("phase", "ready"))
	r := out.only(t)
	worker := fidelityAttr(t, r, "worker")
	require.Equal(t, attribute.MAP, worker.Type())
	fields := map[string]attribute.Value{}
	for _, kv := range worker.AsMap() {
		fields[string(kv.Key)] = kv.Value
	}
	require.EqualValues(t, 7, fields["bound"].AsInt64())
	require.Equal(t, "init", fields["stage"].AsString())
	require.Equal(t, "ready", fields["phase"].AsString())
	_, present := attrOf(r, "phase")
	require.False(t, present)
}

// TestLogFidelityBridgeFields covers declared Zap encoder mappings and original
// operational strings, including synthetic credential-shaped application data.
func TestLogFidelityBridgeFields(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	original := "worker token=SYNTHETIC_APPLICATION_VALUE\n多行 diagnostic"
	lg.Info(original, zap.String("prompt", "synthetic application field"), zap.Int("status", -3),
		zap.Error(errors.New("read body: synthetic application error")),
		zap.Object("object", fidelityObject{}),
		zap.Reflect("reflected", struct {
			Count int    `json:"count"`
			Label string `json:"label"`
		}{3, "ready"}),
		zap.Binary("bytes", []byte{0, 1, 255}), zap.Any("nil", nil))
	r := out.only(t)
	require.Equal(t, original, r.Body().AsString())
	require.Equal(t, "synthetic application field", fidelityAttr(t, r, "prompt").AsString())
	require.EqualValues(t, -3, fidelityAttr(t, r, "status").AsInt64())
	require.Equal(t, "read body: synthetic application error", fidelityAttr(t, r, "error").AsString())
	require.Equal(t, attribute.MAP, fidelityAttr(t, r, "object").Type())
	require.Equal(t, attribute.MAP, fidelityAttr(t, r, "reflected").Type())
	require.Equal(t, []byte{0, 1, 255}, fidelityAttr(t, r, "bytes").AsByteSlice())
	require.Equal(t, attribute.EMPTY, fidelityAttr(t, r, "nil").Type())
}

// TestLogFidelityBridgeDeepValidValue rejects the former arbitrary depth-eight
// replacement for a finite supported nested value.
func TestLogFidelityBridgeDeepValidValue(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	var value any = "leaf"
	for range 12 {
		value = map[string]any{"next": value}
	}
	lg.Info("nested", zap.Any("tree", value))
	tree := fidelityAttr(t, out.only(t), "tree")
	for range 12 {
		require.Equal(t, attribute.MAP, tree.Type())
		tree = tree.AsMap()[0].Value
	}
	require.Equal(t, "leaf", tree.AsString())
}

// TestLogFidelityBridgeEncodingFailure requires visible whole-record failure,
// rather than invented strings, repaired bytes or partial encoded fields.
func TestLogFidelityBridgeEncodingFailure(t *testing.T) {
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	for _, tc := range []struct {
		name, message string
		fields        []zap.Field
	}{
		{"invalid UTF8", "invalid " + string([]byte{255}), nil},
		{"reflected invalid UTF8", "invalid reflected", []zap.Field{zap.Reflect("value", struct{ Text string }{string([]byte{255})})}},
		{"duplicate attrs", "duplicate", []zap.Field{zap.Int("count", 1), zap.Int("count", 2)}},
		{"duplicate nested", "duplicate nested", []zap.Field{zap.Object("map", fidelityDuplicateObject{})}},
		{"unsupported", "unsupported", []zap.Field{zap.Reflect("channel", make(chan int))}},
		{"cycle", "cycle", []zap.Field{zap.Reflect("cycle", cyclic)}},
		{"object error", "object", []zap.Field{zap.Object("object", fidelityObject{fail: true})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
			err := lg.Core().Write(zapcore.Entry{Message: tc.message, Level: zapcore.InfoLevel}, tc.fields)
			require.Error(t, err)
			require.Empty(t, out.all(), "invalid record must not be partially exported")
			require.NotContains(t, err.Error(), "synthetic object encoding failure")
			require.NotContains(t, err.Error(), strings.Repeat("self", 3))
		})
	}
}

// fidelityAttr requires presence independently of EMPTY/nil value semantics.
func fidelityAttr(t *testing.T, r sdklog.Record, key string) attribute.Value {
	t.Helper()
	v, ok := attrOf(r, key)
	require.True(t, ok, key)
	return v
}

// fidelityDuplicateObject supplies an actual duplicate protocol map input.
type fidelityDuplicateObject struct{}

func (fidelityDuplicateObject) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddInt("count", 1)
	e.AddInt("count", 2)
	return nil
}

// fidelityContextStringer is application data, not the explicit correlation API.
type fidelityContextStringer struct{ context.Context }

func (fidelityContextStringer) String() string { return "application context value" }

// fidelitySnapshotObject tracks ordinary With-time serialization.
type fidelitySnapshotObject struct {
	count int
	phase string
}

func (o *fidelitySnapshotObject) MarshalLogObject(e zapcore.ObjectEncoder) error {
	o.count++
	e.AddString("phase", o.phase)
	return nil
}

// TestLogFidelityBoundSnapshotAndMetadata keeps app fields, explicit encoders and
// caller metadata in their own contracts without invisible context consumption.
func TestLogFidelityBoundSnapshotAndMetadata(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	object := &fidelitySnapshotObject{phase: "initial"}
	bound := lg.With(zap.String("code.file.path", "application supplied"), zap.Object("object", object))
	object.phase = "changed"
	bound.WithOptions(zap.AddCaller()).Info("first", zap.Stringer("context", fidelityContextStringer{context.Background()}))
	bound.Info("second")
	require.Equal(t, 1, object.count)
	records := out.all()
	require.Len(t, records, 2)
	require.Equal(t, "application supplied", fidelityAttr(t, records[0], "code.file.path").AsString())
	require.Equal(t, "application context value", fidelityAttr(t, records[0], "context").AsString())
	for _, r := range records {
		require.Equal(t, "initial", fidelityAttr(t, r, "object").AsMap()[0].Value.AsString())
	}
}

// fidelityCustomJSON projects only the caller-declared JSON contract.
type fidelityCustomJSON struct{ privateState string }

func (fidelityCustomJSON) MarshalJSON() ([]byte, error) {
	return []byte(`{"integer":9007199254740993,"empty":null,"list":[]}`), nil
}

// TestLogFidelityReflectedIntegerPrecision asserts actual int64 values rather
// than float-rounded JSON equality, including the caller's custom JSON mapping.
func TestLogFidelityReflectedIntegerPrecision(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("precision",
		zap.Reflect("struct", struct {
			Count int64 `json:"count"`
		}{9007199254740993}),
		zap.Reflect("map", map[string]any{"count": int64(-9007199254740993), "number": json.Number("9007199254740993")}),
		zap.Reflect("custom", fidelityCustomJSON{privateState: "not in declared JSON"}),
		zap.Uint64("large_unsigned", math.MaxUint64))
	r := out.only(t)
	lookup := func(v attribute.Value, key string) attribute.Value {
		for _, kv := range v.AsMap() {
			if string(kv.Key) == key {
				return kv.Value
			}
		}
		t.Fatalf("missing %s", key)
		return attribute.Value{}
	}
	require.EqualValues(t, 9007199254740993, lookup(fidelityAttr(t, r, "struct"), "count").AsInt64())
	require.EqualValues(t, -9007199254740993, lookup(fidelityAttr(t, r, "map"), "count").AsInt64())
	require.EqualValues(t, 9007199254740993, lookup(fidelityAttr(t, r, "map"), "number").AsInt64())
	custom := fidelityAttr(t, r, "custom")
	require.Len(t, custom.AsMap(), 3)
	require.EqualValues(t, 9007199254740993, lookup(custom, "integer").AsInt64())
	require.Equal(t, attribute.EMPTY, lookup(custom, "empty").Type())
	require.Equal(t, attribute.SLICE, lookup(custom, "list").Type())
	require.Empty(t, lookup(custom, "list").AsSlice())
	require.Equal(t, "18446744073709551615", fidelityAttr(t, r, "large_unsigned").AsString())
}
