package otelbridge

// Acceptance tests for the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2, gate
// G3: "Compile/integration with forked Zap, real trace/span correlation").
//
// These tests run the REAL OpenTelemetry log SDK against a recording exporter
// rather than a hand-written fake logger. A test double that accepts whatever
// the bridge produces would prove the bridge is self-consistent, not that the
// SDK accepts it; trace correlation in particular is filled in by the SDK from
// the emit context, so only the real Logger.Emit path can demonstrate it.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/metrics"
)

// semconvExceptionMessageKey mirrors the semantic-convention key the SDK uses
// when it expands Record.SetErr, so the assertion names the same constant the
// production path does.
const semconvExceptionMessageKey = "exception.message"

// recordingExporter is an sdklog.Exporter that keeps every exported record.
type recordingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
	failing bool
}

// Export implements sdklog.Exporter by retaining a clone of each record.
//
// Parameters:
//   - ctx: unused; the exporter never blocks.
//   - records: the batch to retain.
//
// Return values:
//   - error: a synthetic failure when the exporter is configured to fail.
func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failing {
		return errors.New("collector unreachable")
	}
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

// Shutdown implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (e *recordingExporter) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

// all returns a copy of every record exported so far.
//
// Parameters: none.
//
// Return values:
//   - []sdklog.Record: the retained records.
func (e *recordingExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// only returns the single retained record, failing the test otherwise.
//
// Parameters:
//   - t: the test handle.
//
// Return values:
//   - sdklog.Record: the one exported record.
func (e *recordingExporter) only(t *testing.T) sdklog.Record {
	t.Helper()
	records := e.all()
	require.Len(t, records, 1)
	return records[0]
}

// newTestBridge builds a zap logger whose only core is the bridge, wired to a
// real SDK provider that exports synchronously into the returned exporter.
//
// Parameters:
//   - t: the test handle; the provider is shut down on cleanup.
//   - level: the bridge's severity floor.
//
// Return values:
//   - *zap.Logger: a logger writing only to the bridge.
//   - *recordingExporter: the exporter holding what was exported.
//   - *ProviderHolder: the holder, so a test can close or reset it.
func newTestBridge(t *testing.T, level zapcore.LevelEnabler) (*zap.Logger, *recordingExporter, *ProviderHolder) {
	t.Helper()

	exporter := &recordingExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	holder := NewProviderHolder()
	require.NoError(t, holder.Install(provider))

	core := NewCore(holder, DefaultScopeName, level)
	return zap.New(core), exporter, holder
}

// attrOf returns the value of one attribute on a record.
//
// Parameters:
//   - record: the record to inspect.
//   - key: the attribute key to find.
//
// Return values:
//   - attribute.Value: the value, zero when absent.
//   - bool: whether the attribute was present.
func attrOf(record sdklog.Record, key string) (attribute.Value, bool) {
	var (
		found attribute.Value
		ok    bool
	)
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == key {
			found, ok = kv.Value, true
			return false
		}
		return true
	})
	return found, ok
}

// TestBridgeSatisfiesTheForkedZapCoreInterface is the compile-and-run half of
// the G3 requirement that the adapter integrate with the forked Zap.
//
// The upstream otelzap bridge cannot satisfy this: it is written against
// go.uber.org/zap, whose Core interface has no Fields() method, and whose Field
// is a different Go type entirely.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeSatisfiesTheForkedZapCoreInterface(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)

	logger.Info("hello", zap.String("model", "gpt-5"))

	record := exporter.only(t)
	require.Equal(t, "hello", record.Body().AsString())
	value, ok := attrOf(record, "model")
	require.True(t, ok)
	require.Equal(t, "gpt-5", value.AsString())
}

// TestBridgeMapsLevelsAndTimestamps pins the severity mapping and the entry
// fields the OTLP data model carries outside attributes.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeMapsLevelsAndTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name  string
		level zapcore.Level
	}{
		{name: "debug", level: zapcore.DebugLevel},
		{name: "info", level: zapcore.InfoLevel},
		{name: "warn", level: zapcore.WarnLevel},
		{name: "error", level: zapcore.ErrorLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)
			before := time.Now().Add(-time.Second)

			switch tc.level {
			case zapcore.DebugLevel:
				logger.Debug("m")
			case zapcore.InfoLevel:
				logger.Info("m")
			case zapcore.WarnLevel:
				logger.Warn("m")
			default:
				logger.Error("m")
			}

			record := exporter.only(t)
			require.Equal(t, convertLevel(tc.level), record.Severity())
			require.Equal(t, tc.level.String(), record.SeverityText())
			require.True(t, record.Timestamp().After(before))
		})
	}
}

// TestBridgeCorrelatesFromSpanContextField is the core G3 correlation
// requirement: a span context bound onto the logger must reach the exported
// record as its trace and span id.
//
// Trace correlation in OTLP lives in top-level LogRecord protocol fields, not
// in attributes, and the SDK fills them from the emit context. A bridge that
// wrote "trace_id" as an attribute would look correct in a unit test and join
// against nothing in a real backend, so this asserts the protocol fields.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeCorrelatesFromSpanContextField(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)

	traceID, err := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanCtx)

	logger.With(SpanContextField(ctx)).Info("correlated")

	record := exporter.only(t)
	require.Equal(t, traceID, record.TraceID())
	require.Equal(t, spanID, record.SpanID())
	require.Equal(t, oteltrace.FlagsSampled, record.TraceFlags())
}

// TestBridgeCorrelatesFromContextField keeps the upstream otelzap idiom
// working, so instrumentation written against that bridge behaves identically
// here.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeCorrelatesFromContextField(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)

	traceID, err := oteltrace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	spanID, err := oteltrace.SpanIDFromHex("b7ad6b7169203331")
	require.NoError(t, err)
	ctx := oteltrace.ContextWithSpanContext(context.Background(),
		oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID: traceID, SpanID: spanID, TraceFlags: oteltrace.FlagsSampled,
		}))

	logger.Info("correlated", zap.Any("context", ctx))

	record := exporter.only(t)
	require.Equal(t, traceID, record.TraceID())
	require.Equal(t, spanID, record.SpanID())
	_, present := attrOf(record, "context")
	require.False(t, present, "the context field is consumed, never emitted as an attribute")
}

// TestCorrelationFieldIsInvisibleToOtherEncoders is the compatibility half of
// the correlation design: section 2.1 promises "Full application log-line
// fields, existing sink selection", so binding correlation onto the request
// logger must not change one byte of the file or stdout output.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCorrelationFieldIsInvisibleToOtherEncoders(t *testing.T) {
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{0x01},
		SpanID:     oteltrace.SpanID{0x02},
		TraceFlags: oteltrace.FlagsSampled,
	})

	encode := func(fields ...zap.Field) string {
		var buf strings.Builder
		encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			MessageKey:  "msg",
			LevelKey:    "level",
			EncodeLevel: zapcore.LowercaseLevelEncoder,
		})
		core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.DebugLevel)
		zap.New(core).With(fields...).Info("line", zap.String("kept", "yes"))
		return buf.String()
	}

	withoutField := encode()
	withField := encode(SpanContextFieldFrom(spanCtx))

	require.Equal(t, withoutField, withField)
	require.NotContains(t, withField, SpanContextFieldKey)
}

// TestBridgeSeverityFloorDropsBelowThreshold proves the export floor is
// independent of the process log level, so running at debug locally does not
// start shipping debug volume to a shared collector.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeSeverityFloorDropsBelowThreshold(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.WarnLevel)

	logger.Info("not exported")
	logger.Warn("exported")

	record := exporter.only(t)
	require.Equal(t, "exported", record.Body().AsString())
}

// TestBridgeCountsRecordsBeforeProviderInstall proves the startup window is
// accounted rather than silent: main.go configures logging before it
// initializes OpenTelemetry, so early lines cannot be exported.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeCountsRecordsBeforeProviderInstall(t *testing.T) {
	spy := installLogExportSpy(t)

	holder := NewProviderHolder()
	logger := zap.New(NewCore(holder, DefaultScopeName, zapcore.DebugLevel))

	logger.Info("emitted before telemetry init")

	require.Equal(t, 1, spy.outcomes[metrics.AppLogExportOutcomeDroppedNotReady])
	require.Zero(t, spy.outcomes[metrics.AppLogExportOutcomeEmitted])
}

// TestBridgeCountsRecordsAfterShutdown proves the shutdown window is accounted
// too, and that a closed holder never reopens.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeCountsRecordsAfterShutdown(t *testing.T) {
	spy := installLogExportSpy(t)

	logger, _, holder := newTestBridge(t, zapcore.DebugLevel)
	holder.Close()

	logger.Info("emitted during shutdown")

	require.Equal(t, 1, spy.outcomes[metrics.AppLogExportOutcomeDroppedShutdown])
	require.Error(t, holder.Install(sdklog.NewLoggerProvider()),
		"a closed holder must not be reopened")
}

// TestBridgeSyncFlushesTheProvider pins the deliberate divergence from upstream
// otelzap, whose Sync is a hard-coded no-op. zap.Fatal calls Sync and then
// exits the process, so a no-op Sync loses every queued record at exactly the
// moment the records matter most.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeSyncFlushesTheProvider(t *testing.T) {
	flusher := &countingFlushProvider{LoggerProvider: sdklog.NewLoggerProvider()}
	holder := NewProviderHolder()
	require.NoError(t, holder.Install(flusher))

	require.NoError(t, zap.New(NewCore(holder, DefaultScopeName, zapcore.DebugLevel)).Sync())
	require.Equal(t, 1, flusher.flushes)
}

// countingFlushProvider counts ForceFlush calls made through the Flusher
// capability.
type countingFlushProvider struct {
	*sdklog.LoggerProvider
	flushes int
}

// ForceFlush implements Flusher by counting the call.
//
// Parameters:
//   - ctx: forwarded to the embedded provider.
//
// Return values:
//   - error: whatever the embedded provider reports.
func (p *countingFlushProvider) ForceFlush(ctx context.Context) error {
	p.flushes++
	return p.LoggerProvider.ForceFlush(ctx)
}

// TestBridgeRecordsErrorsAsExceptions pins that zap.Error is converted through
// Record.SetErr, which the SDK expands into exception semantic conventions,
// rather than being flattened into a plain "error" string attribute.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeRecordsErrorsAsExceptions(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)

	logger.Error("relay failed", zap.Error(errors.New("upstream refused")))

	record := exporter.only(t)
	value, ok := attrOf(record, string(semconvExceptionMessageKey))
	require.True(t, ok, "an error must surface as an exception attribute")
	require.Equal(t, "upstream refused", value.AsString())
}

// TestBridgeNestsNamespacesAsMaps pins that zap.Namespace produces a nested
// attribute map, matching the upstream bridge so a query written against one
// works against the other.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBridgeNestsNamespacesAsMaps(t *testing.T) {
	logger, exporter, _ := newTestBridge(t, zapcore.DebugLevel)

	logger.Info("nested", zap.Namespace("relay"), zap.String("channel", "openai"))

	record := exporter.only(t)
	value, ok := attrOf(record, "relay")
	require.True(t, ok)
	require.Equal(t, attribute.MAP, value.Type())
}

// TestConvertValueBoundsRecursionDepth proves a self-referential value cannot
// exhaust the stack from inside the logger.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestConvertValueBoundsRecursionDepth(t *testing.T) {
	type node struct{ Next any }

	var cyclic any
	holder := &node{}
	holder.Next = holder
	cyclic = []any{[]any{[]any{[]any{[]any{[]any{[]any{[]any{[]any{"deep"}}}}}}}}}

	require.NotPanics(t, func() { convertValue(cyclic) })
	require.NotPanics(t, func() { convertValue(holder) })
}

// logExportSpy records the export outcomes the bridge reported.
type logExportSpy struct {
	metrics.MetricsRecorder

	outcomes map[string]int
}

// RecordAppLogExportRecords implements metrics.LogExportRecorder by tallying.
//
// Parameters:
//   - outcome: the reported outcome.
//   - count: how many records it applies to.
//
// Return values: none.
func (s *logExportSpy) RecordAppLogExportRecords(outcome string, count int) {
	s.outcomes[outcome] += count
}

// UpdateAppLogExportQueue implements metrics.LogExportRecorder as a no-op.
//
// Parameters:
//   - records, recordLimit, bytes, byteLimit: ignored.
//
// Return values: none.
func (s *logExportSpy) UpdateAppLogExportQueue(_, _, _, _ float64) {}

// installLogExportSpy installs a recorder that captures export outcomes and
// restores the previous recorder when the test ends.
//
// Parameters:
//   - t: the test handle.
//
// Return values:
//   - *logExportSpy: the installed spy.
func installLogExportSpy(t *testing.T) *logExportSpy {
	t.Helper()

	spy := &logExportSpy{
		MetricsRecorder: metrics.Recorder(),
		outcomes:        make(map[string]int),
	}
	previous := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(previous) })
	return spy
}
