package otelbridge

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TestPrivacyCoreExportsOnlyOperationalData verifies the production boundary
// filters bound/call-site fields, exception text, scope names, and log bodies,
// while the local sink and protocol-level trace correlation retain their data.
func TestPrivacyCoreExportsOnlyOperationalData(t *testing.T) {
	exporter := &recordingExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	local, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(zapcore.NewTee(local, NewPrivacyCore(holder, DefaultScopeName, zapcore.InfoLevel)))
	traceID := oteltrace.TraceID{1, 2, 3}
	spanID := oteltrace.SpanID{4, 5, 6}
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	logger = logger.Named("sentinel-scope-secret").With(
		zap.String("prompt", "sentinel-bound-prompt"),
		zap.String("url", "https://host/?unknown=sentinel-url-key"),
		zap.String("cookie", "sentinel-cookie-secret"),
		zap.Error(errors.New("sentinel-bound-error")),
		zap.Int("channel_id", 42), SpanContextFieldFrom(spanCtx))
	logger.Error("sentinel-message-secret",
		zap.String("request_body", "sentinel-request-secret"),
		zap.String("body_bytes", "sentinel-wrong-type"),
		zap.Int("body_bytes", 100), zap.Bool("is_error", true),
		zap.Error(errors.New("sentinel-error-secret")),
		zap.Any("headers", map[string]string{"Authorization": "sentinel-auth-key"}),
		zap.Object("response", zapcore.ObjectMarshalerFunc(func(zapcore.ObjectEncoder) error {
			return nil
		})))
	record := exporter.only(t)
	require.Equal(t, "application log", record.Body().AsString())
	require.Equal(t, traceID, record.TraceID())
	require.Equal(t, spanID, record.SpanID())
	require.Equal(t, DefaultScopeName, record.InstrumentationScope().Name)
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		switch string(kv.Key) {
		case "channel_id", "body_bytes":
			require.Equal(t, attribute.INT64, kv.Value.Type())
		case "is_error":
			require.Equal(t, attribute.BOOL, kv.Value.Type())
		default:
			t.Errorf("unexpected exported field %q", kv.Key)
		}
		return true
	})
	value, ok := attrOf(record, "body_bytes")
	require.True(t, ok)
	require.Equal(t, int64(100), value.AsInt64())
	require.Len(t, observed.All(), 1)
	require.Equal(t, "sentinel-message-secret", observed.All()[0].Message)
	require.Equal(t, "sentinel-request-secret", observed.All()[0].ContextMap()["request_body"])
}

// TestPrivacyCoreSkipsArbitrarySerializers verifies filtered objects are never
// evaluated by the OTLP core, including objects bound before the call site.
func TestPrivacyCoreSkipsArbitrarySerializers(t *testing.T) {
	core := NewPrivacyCore(NewProviderHolder(), DefaultScopeName, zapcore.InfoLevel)
	object := zap.Object("headers", zapcore.ObjectMarshalerFunc(func(zapcore.ObjectEncoder) error {
		t.Fatal("OTLP evaluated a filtered serializer")
		return nil
	}))
	bound := core.With([]zapcore.Field{object}).(*Core)
	require.Empty(t, bound.attrs)
	require.Empty(t, core.convertFields([]zapcore.Field{object}).attrs)
}

// TestPrivacyCoreRejectsInvalidNumericIdentifiers verifies numeric allowlisted
// keys cannot smuggle decimal strings, oversized IDs, NaN, or invalid statuses.
func TestPrivacyCoreRejectsInvalidNumericIdentifiers(t *testing.T) {
	core := NewPrivacyCore(NewProviderHolder(), DefaultScopeName, zapcore.InfoLevel)
	converted := core.convertFields([]zapcore.Field{
		zap.Uint64("channel_id", math.MaxUint64), zap.Int64("server_id", 1<<40),
		zap.Int("user_id", -1), zap.Float64("token_id", 1),
		zap.Float64("body_bytes", math.NaN()), zap.Float64("duration_ms", math.Inf(1)),
		zap.Int("status_code", 999), zap.Bool("body_bytes", true),
		zap.Int("channel_id", 42), zap.Int("body_bytes", 100),
	})
	require.ElementsMatch(t, []attribute.KeyValue{attribute.Int("channel_id", 42), attribute.Int("body_bytes", 100)}, converted.attrs)
}
