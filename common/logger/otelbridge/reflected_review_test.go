package otelbridge

import (
	"encoding/json"
	"math"
	"testing"
	"unicode/utf8"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// fidelityHiddenEmbedding has an exported JSON-visible descendant.
type fidelityHiddenEmbedding struct{ Text string }

// fidelityTextProjection chooses a valid textual projection of its internal state.
type fidelityTextProjection string

func (fidelityTextProjection) MarshalText() ([]byte, error) {
	return []byte("chosen valid projection"), nil
}

// fidelityJSONProjection supplies the caller's chosen explicit JSON document.
type fidelityJSONProjection string

func (v fidelityJSONProjection) MarshalJSON() ([]byte, error) { return []byte(v), nil }

// TestLogFidelityReflectedReviewEmbeddedInvalid rejects bytes that encoding/json
// would otherwise replace inside exported children of an unexported embedding.
func TestLogFidelityReflectedReviewEmbeddedInvalid(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	value := struct{ fidelityHiddenEmbedding }{fidelityHiddenEmbedding{Text: string([]byte{255})}}
	require.Error(t, lg.Core().Write(zapcore.Entry{Message: "embedded", Level: zapcore.InfoLevel}, []zap.Field{zap.Reflect("value", value)}))
	require.Empty(t, out.all())
}

// TestLogFidelityReflectedReviewTextProjection validates the declared encoding,
// not hidden internal text that the selected TextMarshaler never emits.
func TestLogFidelityReflectedReviewTextProjection(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	require.NoError(t, lg.Core().Write(zapcore.Entry{Message: "text", Level: zapcore.InfoLevel}, []zap.Field{zap.Reflect("value", fidelityTextProjection(string([]byte{255})))}))
	require.Equal(t, "chosen valid projection", fidelityAttr(t, out.only(t), "value").AsString())
}

// TestLogFidelityReflectedReviewSurrogates rejects unpaired JSON surrogate
// escapes without rejecting valid pairs or literal backslash-u text.
func TestLogFidelityReflectedReviewSurrogates(t *testing.T) {
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `{"\ud800":"value"}`} {
		lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
		require.Error(t, lg.Core().Write(zapcore.Entry{Message: "invalid surrogate", Level: zapcore.InfoLevel}, []zap.Field{zap.Reflect("value", fidelityJSONProjection(raw))}))
		require.Empty(t, out.all())
	}
	for _, tc := range []struct{ raw, want string }{{`"\ud83d\ude00"`, "😀"}, {`"\\ud800"`, `\ud800`}} {
		lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
		lg.Info("valid", zap.Reflect("value", fidelityJSONProjection(tc.raw)))
		require.Equal(t, tc.want, fidelityAttr(t, out.only(t), "value").AsString())
	}
}

// TestLogFidelityReflectedReviewPreciseNumbers permits binary64 only when its
// JSON round trip retains the original numeric value; exact text is the fallback.
func TestLogFidelityReflectedReviewPreciseNumbers(t *testing.T) {
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("precision", zap.Reflect("value", fidelityJSONProjection(`{"fraction":12345678901234567890.125,"exponent":1.0000000000000000001e20,"ordinary":0.1,"negative_zero":-0}`)))
	values := map[string]attribute.Value{}
	for _, kv := range fidelityAttr(t, out.only(t), "value").AsMap() {
		values[string(kv.Key)] = kv.Value
	}
	require.Equal(t, attribute.STRING, values["fraction"].Type())
	require.Equal(t, "12345678901234567890.125", values["fraction"].AsString())
	require.Equal(t, "1.0000000000000000001e20", values["exponent"].AsString())
	require.Equal(t, 0.1, values["ordinary"].AsFloat64())
	require.Equal(t, attribute.FLOAT64, values["negative_zero"].Type())
	require.True(t, math.Signbit(values["negative_zero"].AsFloat64()))
}

// fidelityInvalidTextOutput exposes only its declared JSON textual projection.
type fidelityInvalidTextOutput struct{ calls *int }

func (v fidelityInvalidTextOutput) MarshalText() ([]byte, error) { *v.calls++; return []byte{255}, nil }

// fidelityOmittedText is omitted by its explicit IsZero contract.
type fidelityOmittedText struct {
	Text  string
	calls *int
}

func (v fidelityOmittedText) IsZero() bool { *v.calls++; return true }

// TestLogFidelityReflectedReviewChosenJSONBoundary verifies explicit projection
// is called once, including JSON's documented TextMarshaler Unicode conversion.
func TestLogFidelityReflectedReviewChosenJSONBoundary(t *testing.T) {
	calls := 0
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("chosen", zap.Reflect("value", fidelityInvalidTextOutput{calls: &calls}))
	require.Equal(t, 1, calls)
	require.Equal(t, string(utf8.RuneError), fidelityAttr(t, out.only(t), "value").AsString())
	native := attribute.Int64Value(42)
	encoded, err := json.Marshal(native)
	require.NoError(t, err)
	want, err := reflectedLogValue(json.RawMessage(encoded))
	require.NoError(t, err)
	got, err := reflectedLogValue(native)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// TestLogFidelityReflectedReviewVisibleFields skips omitted and shadowed internals.
func TestLogFidelityReflectedReviewVisibleFields(t *testing.T) {
	type embedded struct{ Hidden string }
	value := struct {
		embedded
		Hidden  string
		Omitted fidelityOmittedText `json:",omitzero"`
	}{embedded: embedded{Hidden: string([]byte{255})}, Hidden: "chosen", Omitted: fidelityOmittedText{Text: string([]byte{255})}}
	calls := 0
	value.Omitted.calls = &calls
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("visible", zap.Reflect("value", value))
	require.Equal(t, 1, calls)
	got := fidelityAttr(t, out.only(t), "value").AsMap()
	require.Len(t, got, 1)
	require.Equal(t, "Hidden", string(got[0].Key))
	require.Equal(t, "chosen", got[0].Value.AsString())
}

// fidelityProjectedKey is a caller-selected map-key JSON projection.
type fidelityProjectedKey struct{ calls *int }

func (key fidelityProjectedKey) MarshalText() ([]byte, error) {
	*key.calls++
	return []byte("chosen-key"), nil
}

// TestLogFidelityReflectedReviewMapProjection avoids re-running key/omission
// callbacks merely to validate already materialized caller-selected JSON.
func TestLogFidelityReflectedReviewMapProjection(t *testing.T) {
	for _, textKey := range []bool{false, true} {
		keyCalls, zeroCalls := 0, 0
		child := struct {
			Hidden fidelityOmittedText `json:",omitzero"`
		}{Hidden: fidelityOmittedText{Text: string([]byte{255}), calls: &zeroCalls}}
		var value any = map[int]any{42: child}
		if textKey {
			value = map[fidelityProjectedKey]any{{calls: &keyCalls}: child}
		}
		lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
		lg.Info("projection", zap.Reflect("value", value))
		require.Len(t, out.all(), 1)
		require.Equal(t, 1, zeroCalls)
		if textKey {
			require.Equal(t, 1, keyCalls)
		}
	}
}
