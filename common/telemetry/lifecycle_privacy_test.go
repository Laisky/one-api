package telemetry

import (
	"context"
	"testing"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// TestLifecycleDiagnosticsReachPrivateQueue checks the explicitly wired legacy
// privacy components, not the configured APP_LOG_SINK=otlp fidelity pipeline.
// It accepts t and returns no value; arbitrary strings must not enter this queue.
func TestLifecycleDiagnosticsReachPrivateQueue(t *testing.T) {
	capture := &captureProcessor{}
	gate := newBoundedProcessor(capture, 10, 4096)
	resource := privateResource(sdkresource.NewSchemaless(attribute.String("service.name", "one-api")))
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource), sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	lg := zap.New(otelbridge.NewPrivacyCore(holder, otelbridge.DefaultScopeName, zapcore.InfoLevel))
	lg.Warn("sentinel-message-secret",
		zap.String("log_stage", "transport_attempt"),
		zap.String("transport_failure_class", "connection_reset"),
		zap.Bool("response_headers_received", false),
		zap.Int("transport_got_conn_count", 1),
		zap.String("request_id", "sentinel-identity-secret"),
		zap.String("retry_outcome", "sentinel-freeform-secret"),
		zap.String("request_body", "sentinel-payload-secret"))
	capture.mu.Lock()
	got := capture.last.Clone()
	capture.mu.Unlock()
	require.Equal(t, "application log", got.Body().AsString())
	var attrs []attribute.KeyValue
	got.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	require.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("log_stage", "transport_attempt"),
		attribute.String("transport_failure_class", "connection_reset"),
		attribute.Bool("response_headers_received", false),
		attribute.Int("transport_got_conn_count", 1),
	}, attrs)
	require.Equal(t, int64(1), gate.residentRecords.Load())
}
