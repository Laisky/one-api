package otelbridge

import (
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// countingLogStringer makes unexpected user callbacks observable without
// sending application content to an external exporter.
type countingLogStringer struct{ calls *int }

func (s countingLogStringer) String() string {
	*s.calls++
	return "evaluated"
}

func TestBridgeRejectsInvalidEntryBeforeSerializingFields(t *testing.T) {
	lg, output, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	err := lg.Core().Write(zapcore.Entry{
		Level: zapcore.InfoLevel,
		Message: string([]byte{0xff}),
	}, []zapcore.Field{zap.Stringer("unnecessary", countingLogStringer{&calls})})
	require.ErrorIs(t, err, ErrLogEncoding)
	require.Zero(t, calls, "a rejected entry must not evaluate the OTLP serializer")
	require.Empty(t, output.all())
}

func TestBridgeStopsSerializingAfterKnownFailure(t *testing.T) {
	lg, output, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	err := lg.Core().Write(zapcore.Entry{
		Level: zapcore.InfoLevel,
		Message: "will be rejected",
	}, []zapcore.Field{
		zap.Reflect("unsupported", make(chan int)),
		zap.Stringer("unnecessary", countingLogStringer{&calls}),
	})
	require.ErrorIs(t, err, ErrLogEncoding)
	require.Zero(t, calls, "a failed earlier field must short-circuit later OTLP serializers")
	require.Empty(t, output.all())
}

func TestBridgeStopsSerializingAfterNamespaceLimit(t *testing.T) {
	lg, output, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	fields := make([]zapcore.Field, 0, maxLogEncodingDepth+2)
	for range maxLogEncodingDepth + 1 {
		fields = append(fields, zap.Namespace("nested"))
	}
	fields = append(fields, zap.Stringer("unnecessary", countingLogStringer{&calls}))
	err := lg.Core().Write(zapcore.Entry{
		Level: zapcore.InfoLevel,
		Message: "will be rejected",
	}, fields)
	require.ErrorIs(t, err, ErrLogEncoding)
	require.Zero(t, calls, "a depth rejection must short-circuit later OTLP serializers")
	require.Empty(t, output.all())
}

func TestBridgeInvalidBoundStateSkipsLaterSerialization(t *testing.T) {
	lg, output, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	bound := lg.With(zap.Reflect("unsupported", make(chan int)))
	bound = bound.With(zap.Stringer("unnecessary", countingLogStringer{&calls}))
	require.Zero(t, calls, "an invalid bound OTLP core must not evaluate new bound serializers")
	err := bound.Core().Write(zapcore.Entry{Level: zapcore.InfoLevel, Message: "bad bound state"}, nil)
	require.ErrorIs(t, err, ErrLogEncoding)
	require.Empty(t, output.all())
}

func TestBridgeValidSerializerStillExecutesOnce(t *testing.T) {
	lg, output, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	lg.Info("valid", zap.Stringer("value", countingLogStringer{&calls}))
	require.Equal(t, 1, calls)
	value, ok := attrOf(output.only(t), "value")
	require.True(t, ok)
	require.Equal(t, "evaluated", value.AsString())
}
