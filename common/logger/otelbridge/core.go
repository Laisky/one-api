package otelbridge

// zapcore.Core implementation for the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2).
//
// W3.2 opens with a compile constraint, not a design preference: "The upstream
// otelzap bridge uses go.uber.org/zap/zapcore; this repository uses the Laisky
// fork. Compile a minimal adapter against the actual module graph before
// designing configuration around it." That is settled here. The fork is a
// separate Go module, so its zapcore.Field and zapcore.Core are distinct types
// from upstream's, and the fork's Core interface additionally requires a
// Fields() method that go.uber.org/zap has no notion of. No amount of
// configuration makes go.opentelemetry.io/contrib/bridges/otelzap satisfy it.
//
// The record mapping is deliberately identical to the upstream bridge (see
// convert.go and encoder.go) so that a collector pipeline, dashboard, or query
// written against otelzap works unchanged against this one.
//
// Two things here are NOT copied from upstream, both because upstream's choice
// is wrong for a gateway:
//
//  1. Sync() force-flushes the provider. Upstream hard-codes `return nil` and
//     closed the issue asking for more as not-planned, which means a zap
//     Fatal (which calls Sync then os.Exit) loses everything still queued.
//  2. A trace correlation path that does not retain a context. Upstream latches
//     any field whose Interface holds a context.Context. That works, and is
//     still supported here for compatibility, but this repository deliberately
//     keeps request loggers free of the gin context (they are snapshotted by
//     value into detached billing goroutines), so binding a request context
//     into a long-lived logger would retain the whole request. SpanContextField
//     carries the 24 bytes that actually matter instead.

import (
	"context"
	"slices"

	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/metrics"
)

// DefaultScopeName is the instrumentation scope reported for records that do
// not come from a named zap logger.
const DefaultScopeName = "github.com/Laisky/one-api/common/logger"

// Core is a zapcore.Core that converts entries into OpenTelemetry log records
// and emits them through an installed provider.
//
// A Core never blocks the caller on exporter I/O, never returns an error to
// zap, and never logs: it is the component that carries logs, so a failure it
// reports through the logger would be able to feed itself. Every loss is
// counted through common/metrics instead.
type Core struct {
	zapcore.LevelEnabler

	holder *ProviderHolder
	scope  string

	// attrs are the attributes accumulated by With, already converted.
	attrs []attribute.KeyValue
	// spanCtx is the correlation latched by With from a SpanContextField.
	spanCtx oteltrace.SpanContext
	// emitCtx is the context latched by With from a context-bearing field. It
	// is only set when a caller uses the upstream otelzap idiom.
	emitCtx context.Context
	// err is the error latched by With from a zap.Error field.
	err error
}

// Compile-time proof that the adapter satisfies the FORK's Core interface,
// including the Fields() method upstream otelzap does not implement. This
// assertion is the whole point of W3.2's "compile a minimal adapter against the
// actual module graph" instruction; if the fork's interface changes, the build
// breaks here rather than at a call site.
var _ zapcore.Core = (*Core)(nil)

// NewCore returns a zapcore.Core that emits through holder.
//
// The core is usable immediately even though holder may still be empty: records
// written before a provider is installed are counted as dropped_not_ready and
// continue to reach the file and stdout sinks unaffected.
//
// Parameters:
//   - holder: the provider holder to emit through; nil yields a core that drops
//     and counts every record, which keeps a misconfigured caller from
//     panicking inside the logger.
//   - scope: the default instrumentation scope name; empty uses DefaultScopeName.
//   - enab: the minimum level this core exports, independent of the levels the
//     file and stdout sinks accept.
//
// Return values:
//   - *Core: the configured core.
func NewCore(holder *ProviderHolder, scope string, enab zapcore.LevelEnabler) *Core {
	if holder == nil {
		holder = NewProviderHolder()
	}
	if scope == "" {
		scope = DefaultScopeName
	}
	if enab == nil {
		enab = zapcore.DebugLevel
	}
	return &Core{LevelEnabler: enab, holder: holder, scope: scope}
}

// With implements zapcore.Core by returning a copy carrying the extra fields.
//
// Parameters:
//   - fields: the fields to bind to the returned core.
//
// Return values:
//   - zapcore.Core: a clone with the fields converted and latched.
func (c *Core) With(fields []zapcore.Field) zapcore.Core {
	cloned := c.clone()
	if len(fields) == 0 {
		return cloned
	}

	converted := convertFields(fields)
	cloned.attrs = append(cloned.attrs, converted.attrs...)
	if converted.spanCtx.IsValid() {
		cloned.spanCtx = converted.spanCtx
	}
	if converted.ctx != nil {
		cloned.emitCtx = converted.ctx
	}
	if converted.err != nil {
		cloned.err = converted.err
	}
	return cloned
}

// clone returns an independent copy of the core.
//
// Parameters: none.
//
// Return values:
//   - *Core: a copy whose attribute slice does not alias the receiver's.
func (c *Core) clone() *Core {
	return &Core{
		LevelEnabler: c.LevelEnabler,
		holder:       c.holder,
		scope:        c.scope,
		attrs:        slices.Clone(c.attrs),
		spanCtx:      c.spanCtx,
		emitCtx:      c.emitCtx,
		err:          c.err,
	}
}

// Fields implements the Laisky zap fork's zapcore.Core by reporting the fields
// bound to this core.
//
// The bridge stores its bound state as OpenTelemetry attributes rather than as
// zap fields, because that is the form every emitted record needs and
// converting on each write would repeat the work. Reconstructing zap fields
// from attributes would be lossy in the other direction, so this reports an
// empty slice: the fork uses Fields() to let a wrapping core inspect bound
// state, and no wrapper reads the OTLP branch of the tee.
//
// Parameters: none.
//
// Return values:
//   - []zapcore.Field: always empty.
func (c *Core) Fields() []zapcore.Field { return nil }

// Check implements zapcore.Core by adding this core to ce when the entry's
// level passes the core's own level floor.
//
// Parameters:
//   - ent: the entry being checked.
//   - ce: the checked entry accumulating the cores that will write it.
//
// Return values:
//   - *zapcore.CheckedEntry: ce, with this core added when the entry is exported.
func (c *Core) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if !c.Enabled(ent.Level) {
		return ce
	}
	return ce.AddCore(ent, c)
}

// Write implements zapcore.Core by converting the entry into an OpenTelemetry
// log record and emitting it.
//
// It always returns nil. A returned error would make zap write to its internal
// error sink, and a transport problem in the log pipeline must not become
// additional log output; the outcome is counted instead.
//
// Parameters:
//   - ent: the entry to export.
//   - fields: the fields supplied at the log site.
//
// Return values:
//   - error: always nil, by design.
func (c *Core) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	scope := c.scope
	if ent.LoggerName != "" {
		scope = ent.LoggerName
	}

	lg, state := c.holder.logger(scope)
	if lg == nil {
		recordUnavailable(state)
		return nil
	}

	// Walk the fields exactly once: the conversion allocates an encoder and
	// touches every field, and both the record and its emit context need it.
	converted := convertFields(fields)
	record := c.buildRecord(ent, converted)
	emitCtx := c.emitContext(converted)

	lg.Emit(emitCtx, record)
	metrics.RecordAppLogExport(metrics.AppLogExportOutcomeEmitted, 1)
	return nil
}

// buildRecord converts a zap entry and its fields into an OpenTelemetry record.
//
// Parameters:
//   - ent: the entry being exported.
//   - converted: the already-walked fields supplied at the log site.
//
// Return values:
//   - log.Record: the record to emit.
func (c *Core) buildRecord(ent zapcore.Entry, converted convertedFields) log.Record {
	var record log.Record
	record.SetTimestamp(ent.Time)
	record.SetBody(attribute.StringValue(ent.Message))
	record.SetSeverity(convertLevel(ent.Level))
	record.SetSeverityText(ent.Level.String())

	recErr := c.err
	if converted.err != nil {
		recErr = converted.err
	}

	record.AddAttributes(c.attrs...)
	if ent.Caller.Defined {
		record.AddAttributes(
			attribute.String(string(semconv.CodeFilePathKey), ent.Caller.File),
			attribute.Int(string(semconv.CodeLineNumberKey), ent.Caller.Line),
			attribute.String(string(semconv.CodeFunctionNameKey), ent.Caller.Function),
		)
	}
	if ent.Stack != "" {
		stackKey := semconv.CodeStacktraceKey
		if recErr != nil || hasExceptionAttributes(c.attrs) || hasExceptionAttributes(converted.attrs) {
			stackKey = semconv.ExceptionStacktraceKey
		}
		record.AddAttributes(attribute.String(string(stackKey), ent.Stack))
	}
	record.AddAttributes(converted.attrs...)
	if recErr != nil {
		record.SetErr(recErr)
	}
	return record
}

// emitContext resolves the context a record is emitted with, which is what
// gives it trace and span correlation.
//
// Resolution order is: a span context supplied at the log site, then a context
// supplied at the log site (the upstream otelzap idiom), then whatever was
// latched by With, then a background context. The first two let a single call
// override a request-scoped logger's binding, which is what a detached
// goroutine logging on behalf of a different span needs.
//
// Parameters:
//   - converted: the already-walked fields supplied at the log site.
//
// Return values:
//   - context.Context: never nil; a background context when nothing correlates.
func (c *Core) emitContext(converted convertedFields) context.Context {
	if converted.spanCtx.IsValid() {
		return oteltrace.ContextWithSpanContext(context.Background(), converted.spanCtx)
	}
	if converted.ctx != nil {
		return converted.ctx
	}
	if c.spanCtx.IsValid() {
		return oteltrace.ContextWithSpanContext(context.Background(), c.spanCtx)
	}
	if c.emitCtx != nil {
		return c.emitCtx
	}
	return context.Background()
}

// Sync implements zapcore.Core by draining the export pipeline.
//
// Upstream's bridge makes this a no-op, which silently loses every queued
// record when zap.Fatal calls Sync and then exits. Flushing here means a fatal
// path, a test's explicit Sync, and the shutdown sequence all get the records
// that were already accepted.
//
// Parameters: none.
//
// Return values:
//   - error: the provider's flush failure, if any.
func (c *Core) Sync() error {
	return c.holder.flush(context.Background())
}

// convertedFields is the result of walking a zap field slice: the attributes to
// attach, plus the three field kinds that are consumed rather than attached.
type convertedFields struct {
	attrs   []attribute.KeyValue
	spanCtx oteltrace.SpanContext
	ctx     context.Context
	err     error
}

// convertFields walks zap fields, separating correlation and error fields from
// the fields that become record attributes.
//
// Parameters:
//   - fields: the fields to walk.
//
// Return values:
//   - convertedFields: the attributes plus any consumed correlation or error.
func convertFields(fields []zapcore.Field) convertedFields {
	var out convertedFields
	enc := newObjectEncoder(len(fields))

	for _, field := range fields {
		if sc, ok := spanContextFromField(field); ok {
			out.spanCtx = sc
			continue
		}
		if ctxField, ok := field.Interface.(context.Context); ok {
			out.ctx = ctxField
			continue
		}
		if field.Type == zapcore.ErrorType && field.Key == "error" {
			if err, ok := field.Interface.(error); ok && err != nil {
				out.err = err
				continue
			}
		}
		field.AddTo(enc)
	}

	enc.calculate(enc.root)
	out.attrs = enc.root.attrs
	return out
}

// hasExceptionAttributes reports whether attrs already describe an exception,
// which decides whether a stack trace is recorded as code.stacktrace or
// exception.stacktrace.
//
// Parameters:
//   - attrs: the attributes to inspect.
//
// Return values:
//   - bool: true when an exception message or type attribute is present.
func hasExceptionAttributes(attrs []attribute.KeyValue) bool {
	for _, attr := range attrs {
		if attr.Key == semconv.ExceptionMessageKey || attr.Key == semconv.ExceptionTypeKey {
			return true
		}
	}
	return false
}

// convertLevel maps a zap level onto an OpenTelemetry severity.
//
// The mapping matches the upstream otelzap bridge exactly, so severity-based
// queries written against one work against the other.
//
// Parameters:
//   - level: the zap level to map.
//
// Return values:
//   - log.Severity: the OpenTelemetry severity; SeverityUndefined for a level
//     the mapping does not cover.
func convertLevel(level zapcore.Level) log.Severity {
	switch level {
	case zapcore.DebugLevel:
		return log.SeverityDebug
	case zapcore.InfoLevel:
		return log.SeverityInfo
	case zapcore.WarnLevel:
		return log.SeverityWarn
	case zapcore.ErrorLevel:
		return log.SeverityError
	case zapcore.DPanicLevel:
		return log.SeverityFatal1
	case zapcore.PanicLevel:
		return log.SeverityFatal2
	case zapcore.FatalLevel:
		return log.SeverityFatal3
	default:
		return log.SeverityUndefined
	}
}
