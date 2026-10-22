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
// The bridge adapts upstream otelzap concepts to the forked Zap core. It
// additionally preserves application-selected Zap/JSON values and rejects
// unrepresentable records without partial export; consumers must qualify their
// actual OTLP collector/storage mapping rather than assume wire equivalence.
//
// Two things here are NOT copied from upstream, both because upstream's choice
// is wrong for a gateway:
//
//  1. Sync() force-flushes the provider. Upstream hard-codes `return nil` and
//     closed the issue asking for more as not-planned, which means a zap
//     Fatal (which calls Sync then os.Exit) loses everything still queued.
//  2. A trace correlation path that does not retain a context. Upstream latches
//     any field whose Interface holds a context.Context. This repository uses
//     an explicit carrier instead, and deliberately
//     keeps request loggers free of the gin context (they are snapshotted by
//     value into detached billing goroutines), so binding a request context
//     into a long-lived logger would retain the whole request. SpanContextField
//     carries the 24 bytes that actually matter instead.

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"unicode/utf8"

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
// A Core never blocks the caller on exporter I/O and never logs. Encoding
// failures return a fixed error and increment a fixed outcome counter. The
// core never logs its own failures: doing so would recursively invoke itself.
// Every loss is counted through common/metrics instead.
type Core struct {
	zapcore.LevelEnabler

	holder *ProviderHolder
	scope  string

	// attrs are the attributes accumulated by With, already converted.
	attrs         []attribute.KeyValue
	namespacePath []string
	conversionErr error
	// spanCtx is the correlation latched by With from a SpanContextField.
	spanCtx oteltrace.SpanContext
	// exceptionAttrs snapshot derived metadata without retaining a bound error.
	exceptionAttrs []attribute.KeyValue
	hasError       bool
	// private selects the explicit legacy privacy API, never the default sink.
	private bool
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
	if len(fields) == 0 || cloned.conversionErr != nil {
		// Once a bound field failed encoding, later With calls cannot make
		// its OTLP record valid. Do not invoke further OTLP serializers.
		return cloned
	}

	converted := c.convertFields(fields)
	if len(cloned.namespacePath)+len(converted.namespaces) > maxLogEncodingDepth {
		cloned.conversionErr = ErrLogEncoding
		return cloned
	}
	cloned.attrs = appendNamespaced(cloned.attrs, cloned.namespacePath, converted.attrs)
	cloned.namespacePath = append(cloned.namespacePath, converted.namespaces...)
	if converted.conversionErr != nil {
		cloned.conversionErr = converted.conversionErr
	}
	if converted.spanCtx.IsValid() {
		cloned.spanCtx = converted.spanCtx
	}
	if converted.err != nil {
		attrs := cloned.attrs // Previously inferred metadata is not an explicit application override.
		derived, err := DeriveLogExceptionAttributes(converted.err, attrs, converted.errMessage)
		if err != nil {
			cloned.conversionErr = ErrLogEncoding
		} else {
			cloned.exceptionAttrs = derived
			cloned.hasError = true
		}
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
		LevelEnabler:   c.LevelEnabler,
		holder:         c.holder,
		scope:          c.scope,
		attrs:          slices.Clone(c.attrs),
		namespacePath:  slices.Clone(c.namespacePath),
		conversionErr:  c.conversionErr,
		spanCtx:        c.spanCtx,
		exceptionAttrs: slices.Clone(c.exceptionAttrs),
		hasError:       c.hasError,
		private:        c.private,
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
// Encoding failures return a fixed diagnostic and reject the whole record;
// they never print failed values. Best-effort transport failures are counted
// separately and do not synchronously block or fail the application log call.
//
// Parameters:
//   - ent: the entry to export.
//   - fields: the fields supplied at the log site.
//
// Return values:
//   - error: a fixed encoding failure, or nil after successful local handoff.
func (c *Core) Write(ent zapcore.Entry, fields []zapcore.Field) (result error) {
	defer func() {
		if recover() != nil {
			metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
			result = ErrLogEncoding
		}
	}()
	scope := c.scope
	if ent.LoggerName != "" && !c.private {
		scope = ent.LoggerName
	}

	lg, state := c.holder.logger(scope)
	if lg == nil {
		recordUnavailable(state)
		return nil
	}

	// Walk the fields exactly once: the conversion allocates an encoder and
	// touches every field, and both the record and its emit context need it.
	// Validate known entry/bound state before calling application field
	// serializers. Rejection must not trigger avoidable user callbacks.
	if c.conversionErr != nil || (!c.private && (!utf8.ValidString(ent.Message) || ValidateLogTimestamp(ent.Time) != nil)) {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		return ErrLogEncoding
	}
	converted := c.convertFields(fields)
	if converted.conversionErr != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		return ErrLogEncoding
	}
	record, err := c.buildRecord(ent, converted)
	if err != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		return ErrLogEncoding
	}
	var attrs []attribute.KeyValue
	record.WalkAttributes(func(kv attribute.KeyValue) bool { attrs = append(attrs, kv); return true })
	if !c.private && ValidateLogRecordAttributes(attrs) != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedEncoding, 1)
		return ErrLogEncoding
	}
	emitCtx := c.emitContext(converted)
	if c.private {
		emitCtx = context.WithValue(emitCtx, privateContextKey{}, true)
	}

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
func (c *Core) buildRecord(ent zapcore.Entry, converted convertedFields) (log.Record, error) {
	var record log.Record
	record.SetTimestamp(ent.Time)
	if c.private {
		record.SetBody(attribute.StringValue("application log"))
	} else {
		record.SetBody(attribute.StringValue(ent.Message))
	}
	record.SetSeverity(convertLevel(ent.Level))
	record.SetSeverityText(ent.Level.String())

	appAttrs := appendNamespaced(c.attrs, c.namespacePath, converted.attrs)
	record.AddAttributes(appAttrs...)
	if ent.Caller.Defined {
		for _, kv := range []attribute.KeyValue{
			attribute.String(string(semconv.CodeFilePathKey), ent.Caller.File), attribute.Int(string(semconv.CodeLineNumberKey), ent.Caller.Line), attribute.String(string(semconv.CodeFunctionNameKey), ent.Caller.Function),
		} {
			if !hasLogKey(appAttrs, kv.Key) {
				record.AddAttributes(kv)
			}
		}
	}
	if ent.Stack != "" && !c.private {
		stackKey := semconv.CodeStacktraceKey
		if c.hasError || converted.err != nil || hasExceptionAttributes(c.attrs) || hasExceptionAttributes(converted.attrs) {
			stackKey = semconv.ExceptionStacktraceKey
		}
		if !hasLogKey(appAttrs, attribute.Key(stackKey)) {
			record.AddAttributes(attribute.String(string(stackKey), ent.Stack))
		}
	}
	if !c.private {
		derived := c.exceptionAttrs
		if converted.err != nil {
			var err error
			derived, err = DeriveLogExceptionAttributes(converted.err, appAttrs, converted.errMessage)
			if err != nil {
				return log.Record{}, err
			}
		}
		for _, kv := range derived {
			if !hasLogKey(appAttrs, kv.Key) {
				record.AddAttributes(kv)
			}
		}
	}
	return record, nil
}

// emitContext resolves the context a record is emitted with, which is what
// gives it trace and span correlation.
//
// Resolution order is an explicit span context supplied at the log site, then
// one latched by With, then a background context. The explicit field lets a call
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
	if c.spanCtx.IsValid() {
		return oteltrace.ContextWithSpanContext(context.Background(), c.spanCtx)
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
// attach, explicit correlation state, and additive SDK exception metadata.
type convertedFields struct {
	attrs      []attribute.KeyValue
	spanCtx    oteltrace.SpanContext
	err        error
	errMessage *string

	namespaces    []string
	conversionErr error
}

// convertFields preserves encoded app fields, extracting only the explicit
// correlation carrier and retaining additive SDK exception metadata.
//
// Parameters:
//   - fields: the fields to walk.
//
// Return values:
//   - convertedFields: the attributes plus any consumed correlation or error.
func convertFields(fields []zapcore.Field) (out convertedFields) {
	defer func() {
		if recover() != nil {
			out.conversionErr = ErrLogEncoding
		}
	}()
	enc := newObjectEncoder(len(fields))

	for _, field := range fields {
		// An earlier serializer or namespace already doomed the complete
		// OTLP record. Do not evaluate any remaining application callbacks.
		if *enc.failure != nil {
			break
		}
		if sc, ok := spanContextFromField(field); ok {
			out.spanCtx = sc
			continue
		}
		if field.Type == zapcore.ErrorType && field.Key == "error" {
			if err, ok := field.Interface.(error); ok && err != nil {
				out.err = err
			}
		}
		start := len(enc.cur.attrs)
		switch field.Type {
		case zapcore.InlineMarshalerType:
			if err := field.Interface.(zapcore.ObjectMarshaler).MarshalLogObject(enc); err != nil {
				*enc.failure = ErrLogEncoding
			}
		case zapcore.StringerType:
			value := reflect.ValueOf(field.Interface)
			if value.Kind() == reflect.Pointer && value.IsNil() {
				field.AddTo(enc) // Zap's declared nil-pointer Stringer projection.
			} else {
				enc.AddString(field.Key, field.Interface.(fmt.Stringer).String())
			}
		default:
			field.AddTo(enc)
		}
		if field.Type == zapcore.ErrorType {
			for _, kv := range enc.cur.attrs[start:] {
				if string(kv.Key) == field.Key+"Error" {
					*enc.failure = ErrLogEncoding
				}
				if field.Key == "error" && string(kv.Key) == field.Key && kv.Value.Type() == attribute.STRING {
					message := kv.Value.AsString()
					out.errMessage = &message
				}
			}
		}
	}

	for node := enc.root.next; node != nil; node = node.next {
		out.namespaces = append(out.namespaces, node.name)
	}
	enc.calculate(enc.root)
	out.conversionErr = *enc.failure
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
