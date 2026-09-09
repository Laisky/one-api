package tracing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/common/telemetry"
	"github.com/Laisky/one-api/model"
)

// fakeSink records what it was handed so fan-out and selection can be asserted.
type fakeSink struct {
	mu        sync.Mutex
	submitted []*model.Trace
	flushes   int
	closes    int
	submitErr error
}

// Submit implements TraceSink.Submit by recording the row.
//
// Parameters:
//   - ctx: unused.
//   - row: the finished trace row.
//
// Return values:
//   - error: the configured submitErr, if any.
func (f *fakeSink) Submit(_ context.Context, row *model.Trace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, row)
	return f.submitErr
}

// Flush implements TraceSink.Flush by counting the call.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (f *fakeSink) Flush(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushes++
	return nil
}

// Close implements TraceSink.Close by counting the call.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (f *fakeSink) Close(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

// count returns how many rows the sink received.
//
// Parameters: none.
//
// Return values:
//   - int: number of submitted rows.
func (f *fakeSink) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.submitted)
}

// TestMultiSinkFansOutAndReportsFirstError verifies every child sink is
// attempted even when an earlier one fails.
func TestMultiSinkFansOutAndReportsFirstError(t *testing.T) {
	failing := &fakeSink{submitErr: errors.New("boom")}
	healthy := &fakeSink{}
	sink := &multiSink{sinks: []TraceSink{failing, healthy}}

	err := sink.Submit(context.Background(), newTestRow(t, "multi-a"))
	require.Error(t, err)
	require.Equal(t, 1, failing.count())
	require.Equal(t, 1, healthy.count(), "a failing sink must not stop the fan-out")

	require.NoError(t, sink.Flush(context.Background()))
	require.NoError(t, sink.Close(context.Background()))
	require.Equal(t, 1, failing.flushes)
	require.Equal(t, 1, healthy.flushes)
	require.Equal(t, 1, failing.closes)
	require.Equal(t, 1, healthy.closes)
}

// TestNullSinkDropsEverything verifies the drop sink is inert.
func TestNullSinkDropsEverything(t *testing.T) {
	var sink TraceSink = nullSink{}
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "null-a")))
	require.NoError(t, sink.Flush(context.Background()))
	require.NoError(t, sink.Close(context.Background()))
}

// TestSinkReturnsNullBeforeInit verifies the process sink is never nil, so a
// request served before InitSinks cannot panic.
func TestSinkReturnsNullBeforeInit(t *testing.T) {
	restore := SetSinkForTest(nil)
	defer restore()

	require.NotNil(t, Sink())
	require.NoError(t, Sink().Submit(context.Background(), newTestRow(t, "pre-init")))
}

// TestInitSinksRejectsOTLPWithoutOpenTelemetry verifies startup refuses an
// OTLP sink that would otherwise emit into the OpenTelemetry no-op provider.
func TestInitSinksRejectsOTLPWithoutOpenTelemetry(t *testing.T) {
	prevSinks := config.TraceSinks
	prevEnabled := config.OpenTelemetryEnabled
	prevEndpoint := config.OpenTelemetryEndpoint
	config.TraceSinks = []string{config.TraceSinkOTLP}
	config.OpenTelemetryEnabled = false
	config.OpenTelemetryEndpoint = ""
	t.Cleanup(func() {
		config.TraceSinks = prevSinks
		config.OpenTelemetryEnabled = prevEnabled
		config.OpenTelemetryEndpoint = prevEndpoint
	})

	require.Error(t, InitSinks(context.Background()))
}

// TestInitSinksRejectsIncoherentSyncConfiguration verifies direct sink
// initialization is fail-fast even when callers do not invoke ValidateAllEnvVars.
func TestInitSinksRejectsIncoherentSyncConfiguration(t *testing.T) {
	prevMode, prevSinks, prevRate := config.TraceWriteMode, config.TraceSinks, config.TraceSampleRate
	prevEnabled, prevEndpoint := config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint
	config.TraceWriteMode = config.TraceWriteModeSync
	config.TraceSinks = []string{config.TraceSinkOTLP}
	config.TraceSampleRate = 1
	config.OpenTelemetryEnabled = true
	config.OpenTelemetryEndpoint = "collector:4318"
	t.Cleanup(func() {
		config.TraceWriteMode, config.TraceSinks, config.TraceSampleRate = prevMode, prevSinks, prevRate
		config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint = prevEnabled, prevEndpoint
	})

	require.Error(t, InitSinks(context.Background()))
}

// TestInitSinksSelectsConfiguredSinks verifies TRACE_SINK selection, including
// the comma-separated fan-out form.
func TestInitSinksSelectsConfiguredSinks(t *testing.T) {
	useIsolatedTraceDB(t)
	installCountingRecorder(t)
	withSinkConfig(t, 10, 10, 1, 50)

	prevSinks := config.TraceSinks
	prevWriteMode := config.TraceWriteMode
	prevOpenTelemetryEnabled := config.OpenTelemetryEnabled
	prevOpenTelemetryEndpoint := config.OpenTelemetryEndpoint
	config.TraceWriteMode = config.TraceWriteModeBatched
	config.OpenTelemetryEnabled = true
	config.OpenTelemetryEndpoint = "collector:4318"
	// The otlp rows of section 3.2 need a provider that was really installed,
	// not merely OTEL_ENABLED=true; this test is about sink SELECTION, so it
	// states that precondition rather than re-testing it.
	t.Cleanup(telemetry.SetProviderInitializedForTest(true))
	t.Cleanup(func() {
		config.TraceSinks = prevSinks
		config.TraceWriteMode = prevWriteMode
		config.OpenTelemetryEnabled = prevOpenTelemetryEnabled
		config.OpenTelemetryEndpoint = prevOpenTelemetryEndpoint
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = Shutdown(ctx)
		SetSinkForTest(nil)()
	})

	t.Run("none", func(t *testing.T) {
		config.TraceSinks = []string{config.TraceSinkNone}
		require.NoError(t, InitSinks(context.Background()))
		_, ok := Sink().(nullSink)
		require.True(t, ok)
	})

	t.Run("db", func(t *testing.T) {
		config.TraceSinks = []string{config.TraceSinkDB}
		require.NoError(t, InitSinks(context.Background()))
		_, ok := Sink().(*sqlSink)
		require.True(t, ok)
	})

	t.Run("otlp", func(t *testing.T) {
		config.TraceSinks = []string{config.TraceSinkOTLP}
		require.NoError(t, InitSinks(context.Background()))
		_, ok := Sink().(otlpSink)
		require.True(t, ok)
	})

	t.Run("db and otlp fan out", func(t *testing.T) {
		config.TraceSinks = []string{config.TraceSinkDB, config.TraceSinkOTLP}
		require.NoError(t, InitSinks(context.Background()))
		multi, ok := Sink().(*multiSink)
		require.True(t, ok)
		require.Len(t, multi.sinks, 2)
	})

	t.Run("unknown sink is rejected", func(t *testing.T) {
		config.TraceSinks = []string{"cassandra"}
		require.Error(t, InitSinks(context.Background()))
	})
}

// TestOTLPSinkRejectsNonRecordingProvider verifies the OTLP sink does not
// claim an export when the process still has OpenTelemetry's no-op provider.
func TestOTLPSinkRejectsNonRecordingProvider(t *testing.T) {
	prev := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = prev })
	recorder := installCountingRecorder(t)

	sink := newOTLPSink()
	require.Error(t, sink.Submit(context.Background(), newTestRow(t, "otlp-a")))
	require.Zero(t, recorder.count(metrics.TraceOutcomeExported))
	require.NoError(t, sink.Flush(context.Background()))
	require.NoError(t, sink.Close(context.Background()))
}

// TestOTLPSinkReportsMalformedTimestamps verifies a corrupt stored document is
// surfaced rather than silently exported as an empty span.
func TestOTLPSinkReportsMalformedTimestamps(t *testing.T) {
	sink := newOTLPSink()
	require.Error(t, sink.Submit(context.Background(), &model.Trace{
		TraceId:    "otlp-broken",
		Timestamps: "{not json",
	}))
	require.NoError(t, sink.Submit(context.Background(), nil))
}
