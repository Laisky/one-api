package otelbridge

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/zap"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// countingDiscardExporter discards records but counts them, so a benchmark can
// prove it measured an export rather than a rejection.
type countingDiscardExporter struct{ records atomic.Int64 }

// Export implements sdklog.Exporter.
func (e *countingDiscardExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.records.Add(int64(len(records)))
	return nil
}

// Shutdown implements sdklog.Exporter.
func (e *countingDiscardExporter) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter.
func (e *countingDiscardExporter) ForceFlush(context.Context) error { return nil }

// BenchmarkBridgeWriteRequestRecord measures one request-scoped log line as
// production writes it: the process logger's bound "host", the gin request
// logger's bound fields, and three call-site fields.
//
// Sub-benchmarks:
//   - provider "production" mirrors common/telemetry's provider options (SDK
//     deduplication off, no truncation); "sdk-dedup" turns SDK deduplication
//     on, the alternative to resolving collisions in the bridge.
//   - record "unique" has no repeated key; "host-collision" repeats "host"
//     exactly as production does.
//
// exported/op reports how many records reached the exporter per operation, so
// a run that silently measured a rejection shows 0 instead of looking fast. The
// simple processor exports synchronously, so no record is lost to queue overflow
// and exported/op is exact.
// docs/benchmarks/20261104_otlp-app-log-key-collisions.md records the results.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkBridgeWriteRequestRecord(b *testing.B) {
	providers := []struct {
		name string
		opts []sdklog.LoggerProviderOption
	}{
		{"production", []sdklog.LoggerProviderOption{sdklog.WithAllowKeyDuplication()}},
		{"sdk-dedup", nil},
	}
	records := []struct {
		name        string
		requestHost string
	}{
		{"unique", "http_host"},
		{"host-collision", "host"},
	}

	for _, p := range providers {
		for _, r := range records {
			b.Run(p.name+"/"+r.name, func(b *testing.B) {
				exporter := &countingDiscardExporter{}
				opts := append([]sdklog.LoggerProviderOption{
					sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
					sdklog.WithAttributeCountLimit(-1),
					sdklog.WithAttributeValueLengthLimit(-1),
				}, p.opts...)
				provider := sdklog.NewLoggerProvider(opts...)

				holder := NewProviderHolder()
				if err := holder.Install(provider); err != nil {
					b.Fatal(err)
				}
				lg := zap.New(NewCore(holder, DefaultScopeName, zap.InfoLevel)).
					With(zap.String("host", "b4c6097d77f6")).
					With(zap.String("url", "/v1/chat/completions"), zap.String("remote", "100.122.41.16:36950"),
						zap.String(r.requestHost, "oneapi.example.com"), zap.String("trace_id", "b0efda9bb9736dc6:e2fb85dddc61527c::4"),
						zap.String("cost", "0.01s"), zap.String("request_id", "2026110318550712345"))

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					lg.Info("select channel in cache", zap.Int("channel_id", 7), zap.String("model", "gpt-5"), zap.Int("user_id", 42))
				}
				b.StopTimer()

				if err := provider.Shutdown(context.Background()); err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(exporter.records.Load())/float64(b.N), "exported/op")
			})
		}
	}
}
