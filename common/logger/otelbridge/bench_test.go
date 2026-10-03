package otelbridge

// Cost of the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2, gate
// G3's resource requirement).
//
// Two costs matter and they are measured separately, because they are paid by
// different deployments:
//
//   - The DISABLED cost, which every deployment pays. It must be zero: with
//     APP_LOG_SINK unchanged no bridge core is installed at all, so the only
//     honest measurement is of the local core on its own, as the baseline the
//     enabled arms are compared against.
//   - The ENABLED cost, paid only by a deployment that asked for the bridge.
//
// The exporter is a no-op in these benchmarks on purpose: transport latency
// belongs to the collector and would swamp the conversion cost this measures.

import (
	"context"
	"io"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// discardExporter accepts and drops every batch.
type discardExporter struct{}

// Export implements sdklog.Exporter by discarding the batch.
//
// Parameters:
//   - ctx: unused.
//   - records: discarded.
//
// Return values:
//   - error: always nil.
func (discardExporter) Export(context.Context, []sdklog.Record) error { return nil }

// Shutdown implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (discardExporter) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (discardExporter) ForceFlush(context.Context) error { return nil }

// discardCore is a zapcore.Core that accepts and drops every entry, standing in
// for the local file/stdout branch without paying for encoding or IO.
type discardCore struct {
	zapcore.LevelEnabler
}

// With implements zapcore.Core by returning the receiver.
//
// Parameters:
//   - fields: ignored.
//
// Return values:
//   - zapcore.Core: the receiver.
func (c discardCore) With([]zapcore.Field) zapcore.Core { return c }

// Check implements zapcore.Core by always accepting the entry.
//
// Parameters:
//   - ent: the entry.
//   - ce: the checked entry to add to.
//
// Return values:
//   - *zapcore.CheckedEntry: ce with this core added.
func (c discardCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

// Write implements zapcore.Core by discarding the entry.
//
// Parameters:
//   - ent, fields: ignored.
//
// Return values:
//   - error: always nil.
func (discardCore) Write(zapcore.Entry, []zapcore.Field) error { return nil }

// Sync implements zapcore.Core as a no-op.
//
// Parameters: none.
//
// Return values:
//   - error: always nil.
func (discardCore) Sync() error { return nil }

// Fields implements the fork's zapcore.Core by reporting no bound fields.
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: always nil.
func (discardCore) Fields() []zapcore.Field { return nil }

// BenchmarkBridgeWrite compares a log line's cost with the bridge absent,
// present, and present with trace correlation bound.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkBridgeWrite(b *testing.B) {
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(discardExporter{})),
	)
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	holder := NewProviderHolder()
	if err := holder.Install(provider); err != nil {
		b.Fatal(err)
	}

	// Two baselines, because the relative cost of the bridge is meaningless
	// without saying what it is relative TO. `discardCore` isolates the bridge
	// by removing encoding and IO entirely; `encodingCore` is a real JSON
	// encoder writing to io.Discard, which is what the file sink actually costs
	// minus the syscall, and is the fair comparison for "how much does enabling
	// the bridge add to a log line".
	local := discardCore{LevelEnabler: zapcore.DebugLevel}
	encoding := zapcore.NewCore(
		zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			MessageKey:     "msg",
			LevelKey:       "level",
			TimeKey:        "ts",
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.NanosDurationEncoder,
		}),
		zapcore.AddSync(io.Discard),
		zapcore.DebugLevel,
	)
	bridged := zapcore.NewTee(local, NewCore(holder, DefaultScopeName, zapcore.DebugLevel))
	bridgedEncoding := zapcore.NewTee(encoding, NewCore(holder, DefaultScopeName, zapcore.DebugLevel))

	fields := []zap.Field{
		zap.String("model", "gpt-5"),
		zap.Int("channel_id", 42),
		zap.String("request_id", "0123456789abcdef"),
	}

	b.Run("bridge_disabled", func(b *testing.B) {
		logger := zap.New(local)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			logger.Info("relay finished", fields...)
		}
	})

	b.Run("bridge_enabled", func(b *testing.B) {
		logger := zap.New(bridged)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			logger.Info("relay finished", fields...)
		}
	})

	b.Run("encoding_local_only", func(b *testing.B) {
		logger := zap.New(encoding)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			logger.Info("relay finished", fields...)
		}
	})

	b.Run("encoding_local_plus_bridge", func(b *testing.B) {
		logger := zap.New(bridgedEncoding)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			logger.Info("relay finished", fields...)
		}
	})

	b.Run("bridge_enabled_correlated", func(b *testing.B) {
		logger := zap.New(bridged).With(SpanContextFieldFrom(benchSpanContext()))
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			logger.Info("relay finished", fields...)
		}
	})
}

// benchSpanContext returns a valid, sampled span context for the correlated arm.
//
// Parameters: none.
//
// Return values:
//   - oteltrace.SpanContext: a sampled span context.
func benchSpanContext() oteltrace.SpanContext {
	return oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6},
		SpanID:     oteltrace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: oteltrace.FlagsSampled,
	})
}
