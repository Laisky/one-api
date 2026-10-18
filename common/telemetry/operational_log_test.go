package telemetry

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"

	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// operationalLogExporter captures records emitted by the real SDK exporter path.
type operationalLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

// Export retains independent records without making network requests.
func (e *operationalLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

// Shutdown implements the local exporter's no-op cleanup.
func (*operationalLogExporter) Shutdown(context.Context) error { return nil }

// ForceFlush implements the local exporter's no-op flush.
func (*operationalLogExporter) ForceFlush(context.Context) error { return nil }

// TestOperationalLogContentSurvivesProductionPrivacy exercises the complete
// Zap-to-SDK-to-exporter path using source-authored background-worker messages.
func TestOperationalLogContentSurvivesProductionPrivacy(t *testing.T) {
	exporter := &operationalLogExporter{}
	resource := privateResource(sdkresource.NewSchemaless(attribute.String("service.name", "one-api")))
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: sdklog.NewSimpleProcessor(exporter), resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	local, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(zapcore.NewTee(local, otelbridge.NewPrivacyCore(holder, otelbridge.DefaultScopeName, zapcore.DebugLevel)))
	messages := []string{
		"channels synced from database, considering suspensions",
		"syncing channels from database",
		"syncing options from database",
		"channel cache sync failed",
		"worker shard 7 completed 13 checks in 0.25 seconds",
	}
	for _, message := range messages {
		logger.With(zap.String("component", "cache-worker"), zap.Int("sync_frequency", 30)).
			Info(message, zap.String("prompt", "sentinel-private-prompt"),
				zap.String("Authorization", "sentinel-private-credential"),
				zap.Error(errors.New("dial failed: password=sentinel-private-error")),
				zap.Int("content_length", 123), zap.Float64("ratio", 0.25), zap.Bool("ready", true),
				zap.String("model", "deepseek-flash"), zap.Int("prompt_tokens", 17))
	}
	require.NoError(t, provider.ForceFlush(context.Background()))
	exporter.mu.Lock()
	records := append([]sdklog.Record(nil), exporter.records...)
	exporter.mu.Unlock()
	require.Len(t, records, len(messages))
	for i, record := range records {
		require.Equal(t, messages[i], record.Body().AsString())
		attrs := map[string]attribute.Value{}
		record.WalkAttributes(func(kv attribute.KeyValue) bool {
			attrs[string(kv.Key)] = kv.Value
			if kv.Value.Type() == attribute.STRING {
				require.NotContains(t, kv.Value.AsString(), "sentinel-private")
			}
			return true
		})
		require.Equal(t, attribute.StringValue("cache-worker"), attrs["component"])
		require.Equal(t, attribute.Int64Value(30), attrs["sync_frequency"])
		require.Equal(t, attribute.Int64Value(123), attrs["content_length"])
		require.Equal(t, attribute.Float64Value(0.25), attrs["ratio"])
		require.Equal(t, attribute.BoolValue(true), attrs["ready"])
		require.Equal(t, attribute.StringValue("deepseek-flash"), attrs["model"])
		require.Equal(t, attribute.Int64Value(17), attrs["prompt_tokens"])
		require.Equal(t, "dial failed: password=[REDACTED]", attrs["error"].AsString())
	}
	// The original local sink still receives full diagnostic fields.
	require.Len(t, observed.All(), len(messages))
	for i, entry := range observed.All() {
		require.Equal(t, messages[i], entry.Message)
	}
	require.Equal(t, "sentinel-private-prompt", observed.All()[0].ContextMap()["prompt"])
}
