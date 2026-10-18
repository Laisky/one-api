package otelbridge

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// fidelityInlineFailure returns a real serializer error after writing a subset.
type fidelityInlineFailure struct{}

func (fidelityInlineFailure) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("partial", "must not export")
	return errors.New("synthetic inline failure")
}

// fidelityPanickingStringer exercises Zap's normally swallowed serializer panic.
type fidelityPanickingStringer struct{}

func (fidelityPanickingStringer) String() string { panic("synthetic stringer failure") }

// TestLogFidelitySerializerReviewFailures rejects whole records when an Inline
// encoder errors or a nonnil Stringer panics, preserving a valid Inline control.
func TestLogFidelitySerializerReviewFailures(t *testing.T) {
	for _, field := range []zap.Field{zap.Inline(fidelityInlineFailure{}), zap.Stringer("value", fidelityPanickingStringer{})} {
		lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
		require.Error(t, lg.Core().Write(zapcore.Entry{Message: "encoding", Level: zapcore.InfoLevel}, []zap.Field{field}))
		require.Empty(t, out.all())
	}
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("valid", zap.Inline(fidelityObject{}))
	require.EqualValues(t, 3, fidelityAttr(t, out.only(t), "count").AsInt64())
}

// TestLogFidelityTimeReviewRejectsOverflow prevents undefined UnixNano
// results in explicit time fields and array elements; zero field time is year1.
func TestLogFidelityTimeReviewRejectsOverflow(t *testing.T) {
	for _, value := range []time.Time{time.Time{}, time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)} {
		for _, field := range []zap.Field{zap.Time("time", value), zap.Times("times", []time.Time{time.Unix(0, 0), value})} {
			lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
			require.Error(t, lg.Core().Write(zapcore.Entry{Message: "overflow", Level: zapcore.InfoLevel}, []zap.Field{field}))
			require.Empty(t, out.all())
		}
	}
}

// TestLogFidelityTimeReviewBoundaries preserves the established exact int64-ns
// mapping at both edges, while an unset entry timestamp remains valid.
func TestLogFidelityTimeReviewBoundaries(t *testing.T) {
	for _, ns := range []int64{math.MinInt64, -1, 0, math.MaxInt64} {
		lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
		require.NoError(t, lg.Core().Write(zapcore.Entry{Message: "valid", Level: zapcore.InfoLevel}, []zap.Field{zap.Time("time", time.Unix(0, ns))}))
		require.Equal(t, ns, fidelityAttr(t, out.only(t), "time").AsInt64())
	}
}

// fidelityCountingError proves bound fields and their exception metadata are snapshots.
type fidelityCountingError struct {
	errors, types *int
	message       string
}

func (e *fidelityCountingError) Error() string     { *e.errors++; return e.message }
func (e *fidelityCountingError) ErrorType() string { *e.types++; return "synthetic.snapshot" }

// TestLogFidelityErrorReviewSnapshot encodes With errors once and preserves the
// chosen app field plus additive metadata without retaining or re-reading them.
func TestLogFidelityErrorReviewSnapshot(t *testing.T) {
	errors, types := 0, 0
	value := &fidelityCountingError{errors: &errors, types: &types, message: "first"}
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	bound := lg.With(zap.Error(value))
	require.Equal(t, 1, errors)
	require.Equal(t, 1, types)
	value.message = "changed later"
	bound.Info("one")
	bound.Info("two")
	require.Equal(t, 1, errors)
	require.Equal(t, 1, types)
	records := out.all()
	require.Len(t, records, 2)
	for _, record := range records {
		require.Equal(t, "first", fidelityAttr(t, record, "error").AsString())
		require.Equal(t, "first", fidelityAttr(t, record, "exception.message").AsString())
		require.Equal(t, "synthetic.snapshot", fidelityAttr(t, record, "exception.type").AsString())
	}
}

// TestLogFidelityErrorReviewOverrides preserves explicit exception metadata and
// avoids a second Error() or an unnecessary custom ErrorType() callback.
func TestLogFidelityErrorReviewOverrides(t *testing.T) {
	errors, types := 0, 0
	value := &fidelityCountingError{errors: &errors, types: &types, message: "chosen field"}
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	lg.Info("override", zap.Error(value), zap.String("exception.message", "explicit message"), zap.String("exception.type", "explicit type"))
	require.Equal(t, 1, errors)
	require.Zero(t, types)
	record := out.only(t)
	require.Equal(t, "chosen field", fidelityAttr(t, record, "error").AsString())
	require.Equal(t, "explicit message", fidelityAttr(t, record, "exception.message").AsString())
}
