package telemetry

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// TestOperationalLogProductionHTTPWire exports through the actual production
// privacy processor, bounded admission, SDK batch/counting processors and HTTP
// exporter to a local peer. The optional receipt contains synthetic records only.
func TestOperationalLogProductionHTTPWire(t *testing.T) {
	clearLogExporterTestEnvironment(t)
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	type capture struct {
		payload []byte
		err     error
	}
	received := make(chan capture, 2)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got capture
		if r.URL.Path != "/v1/logs" || r.Header.Get("Content-Encoding") != "gzip" ||
			r.Header.Get("Content-Type") != "application/x-protobuf" {
			got.err = fmt.Errorf("unexpected synthetic OTLP request shape")
		} else {
			gz, err := gzip.NewReader(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				got.err = err
			} else {
				got.payload, got.err = io.ReadAll(io.LimitReader(gz, 1<<20))
				if closeErr := gz.Close(); got.err == nil {
					got.err = closeErr
				}
			}
		}
		received <- got
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer peer.Close()
	endpoint, insecure := config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure
	config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = strings.TrimPrefix(peer.URL, "http://"), true
	t.Cleanup(func() { config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = endpoint, insecure })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := newLoggerProvider(ctx, sdkresource.NewSchemaless(attribute.String("service.name", "one-api-synthetic")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	lg := zap.New(otelbridge.NewPrivacyCore(holder, otelbridge.DefaultScopeName, zapcore.InfoLevel))
	const original = "worker shard 7 synchronized 3 caches"
	lg.With(zap.String("component", "synthetic-cache-worker")).Info(original,
		zap.Int("cache_count", 3), zap.Float64("ratio", 0.25), zap.Bool("ready", true),
		zap.String("prompt", "SYNTHETIC_PRIVATE_PROMPT"), zap.String("Authorization", "SYNTHETIC_PRIVATE_TOKEN"),
		zap.String("url", "/v1/responses?unknown=SYNTHETIC_PRIVATE_QUERY"),
		zap.String("diagnostic", "dial tcp cache.internal:6379: connection refused"))
	native := provider.Logger(otelbridge.DefaultScopeName)
	for _, event := range []string{"cache.refresh.completed", "cache.refresh token=SYNTHETIC_PRIVATE_EVENT"} {
		var record otellog.Record
		record.SetBody(attribute.StringValue("native worker completed"))
		record.SetEventName(event)
		record.AddAttributes(attribute.String("event.name", "attribute-only"))
		native.Emit(ctx, record)
	}
	require.NoError(t, provider.ForceFlush(ctx))
	var got capture
	select {
	case got = <-received:
	case <-ctx.Done():
		t.Fatal("production HTTP exporter did not reach the local peer")
	}
	require.NoError(t, got.err)
	var request collectorlogs.ExportLogsServiceRequest
	require.NoError(t, proto.Unmarshal(got.payload, &request))
	require.Len(t, request.ResourceLogs, 1)
	require.Len(t, request.ResourceLogs[0].ScopeLogs, 1)
	records := request.ResourceLogs[0].ScopeLogs[0].LogRecords
	require.Len(t, records, 3)
	require.Equal(t, "cache.refresh.completed", records[1].EventName)
	require.Equal(t, "cache.refresh token=[REDACTED]", records[2].EventName)
	require.Equal(t, "native worker completed", records[1].Body.GetStringValue())
	require.Equal(t, "native worker completed", records[2].Body.GetStringValue())
	require.Equal(t, original, records[0].Body.GetStringValue())
	attrs := map[string]*commonpb.AnyValue{}
	for _, kv := range records[0].Attributes {
		attrs[kv.Key] = kv.Value
	}
	require.Equal(t, "synthetic-cache-worker", attrs["component"].GetStringValue())
	require.IsType(t, &commonpb.AnyValue_IntValue{}, attrs["cache_count"].Value)
	require.EqualValues(t, 3, attrs["cache_count"].GetIntValue())
	require.IsType(t, &commonpb.AnyValue_DoubleValue{}, attrs["ratio"].Value)
	require.Equal(t, 0.25, attrs["ratio"].GetDoubleValue())
	require.IsType(t, &commonpb.AnyValue_BoolValue{}, attrs["ready"].Value)
	require.True(t, attrs["ready"].GetBoolValue())
	require.Equal(t, "/v1/responses", attrs["url"].GetStringValue())
	require.Equal(t, "dial tcp cache.internal:6379: connection refused", attrs["diagnostic"].GetStringValue())
	encoded, err := protojson.MarshalOptions{Indent: "  "}.Marshal(&request)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "SYNTHETIC_PRIVATE")
	if dir := os.Getenv("ONEAPI_TEST_SYNTHETIC_OTLP_RECEIPT_DIR"); dir != "" {
		// An explicit opt-in writes fixed filenames only below the system temp root.
		relative, relErr := filepath.Rel(os.TempDir(), filepath.Clean(dir))
		require.NoError(t, relErr)
		require.NotEqual(t, ".", relative)
		require.False(t, filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
		require.NoError(t, os.MkdirAll(dir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "oneapi-operational-logs.pb"), got.payload, 0600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "oneapi-operational-logs.json"), encoded, 0600))
	}
}
