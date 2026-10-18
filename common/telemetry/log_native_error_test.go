package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

type nativeFidelityError struct {
	message, kind           string
	panicMessage, panicType bool
	messageCalls, typeCalls *int
}

func (e nativeFidelityError) Error() string {
	if e.messageCalls != nil {
		*e.messageCalls++
	}
	if e.panicMessage {
		panic("synthetic Error panic")
	}
	return e.message
}
func (e nativeFidelityError) ErrorType() string {
	if e.typeCalls != nil {
		*e.typeCalls++
	}
	if e.panicType {
		panic("synthetic ErrorType panic")
	}
	return e.kind
}

type nativeFidelityNamedError string

func (e nativeFidelityNamedError) Error() string { return string(e) }

type nativeFidelityPointerError struct{}

func (*nativeFidelityPointerError) Error() string { return "pointer error" }

// nativeErrorRecords extracts records from the actual local HTTP exporter.
func nativeErrorRecords(groups []*logspb.ResourceLogs) []*logspb.LogRecord {
	var records []*logspb.LogRecord
	for _, group := range groups {
		for _, scope := range group.GetScopeLogs() {
			records = append(records, scope.GetLogRecords()...)
		}
	}
	return records
}

// TestLogFidelityNativeErrorRejectsInvalidDerivation requires callback failures
// and invalid text to reject the entire record with a fixed diagnostic.
func TestLogFidelityNativeErrorRejectsInvalidDerivation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  nativeFidelityError
	}{
		{"message invalid UTF8", nativeFidelityError{message: string([]byte{255}), kind: "valid.type"}},
		{"type invalid UTF8", nativeFidelityError{message: "valid message", kind: string([]byte{255})}},
		{"message panic", nativeFidelityError{panicMessage: true}},
		{"type panic", nativeFidelityError{message: "valid message", panicType: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := installLogExportSpy(t)
			p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
			var handled []error
			old := otel.GetErrorHandler()
			otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { handled = append(handled, err) }))
			t.Cleanup(func() { otel.SetErrorHandler(old) })
			var r otellog.Record
			r.SetBody(attribute.StringValue("synthetic original body"))
			r.AddAttributes(attribute.String("preserved", "synthetic original field"))
			r.SetErr(tc.err)
			require.NotPanics(t, func() { p.Logger("synthetic.native.error").Emit(context.Background(), r) })
			require.Empty(t, collect())
			require.Equal(t, 1, spy.outcome("dropped_encoding"))
			require.Len(t, handled, 1)
			require.Equal(t, "application log cannot be encoded faithfully", handled[0].Error())
		})
	}
}

// TestLogFidelityNativeErrorOverridesSkipCallbacks preserves explicit values and
// never evaluates a callback whose corresponding exception field is supplied.
func TestLogFidelityNativeErrorOverridesSkipCallbacks(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		attrs                   []attribute.KeyValue
		err                     nativeFidelityError
		wantMessage, wantType   string
		messageCalls, typeCalls int
	}{
		{"both", []attribute.KeyValue{attribute.String("exception.message", "explicit message"), attribute.String("exception.type", "explicit type")}, nativeFidelityError{panicMessage: true, panicType: true}, "explicit message", "explicit type", 0, 0},
		{"message", []attribute.KeyValue{attribute.String("exception.message", "explicit message")}, nativeFidelityError{panicMessage: true, kind: "derived type"}, "explicit message", "derived type", 0, 1},
		{"type", []attribute.KeyValue{attribute.String("exception.type", "explicit type")}, nativeFidelityError{message: "derived message", panicType: true}, "derived message", "explicit type", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
			var messageCalls, typeCalls int
			tc.err.messageCalls, tc.err.typeCalls = &messageCalls, &typeCalls
			var r otellog.Record
			r.SetBody(attribute.StringValue("original body"))
			r.AddAttributes(tc.attrs...)
			r.SetErr(tc.err)
			require.NotPanics(t, func() { p.Logger("synthetic.native.error").Emit(context.Background(), r) })
			records := nativeErrorRecords(collect())
			require.Len(t, records, 1)
			attrs := fidelityPBAttrs(records[0].GetAttributes())
			require.Equal(t, tc.wantMessage, attrs["exception.message"].GetStringValue())
			require.Equal(t, tc.wantType, attrs["exception.type"].GetStringValue())
			require.Equal(t, tc.messageCalls, messageCalls)
			require.Equal(t, tc.typeCalls, typeCalls)
		})
	}
}

// TestLogFidelityNativeErrorTypes follows the pinned SDK custom, named, pointer,
// and fmt-wrapped type rules rather than a bridge helper type.
func TestLogFidelityNativeErrorTypes(t *testing.T) {
	named := nativeFidelityNamedError("named message")
	pointer := &nativeFidelityPointerError{}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"custom", nativeFidelityError{message: "\u591a\u884c\noriginal", kind: "synthetic.Custom"}, "synthetic.Custom"},
		{"named", named, reflect.TypeOf(named).PkgPath() + "." + reflect.TypeOf(named).Name()},
		{"pointer", pointer, reflect.TypeOf(pointer).String()},
		{"fmt wrapped", fmt.Errorf("outer: %w", named), reflect.TypeOf(named).PkgPath() + "." + reflect.TypeOf(named).Name()},
		{"ordinary", errors.New("ordinary message"), "*errors.errorString"},
		{"empty custom", nativeFidelityError{message: "empty custom kind"}, reflect.TypeOf(nativeFidelityError{}).PkgPath() + ".nativeFidelityError"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
			var r otellog.Record
			r.SetBody(attribute.StringValue("original"))
			r.SetErr(tc.err)
			p.Logger("synthetic.native.error").Emit(context.Background(), r)
			records := nativeErrorRecords(collect())
			require.Len(t, records, 1)
			attrs := fidelityPBAttrs(records[0].GetAttributes())
			require.Equal(t, tc.err.Error(), attrs["exception.message"].GetStringValue())
			require.Equal(t, tc.want, attrs["exception.type"].GetStringValue())
		})
	}
}

// TestLogFidelityNativeErrorSingleEvaluationAndSnapshot protects callback count,
// original Err, and shared spare attribute capacity from mutation.
func TestLogFidelityNativeErrorSingleEvaluationAndSnapshot(t *testing.T) {
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	var messageCalls, typeCalls int
	err := nativeFidelityError{message: "original error", kind: "original.type", messageCalls: &messageCalls, typeCalls: &typeCalls}
	var r otellog.Record
	r.SetBody(attribute.StringValue("original body"))
	for i := 0; i < 8; i++ {
		r.AddAttributes(attribute.Int(fmt.Sprintf("field%d", i), i))
	}
	r.SetErr(err)
	alias := r
	alias.AddAttributes(attribute.String("alias.sentinel", "untouched"))
	p.Logger("synthetic.native.error").Emit(context.Background(), r)
	require.Equal(t, 1, messageCalls)
	require.Equal(t, 1, typeCalls)
	require.Equal(t, err, r.Err())
	require.Equal(t, 8, r.AttributesLen())
	var aliasAttrs []attribute.KeyValue
	alias.WalkAttributes(func(kv attribute.KeyValue) bool { aliasAttrs = append(aliasAttrs, kv); return true })
	require.Equal(t, attribute.String("alias.sentinel", "untouched"), aliasAttrs[len(aliasAttrs)-1])
	records := nativeErrorRecords(collect())
	require.Len(t, records, 1)
	require.Len(t, records[0].GetAttributes(), 10)
}

// TestLogFidelityNativeErrorIsResolvedBeforeDelegate proves the SDK cannot
// evaluate the error again, including when both attributes were explicit.
func TestLogFidelityNativeErrorIsResolvedBeforeDelegate(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			capture := &nativeErrorCapture{}
			lg := fidelityLogger{Logger: capture, validScope: true}
			var r otellog.Record
			r.SetErr(nativeFidelityError{message: "original", kind: "original.type"})
			if explicit {
				r.AddAttributes(attribute.String("exception.message", "override"), attribute.String("exception.type", "override.type"))
			}
			lg.Emit(context.Background(), r)
			require.Len(t, capture.records, 1)
			require.Nil(t, capture.records[0].Err())
			require.NotNil(t, r.Err())
		})
	}
}

type nativeErrorCapture struct {
	otellog.Logger
	records []otellog.Record
}

func (c *nativeErrorCapture) Emit(_ context.Context, r otellog.Record) {
	c.records = append(c.records, r.Clone())
}

// TestLogFidelityNativeTimestampsRejectBeforeSDK checks both original timestamps
// before the SDK can overflow or normalize them; unset and boundary values pass.
func TestLogFidelityNativeTimestampsRejectBeforeSDK(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value time.Time
		valid bool
	}{
		{"unset", time.Time{}, true},
		{"epoch", time.Unix(0, 0), true},
		{"safe upper bound", time.Unix(0, math.MaxInt64), true},
		{"before epoch", time.Unix(-1, 0), false},
		{"overflow signed nanos", time.Unix(0, math.MaxInt64).Add(time.Nanosecond), false},
	} {
		for _, observed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/observed=%t", tc.name, observed), func(t *testing.T) {
				spy := installLogExportSpy(t)
				var handled []error
				old := otel.GetErrorHandler()
				otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { handled = append(handled, err) }))
				t.Cleanup(func() { otel.SetErrorHandler(old) })
				capture := &nativeErrorCapture{}
				lg := fidelityLogger{Logger: capture, validScope: true}
				var r otellog.Record
				if observed {
					r.SetObservedTimestamp(tc.value)
				} else {
					r.SetTimestamp(tc.value)
				}
				lg.Emit(context.Background(), r)
				if tc.valid {
					require.Len(t, capture.records, 1)
					require.Empty(t, handled)
					require.Equal(t, 0, spy.outcome("dropped_encoding"))
				} else {
					require.Empty(t, capture.records)
					require.Len(t, handled, 1)
					require.Equal(t, "application log cannot be encoded faithfully", handled[0].Error())
					require.Equal(t, 1, spy.outcome("dropped_encoding"))
				}
			})
		}
	}
}
