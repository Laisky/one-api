package telemetry

// Optional OTLP application-log provider (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2).
//
// This provider is built ONLY when APP_LOG_SINK names the additive otlp sink.
// A deployment that does not ask for it gets no log exporter, no batch worker,
// and no new goroutine, which is what section 2.1's unchanged-configuration
// contract requires.
//
// Module pinning is part of the G3 gate ("pinned SDK/exporters"). The versions
// this is written against are go.opentelemetry.io/otel v1.46.0 with
// otel/log, otel/sdk/log and otlploghttp at v0.22.0. Two facts about that
// choice matter enough to record here rather than in a commit message:
//
//   - The v0.20.0 log line, which is the one that pairs with otel core v1.44.0
//     (this repository's previous pin), has a BatchProcessor that busy-spins
//     under exporter backpressure and a ForceFlush/Shutdown that cannot report
//     drain errors. Both were fixed in v0.21.0. Shipping application logs on
//     the v0.20.0 processor would have meant a collector outage burning CPU on
//     the gateway, so the core bump to v1.46.0 is a prerequisite, not a tidy-up.
//   - The Logs API and SDK reached release-candidate status on 2026-08-31 and
//     the EXPORTERS were explicitly excluded from that RC's stability scope.
//     The pipeline therefore must not be described as stable; otlploghttp is
//     expected to stay on a v0.x line. See
//     https://opentelemetry.io/blog/2026/go-logs-api-sdk-rc/.

import (
	"context"
	"time"

	laerrors "github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// newLoggerProvider builds the bounded OTLP application-log provider.
//
// Parameters:
//   - ctx: bounds exporter construction.
//   - res: the resource attached to every record; nil is allowed.
//
// Return values:
//   - *sdklog.LoggerProvider: the configured provider.
//   - error: a wrapped failure when the exporter cannot be created.
func newLoggerProvider(ctx context.Context, res *sdkresource.Resource) (*sdklog.LoggerProvider, error) {
	exporter, err := otlploghttp.New(ctx, buildLogExporterOptions()...)
	if err != nil {
		return nil, laerrors.Wrap(err, "create OTLP log exporter")
	}

	queueRecords := int64(config.AppLogOTLPQueueSize)
	queueBytes := config.AppLogOTLPQueueMaxBytes()

	// The gate is declared before the batch processor so the exporter wrapper
	// can release residency the gate accounted for.
	gate := &boundedProcessor{maxRecords: queueRecords, maxBytes: queueBytes}
	counting := newCountingExporter(exporter, gate.release)

	batch := sdklog.NewBatchProcessor(counting,
		// Matching the gate's ceiling is deliberate: the SDK ring drops the
		// OLDEST record on overflow and reports it only through its internal
		// logger, so the gate must be the binding constraint for every drop to
		// be counted.
		sdklog.WithMaxQueueSize(config.AppLogOTLPQueueSize),
		sdklog.WithExportMaxBatchSize(config.AppLogOTLPBatchSize),
		sdklog.WithExportInterval(time.Duration(config.AppLogOTLPExportIntervalMs)*time.Millisecond),
		sdklog.WithExportTimeout(time.Duration(config.AppLogOTLPExportTimeoutMs)*time.Millisecond),
	)
	gate.next = batch

	opts := []sdklog.LoggerProviderOption{
		sdklog.WithProcessor(gate),
		sdklog.WithAttributeCountLimit(config.AppLogOTLPMaxAttributes),
		sdklog.WithAttributeValueLengthLimit(config.AppLogOTLPMaxAttributeValueBytes),
	}
	if res != nil {
		opts = append(opts, sdklog.WithResource(res))
	}

	return sdklog.NewLoggerProvider(opts...), nil
}

// installLoggerProvider publishes a logger provider to the application log
// bridge and to the OpenTelemetry global, so the bridge begins exporting.
//
// Parameters:
//   - provider: the provider to publish; nil is a no-op.
//
// Return values:
//   - error: when the bridge refuses the provider, which happens only after a
//     shutdown has already closed it.
func installLoggerProvider(provider *sdklog.LoggerProvider) error {
	if provider == nil {
		return nil
	}
	// The global is set as well as the bridge holder: an out-of-tree library
	// that emits through go.opentelemetry.io/otel/log/global then reaches the
	// same bounded pipeline instead of a silent no-op.
	global.SetLoggerProvider(provider)
	return laerrors.Wrap(otelbridge.Shared.Install(provider), "install app log bridge provider")
}

// buildLogExporterOptions assembles the OTLP log exporter options.
//
// It reuses OTEL_EXPORTER_OTLP_ENDPOINT and OTEL_EXPORTER_OTLP_INSECURE rather
// than introducing log-specific endpoint settings, so one collector address
// configures traces, metrics and logs together and cannot drift between them.
//
// Parameters: none.
//
// Return values:
//   - []otlploghttp.Option: the configured options.
func buildLogExporterOptions() []otlploghttp.Option {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(config.OpenTelemetryEndpoint),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
	}

	if config.OpenTelemetryInsecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	return opts
}
