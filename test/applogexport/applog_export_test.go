// Package applogexport_test is an end-to-end acceptance test for the OTLP
// application-log export.
//
// It wires the process exactly as main.go does -- logger.SetupEnhancedLogger,
// telemetry.InitOpenTelemetry and middleware.RequestLogger -- and asserts on
// what reaches a real OTLP/HTTP receiver. It lives in its own package because it
// installs process-wide OpenTelemetry providers and shuts the shared log bridge
// down, which must not leak into unrelated tests.
//
// It exists because the unit tests in common/logger/otelbridge and middleware
// built their loggers by hand and passed while production exported no
// request-scoped log at all: the hand-built loggers did not bind the process
// "host" field that collides with the request logger's own "host".
package applogexport_test

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/telemetry"
	"github.com/Laisky/one-api/middleware"
)

// exportedRecord is one log record as the OTLP receiver decoded it.
type exportedRecord struct {
	scope string
	body  string
	keys  []string
	attrs map[string]*commonpb.AnyValue
}

// otlpReceiver is a minimal OTLP/HTTP collector that keeps every log record.
type otlpReceiver struct {
	mu      sync.Mutex
	records []exportedRecord
}

// ServeHTTP decodes /v1/logs protobuf requests and acknowledges every signal.
//
// Parameters:
//   - w: the response writer.
//   - r: the export request.
//
// Return values: none.
func (rcv *otlpReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}()
	if r.URL.Path != "/v1/logs" {
		return
	}

	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return
		}
		defer gz.Close()
		body = gz
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return
	}
	var request collectorlogs.ExportLogsServiceRequest
	if proto.Unmarshal(raw, &request) != nil {
		return
	}

	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	for _, rl := range request.GetResourceLogs() {
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				rec := exportedRecord{
					scope: sl.GetScope().GetName(),
					body:  lr.GetBody().GetStringValue(),
					attrs: map[string]*commonpb.AnyValue{},
				}
				for _, kv := range lr.GetAttributes() {
					rec.keys = append(rec.keys, kv.GetKey())
					rec.attrs[kv.GetKey()] = kv.GetValue()
				}
				rcv.records = append(rcv.records, rec)
			}
		}
	}
}

// find returns the single record whose body contains fragment.
//
// Parameters:
//   - t: the test handle.
//   - fragment: text the record body must contain.
//
// Return values:
//   - exportedRecord: the matching record.
func (rcv *otlpReceiver) find(t *testing.T, fragment string) exportedRecord {
	t.Helper()

	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	var found []exportedRecord
	var bodies []string
	for _, rec := range rcv.records {
		bodies = append(bodies, rec.body)
		if strings.Contains(rec.body, fragment) {
			found = append(found, rec)
		}
	}
	require.Len(t, found, 1, "expected exactly one exported record containing %q; exported: %q", fragment, bodies)
	return found[0]
}

// startProductionLogging wires logging and OpenTelemetry the way main.go does,
// pointed at a local receiver, and restores every global it touched.
//
// Parameters:
//   - t: the test handle.
//
// Return values:
//   - *otlpReceiver: the receiver holding exported records.
//   - func(): flushes and shuts the pipeline down; call it before asserting.
func startProductionLogging(t *testing.T) (*otlpReceiver, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")

	receiver := &otlpReceiver{}
	server := httptest.NewServer(receiver)
	t.Cleanup(server.Close)

	previousConfig := struct {
		bridge, otelEnabled, insecure, debug bool
		endpoint, logPush                    string
	}{config.AppLogOTLPEnabled, config.OpenTelemetryEnabled, config.OpenTelemetryInsecure, config.DebugEnabled,
		config.OpenTelemetryEndpoint, config.LogPushAPI}
	previousLogger, previousHolder := logger.Logger, otelbridge.Shared
	previousTracer, previousMeter, previousPropagator := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	previousLogProvider := global.GetLoggerProvider()
	t.Cleanup(func() {
		config.AppLogOTLPEnabled, config.OpenTelemetryEnabled = previousConfig.bridge, previousConfig.otelEnabled
		config.OpenTelemetryInsecure, config.DebugEnabled = previousConfig.insecure, previousConfig.debug
		config.OpenTelemetryEndpoint, config.LogPushAPI = previousConfig.endpoint, previousConfig.logPush
		logger.Logger, otelbridge.Shared = previousLogger, previousHolder
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
		otel.SetTextMapPropagator(previousPropagator)
		global.SetLoggerProvider(previousLogProvider)
	})

	config.AppLogOTLPEnabled = true
	config.OpenTelemetryEnabled = true
	config.OpenTelemetryEndpoint = strings.TrimPrefix(server.URL, "http://")
	config.OpenTelemetryInsecure = true
	config.DebugEnabled = false
	config.LogPushAPI = ""
	// The bundle shutdown below closes the bridge holder for good, so this
	// test gets a fresh one rather than closing the process-wide default.
	otelbridge.Shared = otelbridge.NewProviderHolder()

	ctx := context.Background()
	logger.SetupEnhancedLogger(ctx)
	bundle, err := telemetry.InitOpenTelemetry(ctx)
	require.NoError(t, err)
	require.NotNil(t, bundle)

	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			require.NoError(t, bundle.Shutdown(shutdownCtx))
		})
	}
	t.Cleanup(shutdown)
	return receiver, shutdown
}

// TestRequestScopedLogsReachTheOTLPCollector reproduces the production report
// "only the periodic sync lines ever reach the log store" and pins its fix.
//
// Every log line written through the request logger used to be discarded by
// the bridge, because the process logger binds "host" (the container hostname)
// and gin-middlewares binds "host" again (the HTTP Host header). The record is
// now exported with the later value, as the OpenTelemetry specification
// prescribes for a key set twice.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRequestScopedLogsReachTheOTLPCollector(t *testing.T) {
	receiver, shutdown := startProductionLogging(t)

	engine := gin.New()
	engine.Use(middleware.RequestLogger())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		gmw.GetLogger(c).Info("select channel in cache", zap.Int("channel_id", 7))
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	req.Host = "oneapi.example.com"
	engine.ServeHTTP(httptest.NewRecorder(), req)

	shutdown()

	handler := receiver.find(t, "select channel in cache")
	require.Equal(t, "one-api.gin", handler.scope)
	require.Equal(t, 1, strings.Count(strings.Join(handler.keys, ","), "host"), "each key is exported once: %v", handler.keys)
	require.Equal(t, "oneapi.example.com", handler.attrs["host"].GetStringValue(),
		"the request logger's host is set after the process host, so it wins")
	require.Equal(t, "/v1/chat/completions", handler.attrs["url"].GetStringValue())
	require.EqualValues(t, 7, handler.attrs["channel_id"].GetIntValue())

	access := receiver.find(t, "POST")
	require.Equal(t, "one-api.gin", access.scope)
	require.Equal(t, "/v1/chat/completions", access.attrs["url"].GetStringValue())
}

// TestBackgroundContextLogsReachTheOTLPCollector reproduces the second leak:
// logger.FromContext on a context without a request logger returned
// gin-middlewares' stdout-only library logger, so background workers such as
// MCP catalog sync never reached the log store.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBackgroundContextLogsReachTheOTLPCollector(t *testing.T) {
	receiver, shutdown := startProductionLogging(t)

	logger.FromContext(context.Background()).Info("mcp auto sync succeeded", zap.Int("tools", 3))

	shutdown()

	rec := receiver.find(t, "mcp auto sync succeeded")
	require.Equal(t, "one-api", rec.scope)
	require.EqualValues(t, 3, rec.attrs["tools"].GetIntValue())
}

// TestCollisionFreeRecordsAreExportedUnchanged is the backward-compatibility
// control: a record that was exported before the fix must keep exactly the same
// attribute keys, in the same order, with the same values. It mirrors the
// periodic sync line that production stored all along.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCollisionFreeRecordsAreExportedUnchanged(t *testing.T) {
	receiver, shutdown := startProductionLogging(t)

	logger.Logger.Info("channels synced from database, considering suspensions")

	shutdown()

	hostname, err := os.Hostname()
	require.NoError(t, err)
	rec := receiver.find(t, "channels synced from database, considering suspensions")
	require.Equal(t, "one-api", rec.scope)
	require.Equal(t, []string{"host", "code.file.path", "code.line.number", "code.function.name"}, rec.keys)
	require.Equal(t, hostname, rec.attrs["host"].GetStringValue())
}
