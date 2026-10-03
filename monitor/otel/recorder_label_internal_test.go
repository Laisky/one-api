package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestHTTPMetricsWithNonUTF8PathStillExport reproduces the silent failure mode
// behind the Prometheus panic: OTLP is protobuf, proto.Marshal rejects any
// string field that is not valid UTF-8, and metric aggregators are cumulative.
// One request path such as "/\xc0" therefore poisons the attribute set forever
// and EVERY later metric export fails until the process restarts.
func TestHTTPMetricsWithNonUTF8PathStillExport(t *testing.T) {
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(strings.TrimPrefix(srv.URL, "http://")),
		otlpmetrichttp.WithInsecure(),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}),
	)
	require.NoError(t, err)

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Hour))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })

	recorder, err := NewOtelRecorder()
	require.NoError(t, err)

	recorder.RecordHTTPActiveRequest("/\xc0", "GET", 1)
	recorder.RecordHTTPRequest(time.Now(), "/\xc0", "GET", "200")
	recorder.RecordHTTPActiveRequest("/\xc0", "GET", -1)

	require.NoError(t, provider.ForceFlush(ctx), "metric export must survive a non-UTF-8 request path")
	require.Equal(t, int64(1), received.Load(), "collector must receive the batch")

	// A later cycle must still export: cumulative aggregators keep the attribute set.
	recorder.RecordHTTPRequest(time.Now(), "/healthy", "GET", "200")
	require.NoError(t, provider.ForceFlush(ctx))
	require.Equal(t, int64(2), received.Load())
}

// TestHTTPMetricsRecordSanitizedPathAttribute verifies the recorded attribute
// value is the U+FFFD-sanitized path, so the request is still counted and
// every string attribute in the export is valid UTF-8.
func TestHTTPMetricsRecordSanitizedPathAttribute(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	recorder, err := NewOtelRecorder()
	require.NoError(t, err)
	recorder.RecordHTTPRequest(time.Now(), "/\xc0", "GET", "200")
	recorder.RecordRelayRequest(time.Now(), 1, "openai", "model\xff", "1", "grp\xfe", "1", "openai", "chat", true, 1, 1, 1)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var sawSanitizedPath, sawSanitizedModel bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			for _, kv := range attributeSetsOf(m) {
				require.True(t, utf8.ValidString(kv.Value.AsString()), "metric %s attribute %s", m.Name, kv.Key)
				if m.Name == "one_api_http_requests_total" && kv.Key == "path" && kv.Value.AsString() == "/�" {
					sawSanitizedPath = true
				}
				if m.Name == "one_api_relay_requests_total" && kv.Key == "model" && kv.Value.AsString() == "model�" {
					sawSanitizedModel = true
				}
			}
		}
	}
	require.True(t, sawSanitizedPath, "http counter must carry the sanitized path attribute")
	require.True(t, sawSanitizedModel, "relay counter must carry the sanitized model attribute")
}

// attributeSetsOf flattens every string attribute of every data point in m.
func attributeSetsOf(m metricdata.Metrics) []attribute.KeyValue {
	var out []attribute.KeyValue
	collect := func(set attribute.Set) {
		for _, kv := range set.ToSlice() {
			if kv.Value.Type() == attribute.STRING {
				out = append(out, kv)
			}
		}
	}
	switch data := m.Data.(type) {
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			collect(dp.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, dp := range data.DataPoints {
			collect(dp.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, dp := range data.DataPoints {
			collect(dp.Attributes)
		}
	case metricdata.Gauge[int64]:
		for _, dp := range data.DataPoints {
			collect(dp.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, dp := range data.DataPoints {
			collect(dp.Attributes)
		}
	}
	return out
}
