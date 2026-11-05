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

// Emit rejects a complete invalid record with a fixed observable reason, and
// resolves repeated attribute or body-map keys last-value-wins exactly as the
// Zap bridge does, so both paths into the provider follow one rule.
func (l fidelityLogger) Emit(ctx context.Context, r otellog.Record) {
	var attrs []attribute.KeyValue
	r.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	attrs, attrsDeduplicated, attrsErr := otelbridge.NormalizeLogRecordAttributes(attrs)
	body, bodyDeduplicated, bodyErr := otelbridge.NormalizeLogValue(r.Body())
	if !l.validScope || !utf8.ValidString(r.EventName()) || !utf8.ValidString(r.SeverityText()) ||
		bodyErr != nil || attrsErr != nil ||
		otelbridge.ValidateLogTimestamp(r.Timestamp()) != nil || otelbridge.ValidateLogTimestamp(r.ObservedTimestamp()) != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		otel.Handle(otelbridge.ErrLogEncoding)
		return
	}
	deduplicated := attrsDeduplicated || bodyDeduplicated
	if deduplicated {
		r = withNormalizedContent(r, body, attrs)
	}
	prepared, err := resolveNativeLogError(r, attrs)
	if err != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		otel.Handle(otelbridge.ErrLogEncoding)
		return
	}
	l.Logger.Emit(ctx, prepared)
	if deduplicated {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDeduplicated, 1)
	}
}

// withNormalizedContent rebuilds r with a normalized body and attribute list.
//
// The log API can append attributes but never remove one, so a record whose
// keys were deduplicated has to be copied field by field.
//
// Parameters:
//   - r: the original record.
//   - body: the body to use instead of r's.
//   - attrs: the attributes to use instead of r's.
//
// Return values:
//   - otellog.Record: a copy of r carrying body and attrs.
func withNormalizedContent(r otellog.Record, body attribute.Value, attrs []attribute.KeyValue) otellog.Record {
	var out otellog.Record
	out.SetEventName(r.EventName())
	out.SetTimestamp(r.Timestamp())
	out.SetObservedTimestamp(r.ObservedTimestamp())
	out.SetSeverity(r.Severity())
	out.SetSeverityText(r.SeverityText())
	out.SetBody(body)
	out.SetErr(r.Err())
	out.AddAttributes(attrs...)
	return out
}
