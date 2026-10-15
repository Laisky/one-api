package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// alertTestLimiter admits synthetic alerts without production timers.
type alertTestLimiter struct{}

// Allow takes no arguments and returns true for a synthetic alert.
func (alertTestLimiter) Allow() bool { return true }

// alertTestEnvelope contains the real GraphQL request's synthetic routing metadata and message.
type alertTestEnvelope struct {
	Variables struct {
		Type    string `json:"type"`
		Token   string `json:"token"`
		Message string `json:"msg"`
	} `json:"variables"`
}

// alertCoreFixture takes t and limiter and returns an owned loopback transport, production core and captured request channel.
func alertCoreFixture(t *testing.T, limiter glog.RateLimiter) (*glog.Alert, zapcore.Core, <-chan alertTestEnvelope) {
	t.Helper()
	requests := make(chan alertTestEnvelope, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request alertTestEnvelope
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&request); err != nil {
			t.Errorf("decode synthetic request: %v", err)
			http.Error(w, "bad fixture request", 400)
			return
		}
		select {
		case requests <- request:
		default:
			t.Error("unexpected extra synthetic request")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)); err != nil {
			t.Errorf("reply to synthetic request: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	alert, core, err := NewAlertCore(ctx, server.URL, "synthetic-alert", "synthetic-auth-not-real", limiter)
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		cancel()
		select {
		case <-alert.Done():
		case <-time.After(5 * time.Second):
			t.Error("synthetic worker did not stop")
		}
	})
	return alert, core, requests
}

// receiveAlertJSON takes t and request channel and returns the actual delivered JSON object and raw message.
func receiveAlertJSON(t *testing.T, requests <-chan alertTestEnvelope) (map[string]any, string) {
	t.Helper()
	select {
	case request := <-requests:
		require.Equal(t, "synthetic-alert", request.Variables.Type)
		require.Equal(t, "synthetic-auth-not-real", request.Variables.Token)
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(request.Variables.Message), &fields))
		return fields, request.Variables.Message
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic alert was not delivered")
		return nil, ""
	}
}

// TestAlertCoreTransparentDelivery takes t, verifies ordinary diagnostics and fake-sensitive names survive real delivery without implicit policy, and returns no value.
func TestAlertCoreTransparentDelivery(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	logger := zap.New(core).Named("synthetic-complete").With(zap.String("trace_id", "caller-owned-trace"), zap.String("user_id", "caller-owned-user"))
	logger.Error("synthetic suspension", zap.Int("channel_id", 71003), zap.String("channel_name", "synthetic channel"), zap.String("origin_model", "synthetic-model"), zap.String("actual_model", "synthetic-mapped-model"), zap.String("group", "synthetic-group"), zap.Int("normalized_status_code", 503), zap.String("status_semantics", "normalized_relay_status"), zap.String("error_kind", "upstream"), zap.String("error_kind_basis", "normalized_status_fallback"), zap.Int64("suspension_duration_ms", -30000), zap.String("suspension_outcome", "persisted"), zap.String("authorization", "SYNTHETIC_NOT_REAL_SECRET"), zap.String("payload", "synthetic ordinary caller field"))
	got, _ := receiveAlertJSON(t, requests)
	for key, want := range map[string]any{"logger": "synthetic-complete", "message": "synthetic suspension", "level": "error", "trace_id": "caller-owned-trace", "user_id": "caller-owned-user", "channel_id": float64(71003), "channel_name": "synthetic channel", "origin_model": "synthetic-model", "actual_model": "synthetic-mapped-model", "group": "synthetic-group", "normalized_status_code": float64(503), "status_semantics": "normalized_relay_status", "error_kind": "upstream", "error_kind_basis": "normalized_status_fallback", "suspension_duration_ms": float64(-30000), "suspension_outcome": "persisted", "authorization": "SYNTHETIC_NOT_REAL_SECRET", "payload": "synthetic ordinary caller field"} {
		require.Equal(t, want, got[key], key)
	}
	_, err := time.Parse(time.RFC3339Nano, got["time"].(string))
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(got["time"].(string), "Z"))
}

// alertSerializationFixture provides ordinary Zap serializers with synthetic values and optional invocation counting.
type alertSerializationFixture struct{ calls atomic.Int32 }

// MarshalLogObject takes encoder, records a call and returns the result of encoding a synthetic object.
func (p *alertSerializationFixture) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	p.calls.Add(1)
	encoder.AddString("nested", "object-value")
	return nil
}

// MarshalLogArray takes encoder, records a call and returns no error after encoding a synthetic element.
func (p *alertSerializationFixture) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	p.calls.Add(1)
	encoder.AppendString("array-value")
	return nil
}

// MarshalJSON takes no arguments, records a call and returns a synthetic JSON object and no error.
func (p *alertSerializationFixture) MarshalJSON() ([]byte, error) {
	p.calls.Add(1)
	return []byte(`{"nested":"reflect-value"}`), nil
}

// String takes no arguments, records a call and returns a synthetic stringer value.
func (p *alertSerializationFixture) String() string { p.calls.Add(1); return "stringer-value" }

// Error takes no arguments, records a call and returns a synthetic error value.
func (p *alertSerializationFixture) Error() string { p.calls.Add(1); return "error-value" }

// TestAlertCoreOrdinarySerialization takes t, checks standard serializer and duplicate-key behavior reaches HTTP unchanged, and returns no value.
func TestAlertCoreOrdinarySerialization(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	probe := &alertSerializationFixture{}
	zap.New(core).With(zap.String("duplicate", "inherited")).Error("synthetic serializers", zap.Object("object", probe), zap.Array("array", probe), zap.Reflect("reflect", probe), zap.Stringer("stringer", probe), zap.Error(probe), zap.String("duplicate", "call-site"))
	got, raw := receiveAlertJSON(t, requests)
	require.Equal(t, map[string]any{"nested": "object-value"}, got["object"])
	require.Equal(t, []any{"array-value"}, got["array"])
	require.Equal(t, map[string]any{"nested": "reflect-value"}, got["reflect"])
	require.Equal(t, "stringer-value", got["stringer"])
	require.Equal(t, "error-value", got["error"])
	require.Equal(t, "call-site", got["duplicate"])
	require.Equal(t, 2, strings.Count(raw, `"duplicate":`))
	require.Equal(t, int32(5), probe.calls.Load())
}

// TestAlertCoreInheritedNamespaces takes t, compares nested context to an ordinary Zap JSON core and returns no value.
func TestAlertCoreInheritedNamespaces(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	var ordinary bytes.Buffer
	control := zapcore.NewCore(zapcore.NewJSONEncoder(zapcore.EncoderConfig{MessageKey: "message"}), zapcore.AddSync(&ordinary), zapcore.ErrorLevel)
	logger := zap.New(zapcore.NewTee(control, core)).With(zap.Namespace("request"), zap.String("inherited", "outer")).With(zap.Namespace("nested"), zap.Int("bound", 1))
	logger.Error("synthetic namespace", zap.String("model", "synthetic-model"), zap.String("authorization", "SYNTHETIC_NAMESPACE_VALUE"))
	got, _ := receiveAlertJSON(t, requests)
	var want map[string]any
	require.NoError(t, json.Unmarshal(ordinary.Bytes(), &want))
	require.Equal(t, want["request"], got["request"])
	require.Equal(t, want["message"], got["message"])
}

// TestAlertCoreExplicitRedaction takes t, proves caller-selected redaction affects all sinks without invoking the private value, and returns no value.
func TestAlertCoreExplicitRedaction(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	var local bytes.Buffer
	probe := &alertSerializationFixture{}
	control := zapcore.NewCore(zapcore.NewJSONEncoder(zapcore.EncoderConfig{MessageKey: "message"}), zapcore.AddSync(&local), zapcore.ErrorLevel)
	zap.New(zapcore.NewTee(control, core)).Error("synthetic redaction", zap.Any("selected_private_value", Redact(probe)), zap.String("authorization", Redact("SYNTHETIC_PRIVATE_SENTINEL")))
	got, raw := receiveAlertJSON(t, requests)
	require.Equal(t, "[REDACTED]", got["selected_private_value"])
	require.Equal(t, "[REDACTED]", got["authorization"])
	require.Contains(t, local.String(), "[REDACTED]")
	require.NotContains(t, raw, "SYNTHETIC_PRIVATE_SENTINEL")
	require.NotContains(t, local.String(), "SYNTHETIC_PRIVATE_SENTINEL")
	require.Zero(t, probe.calls.Load())
}

// TestAlertCoreOverflowVisible takes t, verifies oversized entries produce bounded valid JSON with a visible truncation marker, and returns no value.
func TestAlertCoreOverflowVisible(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	zap.New(core).Error("synthetic oversize", zap.String("ordinary", strings.Repeat("☃\n\"", 16000)))
	got, raw := receiveAlertJSON(t, requests)
	require.LessOrEqual(t, len(raw), 16*1024)
	require.True(t, utf8.ValidString(raw))
	require.Equal(t, true, got["alert_truncated"])
	require.Greater(t, got["original_bytes"], float64(16*1024))
	require.NotEmpty(t, got["preview"])
}

// alertBrokenObject returns a synthetic standard Zap serialization failure.
type alertBrokenObject struct{}

// MarshalLogObject takes an unused encoder and returns a synthetic failure for Zap's standard error-field reporting.
func (alertBrokenObject) MarshalLogObject(zapcore.ObjectEncoder) error {
	return errors.New("synthetic serializer failure")
}

// TestAlertCoreSerializationErrorVisible takes t, checks Zap's ordinary serialization-error diagnostic survives delivery, and returns no value.
func TestAlertCoreSerializationErrorVisible(t *testing.T) {
	_, core, requests := alertCoreFixture(t, alertTestLimiter{})
	zap.New(core).Error("synthetic marshal error", zap.Object("broken", alertBrokenObject{}))
	got, _ := receiveAlertJSON(t, requests)
	require.Contains(t, fmt.Sprint(got["brokenError"]), "synthetic serializer failure")
}

// alertCountingLimiter admits only the first queued record and counts sender-side checks.
type alertCountingLimiter struct{ calls atomic.Int32 }

// Allow takes no arguments and returns true only for its first sender-side invocation.
func (l *alertCountingLimiter) Allow() bool { return l.calls.Add(1) == 1 }

// TestAlertCoreOperationalContract takes t, verifies the ERROR floor, existing worker rate limit and closed-queue errors, and returns no value.
func TestAlertCoreOperationalContract(t *testing.T) {
	limiter := &alertCountingLimiter{}
	alert, core, requests := alertCoreFixture(t, limiter)
	logger := zap.New(core)
	logger.Warn("synthetic warning excluded by existing floor")
	logger.Error("synthetic first")
	logger.Error("synthetic second")
	logger.Error("synthetic third")
	got, _ := receiveAlertJSON(t, requests)
	require.Equal(t, "synthetic first", got["message"])
	require.Eventually(t, func() bool { return limiter.calls.Load() == 3 }, 5*time.Second, time.Millisecond)
	require.Empty(t, requests)
	alert.Close()
	select {
	case <-alert.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not close")
	}
	require.ErrorContains(t, core.Write(zapcore.Entry{Level: zapcore.ErrorLevel, Message: "synthetic after close", Time: time.Now().UTC()}, nil), "closed")
	require.NoError(t, core.Sync())
}
