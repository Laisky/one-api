package logger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

const diagnosticLegacyTrace = "123456789abcdef:23456789abcdef1::4"
const diagnosticNativeTrace = "123456789abcdef0123456789abcdef0"
const diagnosticSecret = "SYNTHETIC_PRIVATE_ALERT_FIELD"

// diagnosticLimiter allows fixture delivery without timers or production limits.
type diagnosticLimiter struct{}

// Allow takes no arguments and returns true to admit synthetic messages without starting a rate-limiter worker.
func (diagnosticLimiter) Allow() bool { return true }

// diagnosticEnvelope contains only the synthetic GraphQL request sent to the loopback fixture.
type diagnosticEnvelope struct {
	Variables struct {
		Type    string `json:"type"`
		Token   string `json:"token"`
		Message string `json:"msg"`
	} `json:"variables"`
}

// diagnosticAlertFixture takes t and returns a real loopback alert sender and request channel; cleanup joins the sender.
func diagnosticAlertFixture(t *testing.T) (*glog.Alert, <-chan diagnosticEnvelope) {
	t.Helper()
	requests := make(chan diagnosticEnvelope, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope diagnosticEnvelope
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&envelope); err != nil {
			t.Errorf("decode synthetic alert: %v", err)
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		select {
		case requests <- envelope:
		default:
			t.Error("unexpected extra synthetic webhook request")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)); err != nil {
			t.Errorf("reply to synthetic alert: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	alert, err := newDiagnosticAlert(ctx, server.URL, "synthetic-diagnostic", "synthetic-auth-not-real", diagnosticLimiter{})
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		cancel()
		select {
		case <-alert.Done():
		case <-time.After(5 * time.Second):
			t.Error("synthetic alert worker did not stop")
		}
	})
	return alert, requests
}

// diagnosticReceive takes t and a request channel, and returns the decoded trailing field object and full synthetic message.
func diagnosticReceive(t *testing.T, requests <-chan diagnosticEnvelope) (map[string]any, string) {
	t.Helper()
	select {
	case request := <-requests:
		require.Equal(t, "synthetic-diagnostic", request.Variables.Type)
		require.Equal(t, "synthetic-auth-not-real", request.Variables.Token)
		message := request.Variables.Message
		lines := strings.Split(message, "\n")
		require.NotEmpty(t, lines)
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &fields))
		require.NotContains(t, message, diagnosticSecret, "unrelated fields must not cross the webhook boundary")
		return fields, message
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic webhook did not receive an alert")
		return nil, ""
	}
}

// TestDiagnosticAlertInheritedIDsExported takes t, checks inherited canonical IDs through real Zap and webhook delivery, and returns no value.
func TestDiagnosticAlertInheritedIDsExported(t *testing.T) {
	for _, traceID := range []string{diagnosticLegacyTrace, diagnosticNativeTrace} {
		t.Run(traceID, func(t *testing.T) {
			alert, requests := diagnosticAlertFixture(t)
			logger, err := glog.New(glog.WithName("synthetic-diagnostic"), glog.WithOutputPaths([]string{}),
				glog.WithZapOptions(zap.HooksWithFields(diagnosticAlertHook(alert))))
			require.NoError(t, err)
			bound := logger.With(zap.String("trace_id", traceID), zap.Int("user_id", 71002),
				zap.String("username", diagnosticSecret), zap.String("user_uuid", diagnosticSecret),
				zap.String("token_name", diagnosticSecret), zap.String("authorization", diagnosticSecret))
			bound.Error("synthetic fixed diagnostic event", zap.Int("user_id", 71002),
				zap.String("email", diagnosticSecret), zap.String("body", diagnosticSecret),
				zap.String("request_url", diagnosticSecret), zap.String("upstream_error", diagnosticSecret),
				zap.Int("channel_id", 71003), zap.Int("status_code", 503))
			fields, message := diagnosticReceive(t, requests)
			require.Equal(t, map[string]any{"trace_id": traceID, "user_id": float64(71002)}, fields)
			require.Equal(t, 1, strings.Count(message, `"user_id"`), "duplicate equal bound and call-site IDs must be canonicalized")
			require.Contains(t, message, "logger: `synthetic-diagnostic`")
			require.Contains(t, message, "level: `error`")
			require.Contains(t, message, "message: `synthetic fixed diagnostic event`")
		})
	}
}

// TestDiagnosticAlertRejectsInvalidCorrelation takes t, verifies the transport rejects unsupported ID values, and returns no value.
func TestDiagnosticAlertRejectsInvalidCorrelation(t *testing.T) {
	cases := []struct {
		name   string
		fields []zapcore.Field
		want   map[string]any
	}{
		{"malicious_trace", []zapcore.Field{zap.String("trace_id", diagnosticSecret+"@example.invalid"), zap.Int("user_id", 71002)}, map[string]any{"user_id": float64(71002)}},
		{"newline_trace", []zapcore.Field{zap.String("trace_id", diagnosticLegacyTrace+"\n"+diagnosticSecret)}, map[string]any{}},
		{"oversized_trace", []zapcore.Field{zap.String("trace_id", strings.Repeat("a", 65))}, map[string]any{}},
		{"zero_native", []zapcore.Field{zap.String("trace_id", strings.Repeat("0", 32))}, map[string]any{}},
		{"zero_legacy_trace", []zapcore.Field{zap.String("trace_id", "0:23456789abcdef1::4")}, map[string]any{}},
		{"zero_legacy_span", []zapcore.Field{zap.String("trace_id", "123456789abcdef:0::4")}, map[string]any{}},
		{"too_long_legacy_trace", []zapcore.Field{zap.String("trace_id", "123456789abcdef00:23456789abcdef1::4")}, map[string]any{}},
		{"invalid_parent", []zapcore.Field{zap.String("trace_id", "123456789abcdef:23456789abcdef1:private:4")}, map[string]any{}},
		{"invalid_flags", []zapcore.Field{zap.String("trace_id", "123456789abcdef:23456789abcdef1::100")}, map[string]any{}},
		{"valid_parent", []zapcore.Field{zap.String("trace_id", "123456789abcdef:23456789abcdef1:1:0")}, map[string]any{"trace_id": "123456789abcdef:23456789abcdef1:1:0"}},
		{"valid_uppercase", []zapcore.Field{zap.String("trace_id", strings.ToUpper(diagnosticNativeTrace))}, map[string]any{"trace_id": strings.ToUpper(diagnosticNativeTrace)}},
		{"numeric_trace", []zapcore.Field{zap.Int("trace_id", 71002)}, map[string]any{}},
		{"string_user", []zapcore.Field{zap.String("user_id", diagnosticSecret), zap.String("trace_id", diagnosticLegacyTrace)}, map[string]any{"trace_id": diagnosticLegacyTrace}},
		{"zero_user", []zapcore.Field{zap.Int("user_id", 0)}, map[string]any{}},
		{"negative_user", []zapcore.Field{zap.Int("user_id", -1)}, map[string]any{}},
		{"float_user", []zapcore.Field{zap.Float64("user_id", 71002)}, map[string]any{}},
		{"unsigned_user", []zapcore.Field{zap.Uint64("user_id", 71002)}, map[string]any{}},
		{"boolean_user", []zapcore.Field{zap.Bool("user_id", true)}, map[string]any{}},
		{"duplicate_equal", []zapcore.Field{zap.String("trace_id", diagnosticLegacyTrace), zap.String("trace_id", diagnosticLegacyTrace), zap.Int("user_id", 71002), zap.Int("user_id", 71002)}, map[string]any{"trace_id": diagnosticLegacyTrace, "user_id": float64(71002)}},
		{"valid_then_invalid_trace", []zapcore.Field{zap.String("trace_id", diagnosticLegacyTrace), zap.String("trace_id", diagnosticSecret)}, map[string]any{}},
		{"invalid_then_valid_trace", []zapcore.Field{zap.String("trace_id", diagnosticSecret), zap.String("trace_id", diagnosticLegacyTrace)}, map[string]any{}},
		{"valid_then_invalid_user", []zapcore.Field{zap.Int("user_id", 71002), zap.String("user_id", diagnosticSecret)}, map[string]any{}},
		{"invalid_then_valid_user", []zapcore.Field{zap.String("user_id", diagnosticSecret), zap.Int("user_id", 71002)}, map[string]any{}},
		{"smallest_legacy", []zapcore.Field{zap.String("trace_id", "1:1::0")}, map[string]any{"trace_id": "1:1::0"}},
		{"largest_legacy", []zapcore.Field{zap.String("trace_id", "123456789abcdef0:23456789abcdef01:3456789abcdef012:ff")}, map[string]any{"trace_id": "123456789abcdef0:23456789abcdef01:3456789abcdef012:ff"}},
		{"native_wrong_width", []zapcore.Field{zap.String("trace_id", diagnosticNativeTrace[:31])}, map[string]any{}},
		{"conflicting_users", []zapcore.Field{zap.Int("user_id", 71002), zap.Int("user_id", 71003)}, map[string]any{}},
		{"conflicting_traces", []zapcore.Field{zap.String("trace_id", diagnosticLegacyTrace), zap.String("trace_id", diagnosticNativeTrace)}, map[string]any{}},
		{"namespace", []zapcore.Field{zap.Namespace("private"), zap.String("trace_id", diagnosticLegacyTrace), zap.Int("user_id", 71002)}, map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alert, requests := diagnosticAlertFixture(t)
			require.NoError(t, diagnosticAlertHook(alert)(zapcore.Entry{LoggerName: "synthetic-diagnostic", Level: zapcore.ErrorLevel, Message: "synthetic fixed diagnostic event", Time: time.Unix(1, 0).UTC()}, tc.fields))
			fields, _ := diagnosticReceive(t, requests)
			require.Equal(t, tc.want, fields)
		})
	}
}

// diagnosticSerializationProbe detects any object, array, reflection, stringer or error serializer invoked by the remote hook.
type diagnosticSerializationProbe struct{ calls atomic.Int32 }

// MarshalLogObject takes an unused encoder, records unexpected serialization, and returns no error.
func (p *diagnosticSerializationProbe) MarshalLogObject(zapcore.ObjectEncoder) error {
	p.calls.Add(1)
	return nil
}

// MarshalLogArray takes an unused encoder, records unexpected serialization, and returns no error.
func (p *diagnosticSerializationProbe) MarshalLogArray(zapcore.ArrayEncoder) error {
	p.calls.Add(1)
	return nil
}

// MarshalJSON takes no arguments, records unexpected serialization, and returns a synthetic sentinel and no error.
func (p *diagnosticSerializationProbe) MarshalJSON() ([]byte, error) {
	p.calls.Add(1)
	return []byte(`"` + diagnosticSecret + `"`), nil
}

// String takes no arguments, records unexpected serialization, and returns a synthetic private sentinel.
func (p *diagnosticSerializationProbe) String() string { p.calls.Add(1); return diagnosticSecret }

// Error takes no arguments, records unexpected serialization, and returns a synthetic private sentinel.
func (p *diagnosticSerializationProbe) Error() string { p.calls.Add(1); return diagnosticSecret }

// TestDiagnosticAlertDoesNotSerializePrivateFields takes t, verifies rejected values never invoke serializers, and returns no value.
func TestDiagnosticAlertDoesNotSerializePrivateFields(t *testing.T) {
	alert, requests := diagnosticAlertFixture(t)
	probe := &diagnosticSerializationProbe{}
	require.NoError(t, diagnosticAlertHook(alert)(zapcore.Entry{Level: zapcore.ErrorLevel, Message: "synthetic fixed diagnostic event", Time: time.Unix(1, 0).UTC()}, []zapcore.Field{
		zap.Object("trace_id", probe), zap.Array("user_id", probe), zap.Reflect("payload", probe),
		zap.Stringer("user_id", probe), zap.Error(probe), zap.Binary("trace_id", []byte(diagnosticSecret)),
		zap.ByteString("user_id", []byte(diagnosticSecret)), zap.String("cookie", diagnosticSecret),
		zap.String("password", diagnosticSecret), zap.String("request_id", diagnosticSecret),
	}))
	fields, _ := diagnosticReceive(t, requests)
	require.Empty(t, fields)
	require.Zero(t, probe.calls.Load())
}

// TestDiagnosticAlertEntryMetadataBoundaryPreserved takes t, verifies entry metadata remains outside field filtering, and returns no value.
func TestDiagnosticAlertEntryMetadataBoundaryPreserved(t *testing.T) {
	alert, requests := diagnosticAlertFixture(t)
	entry := zapcore.Entry{LoggerName: "synthetic-metadata", Level: zapcore.ErrorLevel, Message: "synthetic message boundary marker", Time: time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("synthetic-zone", 3600))}
	require.NoError(t, diagnosticAlertHook(alert)(entry, nil))
	fields, message := diagnosticReceive(t, requests)
	require.Empty(t, fields)
	require.Contains(t, message, "logger: `synthetic-metadata`")
	require.Contains(t, message, "message: `synthetic message boundary marker`")
	require.Contains(t, message, "time: `2026-01-02T02:04:05.000000006Z`")
}

// TestDiagnosticAlertInheritedNamespaceRejected takes t, verifies inherited namespaces cannot export nested IDs as top-level fields, and returns no value.
func TestDiagnosticAlertInheritedNamespaceRejected(t *testing.T) {
	alert, requests := diagnosticAlertFixture(t)
	logger, err := glog.New(glog.WithName("synthetic-diagnostic"), glog.WithOutputPaths([]string{}),
		glog.WithZapOptions(zap.HooksWithFields(diagnosticAlertHook(alert))))
	require.NoError(t, err)
	logger.With(zap.Namespace("payload")).Error("synthetic fixed diagnostic event",
		zap.String("trace_id", diagnosticLegacyTrace), zap.Int("user_id", 71002))
	fields, _ := diagnosticReceive(t, requests)
	require.Empty(t, fields)
}
