package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
)

// TestGlobalSDKLogsArePrivateBeforeAdmission verifies direct global Emit calls
// cannot bypass privacy via body, event, scope options, attributes, or resources.
func TestGlobalSDKLogsArePrivateBeforeAdmission(t *testing.T) {
	capture := &captureProcessor{}
	gate := newBoundedProcessor(capture, 10, 4096)
	resource := privateResource(sdkresource.NewSchemaless(attribute.String("service.name", "one-api"),
		attribute.String("secret", "sentinel-resource-secret")))
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(privacyLoggerProvider{LoggerProvider: provider})
	t.Cleanup(func() { global.SetLoggerProvider(previous) })
	logger := global.Logger("sentinel-scope-secret", otellog.WithInstrumentationVersion("sentinel-version-secret"),
		otellog.WithSchemaURL("https://sentinel-schema-secret"),
		otellog.WithInstrumentationAttributes(attribute.String("secret", "sentinel-scope-attribute")))
	stamp := time.Now()
	var record otellog.Record
	record.SetTimestamp(stamp)
	record.SetBody(attribute.StringValue("sentinel-body-secret"))
	record.SetEventName("sentinel-event-secret")
	record.SetSeverity(otellog.SeverityError)
	record.SetSeverityText("sentinel-severity-secret")
	record.AddAttributes(attribute.String("error", "sentinel-error-secret"),
		attribute.String("code.file.path", "sentinel-forged-caller"),
		attribute.String("cookie", "sentinel-cookie-secret"),
		attribute.Map("headers", attribute.String("Authorization", "sentinel-key")),
		attribute.Int("channel_id", 7), attribute.Int("status_code", 503),
		attribute.Int64("server_id", 1<<40), attribute.Int("token_id", -1),
		attribute.Float64("user_id", 123), attribute.Int("body_bytes", 100))
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1, 2, 3}, SpanID: trace.SpanID{4, 5, 6}})
	logger.Emit(trace.ContextWithSpanContext(context.Background(), sc), record)
	capture.mu.Lock()
	got := capture.last.Clone()
	capture.mu.Unlock()
	require.Equal(t, int64(1), gate.residentRecords.Load())
	require.Equal(t, "application log", got.Body().AsString())
	require.Empty(t, got.EventName())
	require.Equal(t, otellog.SeverityError.String(), got.SeverityText())
	require.Equal(t, stamp, got.Timestamp())
	require.Equal(t, sc.TraceID(), got.TraceID())
	require.Equal(t, sc.SpanID(), got.SpanID())
	require.Equal(t, otelbridge.DefaultScopeName, got.InstrumentationScope().Name)
	require.Empty(t, got.InstrumentationScope().Version)
	require.Empty(t, got.InstrumentationScope().SchemaURL)
	scope := got.InstrumentationScope()
	require.Equal(t, 0, scope.Attributes.Len())
	var attrs []attribute.KeyValue
	got.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	require.ElementsMatch(t, []attribute.KeyValue{attribute.Int("channel_id", 7), attribute.Int("status_code", 503), attribute.Int("body_bytes", 100)}, attrs)
	require.Equal(t, []attribute.KeyValue{attribute.String("service.name", "one-api")}, got.Resource().Attributes())
}

// TestRawSDKUnsafeScopeIsDroppedBeforeAdmission verifies an internal raw SDK
// provider reference cannot enqueue immutable unsafe scope metadata.
func TestRawSDKUnsafeScopeIsDroppedBeforeAdmission(t *testing.T) {
	spy := installLogExportSpy(t)
	next := &nopProcessor{}
	gate := newBoundedProcessor(next, 10, 4096)
	resource := sdkresource.Empty()
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	var record otellog.Record
	record.SetBody(attribute.StringValue("sentinel-key"))
	provider.Logger("sentinel-scope-secret").Emit(context.Background(), record)
	require.Equal(t, 0, next.count())
	require.Equal(t, int64(0), gate.residentRecords.Load())
	require.Equal(t, 1, spy.outcome(metrics.AppLogExportOutcomeDroppedPrivacy))
}

// TestSDKEnvironmentResourceCannotBypassPrivacy verifies the SDK's implicit
// environment-resource merge cannot reintroduce secrets after source filtering.
func TestSDKEnvironmentResourceCannotBypassPrivacy(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "unknown_secret=sentinel-env-secret")
	spy := installLogExportSpy(t)
	next := &nopProcessor{}
	gate := newBoundedProcessor(next, 10, 4096)
	resource := privateResource(sdkresource.NewSchemaless(attribute.String("service.name", "one-api")))
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	var record otellog.Record
	record.SetBody(attribute.StringValue("application log"))
	provider.Logger(otelbridge.DefaultScopeName).Emit(context.Background(), record)
	require.Equal(t, 0, next.count())
	require.Equal(t, int64(0), gate.residentRecords.Load())
	require.Equal(t, 1, spy.outcome(metrics.AppLogExportOutcomeDroppedPrivacy))
}
