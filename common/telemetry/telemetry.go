package telemetry

import (
	"context"
	stdErrors "errors"
	"time"

	laerrors "github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
)

// ProviderBundle holds the tracer and meter providers so they can be shut down gracefully.
type ProviderBundle struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
}

// InitOpenTelemetry configures global OpenTelemetry providers when enabled.
// It returns a ProviderBundle for graceful shutdown. When OpenTelemetry is
// disabled, the function returns nil without error.
func InitOpenTelemetry(ctx context.Context) (*ProviderBundle, error) {
	if !config.OpenTelemetryEnabled {
		return nil, nil
	}

	if config.OpenTelemetryEndpoint == "" {
		return nil, laerrors.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_ENABLED is true")
	}

	res, err := buildResource(ctx)
	if err != nil {
		return nil, laerrors.Wrap(err, "build OpenTelemetry resource")
	}

	traceExporter, err := otlptracehttp.New(ctx, buildTraceExporterOptions()...)
	if err != nil {
		return nil, laerrors.Wrap(err, "create OTLP trace exporter")
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	metricExporter, err := otlpmetrichttp.New(ctx, buildMetricExporterOptions()...)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, laerrors.Wrap(err, "create OTLP metric exporter")
	}

	reader := sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Logger.Info("OpenTelemetry initialized",
		zap.String("endpoint", config.OpenTelemetryEndpoint),
		zap.Bool("insecure", config.OpenTelemetryInsecure),
		zap.String("service", config.OpenTelemetryServiceName),
		zap.String("environment", config.OpenTelemetryEnvironment),
	)

	return &ProviderBundle{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
	}, nil
}

// Shutdown drains telemetry providers, ensuring exporters flush pending data.
func (p *ProviderBundle) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	var errs []error

	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, laerrors.Wrap(err, "shutdown meter provider"))
		}
	}

	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, laerrors.Wrap(err, "shutdown tracer provider"))
		}
	}

	if len(errs) > 0 {
		return laerrors.Wrap(stdErrors.Join(errs...), "shutdown OpenTelemetry providers")
	}

	return nil
}

func buildResource(ctx context.Context) (*sdkresource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", config.OpenTelemetryServiceName),
		attribute.String("service.version", common.Version),
	}

	if config.OpenTelemetryEnvironment != "" {
		attrs = append(attrs, attribute.String("deployment.environment", config.OpenTelemetryEnvironment))
	}

	return sdkresource.New(ctx,
		sdkresource.WithFromEnv(),
		sdkresource.WithHost(),
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithProcess(),
		sdkresource.WithAttributes(attrs...),
	)
}

func buildTraceExporterOptions() []otlptracehttp.Option {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.OpenTelemetryEndpoint),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}

	if config.OpenTelemetryInsecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	return opts
}

func buildMetricExporterOptions() []otlpmetrichttp.Option {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(config.OpenTelemetryEndpoint),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
	}

	if config.OpenTelemetryInsecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	return opts
}
