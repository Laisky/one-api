package telemetry

import (
	"compress/gzip"
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// TestEdgeResourcePrivacyAndIdentity checks automatic resource detection with
// synthetic command-line secrets and an operator-supplied stable host identity.
// Parameters: t owns environment and argument restoration. It returns no value.
func TestEdgeResourcePrivacyAndIdentity(t *testing.T) {
	previous := os.Args
	os.Args = []string{"one-api", "--token=synthetic-secret-not-for-export"}
	t.Cleanup(func() { os.Args = previous })
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "host.name=b1,operator.test=retained")
	res, err := buildResource(context.Background())
	require.NoError(t, err)
	attrs := map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	require.NotContains(t, attrs, "process.command_args")
	require.NotContains(t, attrs, "process.owner")
	require.Equal(t, "b1", attrs["host.name"])
	require.Equal(t, "retained", attrs["operator.test"])
	if config.OpenTelemetryEnvironment != "" {
		require.Equal(t, config.OpenTelemetryEnvironment, attrs["deployment.environment.name"])
		require.Equal(t, config.OpenTelemetryEnvironment, attrs["deployment.environment"])
	}
}

// TestEdgeTLSAllSignals exports real SDK payloads through the production option
// builders. It proves private-CA verification, bearer headers, protobuf and gzip
// for logs, metrics and traces; a responding empty HTTP server is insufficient.
// Parameters: t owns isolated exporters, the TLS peer and environment. No return.
func TestEdgeTLSAllSignals(t *testing.T) {
	const token = "synthetic-edge-token"
	var mu sync.Mutex
	seen := map[string]int{}
	var peerErrors []string
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail := func(message string) {
			mu.Lock()
			peerErrors = append(peerErrors, message)
			mu.Unlock()
			http.Error(w, "invalid test export", http.StatusBadRequest)
		}
		if r.TLS == nil || r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Content-Type") != "application/x-protobuf" || r.Header.Get("Content-Encoding") != "gzip" {
			fail("missing TLS, authentication, protobuf or gzip")
			return
		}
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			fail("invalid gzip")
			return
		}
		payload, err := io.ReadAll(io.LimitReader(gz, 1<<20))
		closeErr := gz.Close()
		if err != nil || closeErr != nil {
			fail("cannot read test payload")
			return
		}
		count := 0
		switch r.URL.Path {
		case "/v1/logs":
			var request collectorlogs.ExportLogsServiceRequest
			if err = proto.Unmarshal(payload, &request); err == nil {
				for _, resource := range request.ResourceLogs {
					for _, scope := range resource.ScopeLogs {
						count += len(scope.LogRecords)
					}
				}
			}
		case "/v1/metrics":
			var request collectormetrics.ExportMetricsServiceRequest
			if err = proto.Unmarshal(payload, &request); err == nil {
				for _, resource := range request.ResourceMetrics {
					for _, scope := range resource.ScopeMetrics {
						count += len(scope.Metrics)
					}
				}
			}
		case "/v1/traces":
			var request collectortrace.ExportTraceServiceRequest
			if err = proto.Unmarshal(payload, &request); err == nil {
				for _, resource := range request.ResourceSpans {
					for _, scope := range resource.ScopeSpans {
						count += len(scope.Spans)
					}
				}
			}
		default:
			fail("wrong signal path")
			return
		}
		if err != nil || count != 1 {
			fail("missing or invalid telemetry item")
			return
		}
		mu.Lock()
		seen[r.URL.Path] += count
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer peer.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.Certificate().Raw}), 0600))
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", ca)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization="+url.QueryEscape("Bearer "+token))
	for _, signal := range []string{"LOGS", "METRICS", "TRACES"} {
		for _, suffix := range []string{"HEADERS", "CERTIFICATE", "CLIENT_CERTIFICATE", "CLIENT_KEY", "ENDPOINT", "INSECURE"} {
			t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_"+suffix, "")
		}
	}
	endpoint, insecure := config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure
	config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = strings.TrimPrefix(peer.URL, "https://"), false
	t.Cleanup(func() { config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = endpoint, insecure })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := buildResource(ctx)
	require.NoError(t, err)
	now := time.Now()
	logs, err := otlploghttp.New(ctx, buildLogExporterOptions()...)
	require.NoError(t, err)
	var record sdklog.Record
	record.SetTimestamp(now)
	record.SetBody(log.StringValue("edge-canary"))
	require.NoError(t, logs.Export(ctx, []sdklog.Record{record}))
	require.NoError(t, logs.Shutdown(ctx))
	metrics, err := otlpmetrichttp.New(ctx, buildMetricExporterOptions()...)
	require.NoError(t, err)
	require.NoError(t, metrics.Export(ctx, &metricdata.ResourceMetrics{Resource: res, ScopeMetrics: []metricdata.ScopeMetrics{{Metrics: []metricdata.Metrics{{Name: "edge_canary", Data: metricdata.Gauge[int64]{DataPoints: []metricdata.DataPoint[int64]{{Time: now, Value: 1}}}}}}}}))
	require.NoError(t, metrics.Shutdown(ctx))
	traces, err := otlptracehttp.New(ctx, buildTraceExporterOptions()...)
	require.NoError(t, err)
	span := tracetest.SpanStub{Name: "edge-canary", Resource: res, StartTime: now.Add(-time.Second), EndTime: now,
		SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}}), Attributes: []attribute.KeyValue{attribute.String("test", "edge")}}
	require.NoError(t, traces.ExportSpans(ctx, []sdktrace.ReadOnlySpan{span.Snapshot()}))
	require.NoError(t, traces.Shutdown(ctx))
	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, peerErrors)
	require.Equal(t, map[string]int{"/v1/logs": 1, "/v1/metrics": 1, "/v1/traces": 1}, seen)
}
