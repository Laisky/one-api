package telemetry

import (
	"context"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
)

// fidelityLoggerProvider forwards all application scope/options. Its lifecycle
// methods remain those of the embedded real SDK provider.
type fidelityLoggerProvider struct{ *sdklog.LoggerProvider }

// Logger validates metadata without forcing a different name, version or schema.
func (p fidelityLoggerProvider) Logger(name string, options ...otellog.LoggerOption) otellog.Logger {
	cfg := otellog.NewLoggerConfig(options...)
	attrs := cfg.InstrumentationAttributes()
	valid := utf8.ValidString(name) && utf8.ValidString(cfg.InstrumentationVersion()) && utf8.ValidString(cfg.SchemaURL()) &&
		otelbridge.ValidateLogAttributes(attrs.ToSlice()) == nil
	return fidelityLogger{Logger: p.LoggerProvider.Logger(name, options...), validScope: valid}
}

// fidelityLogger checks the original public Record before SDK newRecord can
// discard invalid keys. It never rewrites fields or changes Enabled decisions.
type fidelityLogger struct {
	otellog.Logger
	validScope bool
}

// Emit rejects a complete invalid record with a fixed observable reason.
func (l fidelityLogger) Emit(ctx context.Context, r otellog.Record) {
	var attrs []attribute.KeyValue
	r.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	if !l.validScope || !utf8.ValidString(r.EventName()) || !utf8.ValidString(r.SeverityText()) ||
		otelbridge.ValidateLogValue(r.Body()) != nil || otelbridge.ValidateLogRecordAttributes(attrs) != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		otel.Handle(otelbridge.ErrLogEncoding)
		return
	}
	l.Logger.Emit(ctx, r)
}
