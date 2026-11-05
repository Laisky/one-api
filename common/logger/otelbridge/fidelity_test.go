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

// TestLogFidelityBridgeDuplicateKeysResolveLastValueWins pins how a Zap field
// collision reaches OTLP.
//
// Zap allows the same key more than once; an OTLP attribute collection and
// every nested map must not hold one twice. The OpenTelemetry common
// specification resolves that by having a later value overwrite an earlier one,
// which is also what the SDK does by default and how every JSON reader of the
// local log line already interprets it. The bridge used to reject the whole
// record instead, and because the request logger and the process logger both
// bind "host", that dropped every request-scoped line in production.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogFidelityBridgeDuplicateKeysResolveLastValueWins(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(lg *zap.Logger)
		check func(t *testing.T, r sdklog.Record)
	}{
		{
			name: "process and request loggers both bind host",
			write: func(lg *zap.Logger) {
				lg.With(zap.String("host", "b4c6097d77f6")).
					With(zap.String("url", "/v1/chat/completions"), zap.String("host", "oneapi.example.com")).
					Info("select channel in cache", zap.Int("channel_id", 7))
			},
			check: func(t *testing.T, r sdklog.Record) {
				require.Equal(t, 1, attrCount(r, "host"))
				require.Equal(t, "oneapi.example.com", fidelityAttr(t, r, "host").AsString())
				require.Equal(t, "/v1/chat/completions", fidelityAttr(t, r, "url").AsString())
				require.EqualValues(t, 7, fidelityAttr(t, r, "channel_id").AsInt64())
			},
		},
		{
			name: "call site overrides a bound field",
			write: func(lg *zap.Logger) {
				lg.With(zap.String("model", "bound")).Info("relay finished", zap.String("model", "call-site"))
			},
			check: func(t *testing.T, r sdklog.Record) {
				require.Equal(t, 1, attrCount(r, "model"))
				require.Equal(t, "call-site", fidelityAttr(t, r, "model").AsString())
			},
		},
		{
			name: "repeated call-site field",
			write: func(lg *zap.Logger) {
				lg.Info("duplicate", zap.Int("count", 1), zap.Int("count", 2))
			},
			check: func(t *testing.T, r sdklog.Record) {
				require.Equal(t, 1, attrCount(r, "count"))
				require.EqualValues(t, 2, fidelityAttr(t, r, "count").AsInt64())
			},
		},
		{
			name: "repeated key inside an object",
			write: func(lg *zap.Logger) {
				lg.Info("duplicate nested", zap.Object("map", fidelityDuplicateObject{}))
			},
			check: func(t *testing.T, r sdklog.Record) {
				nested := fidelityAttr(t, r, "map").AsMap()
				require.Len(t, nested, 1)
				require.Equal(t, attribute.Key("count"), nested[0].Key)
				require.EqualValues(t, 2, nested[0].Value.AsInt64())
			},
		},
		{
			name: "repeated key inside a namespace",
			write: func(lg *zap.Logger) {
				lg.With(zap.Namespace("worker"), zap.Int("attempt", 1)).Info("retrying", zap.Int("attempt", 2))
			},
			check: func(t *testing.T, r sdklog.Record) {
				nested := fidelityAttr(t, r, "worker").AsMap()
				require.Len(t, nested, 1)
				require.EqualValues(t, 2, nested[0].Value.AsInt64())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := installLogExportSpy(t)
			lg, out, _ := newTestBridge(t, zapcore.InfoLevel)

			tc.write(lg)

			tc.check(t, out.only(t))
			require.Zero(t, spy.outcomes["dropped_encoding"], "a key collision is not an encoding failure")
			require.Equal(t, 1, spy.outcomes["emitted"])
			require.Equal(t, 1, spy.outcomes["deduplicated"],
				"overwriting a value is visible in metrics even though the record is kept")
		})
	}
}

// TestLogFidelityBridgeUniqueKeysAreNotCountedAsDeduplicated is the control for
// the test above: an ordinary record must not report a collision.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestLogFidelityBridgeUniqueKeysAreNotCountedAsDeduplicated(t *testing.T) {
	spy := installLogExportSpy(t)
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)

	lg.With(zap.String("host", "b4c6097d77f6")).Info("channels synced from database", zap.Object("map", fidelityObject{}))

	r := out.only(t)
	require.Equal(t, "b4c6097d77f6", fidelityAttr(t, r, "host").AsString())
	require.Equal(t, 1, spy.outcomes["emitted"])
	require.Zero(t, spy.outcomes["deduplicated"])
}

// attrCount returns how many top-level attributes of r use key.
//
// Parameters:
//   - r: the exported record.
//   - key: the attribute key.
//
// Return values:
//   - int: the number of attributes with that key.
func attrCount(r sdklog.Record, key string) int {
	n := 0
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == key {
			n++
		}
		return true
	})
	return n
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
