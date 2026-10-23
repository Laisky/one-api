package telemetry

import (
	"context"
	"unicode/utf8"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// fidelityLogProcessor performs no content rewrite, key allowlisting, metadata
// canonicalization or type conversion. It makes encoding/operator failures visible
// before bounded admission, preserving every valid field of an admitted record.
type fidelityLogProcessor struct {
	next          sdklog.Processor
	maxAttributes int
	maxValueBytes int
}

// Enabled preserves the original SDK filtering parameters.
func (p *fidelityLogProcessor) Enabled(ctx context.Context, param sdklog.EnabledParameters) bool {
	return p.next.Enabled(ctx, param)
}

// OnEmit validates complete records without partially removing fields.
func (p *fidelityLogProcessor) OnEmit(ctx context.Context, r *sdklog.Record) error {
	if r == nil {
		return nil
	}
	var attrs []attribute.KeyValue
	r.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	scope := r.InstrumentationScope()
	valid := otelbridge.ValidateLogTimestamp(r.Timestamp()) == nil && otelbridge.ValidateLogTimestamp(r.ObservedTimestamp()) == nil && utf8.ValidString(r.EventName()) && utf8.ValidString(r.SeverityText()) && utf8.ValidString(scope.Name) &&
		utf8.ValidString(scope.Version) && utf8.ValidString(scope.SchemaURL) &&
		otelbridge.ValidateLogValue(r.Body()) == nil && otelbridge.ValidateLogAttributes(attrs) == nil &&
		otelbridge.ValidateLogAttributes(scope.Attributes.ToSlice()) == nil
	if r.Resource() != nil {
		valid = valid && utf8.ValidString(r.Resource().SchemaURL()) && otelbridge.ValidateLogAttributes(r.Resource().Attributes()) == nil
	}
	if !valid {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		return otelbridge.ErrLogEncoding
	}
	if p.maxAttributes > 0 && len(attrs) > p.maxAttributes {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedOperatorLimit, 1)
		return nil
	}
	if p.maxValueBytes > 0 {
		for _, kv := range attrs {
			if exceedsValueBytes(kv.Value, p.maxValueBytes) {
				metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedOperatorLimit, 1)
				return nil
			}
		}
	}
	return p.next.OnEmit(ctx, r)
}

// exceedsValueBytes applies only an explicitly configured text/binary leaf
// limit, including nested attribute values; default zero imposes no content cap.
func exceedsValueBytes(v attribute.Value, limit int) bool {
	switch v.Type() {
	case attribute.STRING:
		return len(v.AsString()) > limit
	case attribute.BYTESLICE:
		return len(v.AsByteSlice()) > limit
	case attribute.STRINGSLICE:
		for _, s := range v.AsStringSlice() {
			if len(s) > limit {
				return true
			}
		}
	case attribute.SLICE:
		for _, item := range v.AsSlice() {
			if exceedsValueBytes(item, limit) {
				return true
			}
		}
	case attribute.MAP:
		for _, kv := range v.AsMap() {
			if exceedsValueBytes(kv.Value, limit) {
				return true
			}
		}
	}
	return false
}

// ForceFlush forwards the bounded drain.
func (p *fidelityLogProcessor) ForceFlush(ctx context.Context) error { return p.next.ForceFlush(ctx) }

// Shutdown forwards shutdown without rewriting pending records.
func (p *fidelityLogProcessor) Shutdown(ctx context.Context) error { return p.next.Shutdown(ctx) }
