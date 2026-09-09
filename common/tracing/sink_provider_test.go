package tracing

// Startup rejection of an OTLP sink with no installed provider (proposal
// docs/proposals/20260905_observability-data-tiering.md, section 3.2, row "Any
// sink including otlp | batched | OTEL_ENABLED=false -> Reject; never count a
// no-op provider as export").
//
// OTEL_ENABLED only records that an operator ASKED for OpenTelemetry.
// ValidateTraceSinkOpenTelemetryConfig already enforces that flag, but nothing
// checked whether telemetry.InitOpenTelemetry actually installed providers. When
// it did not -- it failed, or it ran after InitSinks -- otel.GetTracerProvider()
// answers with the global no-op provider, every Submit finds
// !span.IsRecording(), and the startup misconfiguration is rediscovered once per
// request as TraceOutcomeSpanRecordFailed while traces go nowhere.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/telemetry"
)

// useOTLPSinkConfig installs a section 3.2-valid otlp configuration and
// restores the previous values afterwards.
//
// Parameters:
//   - t: the test, used to register the restore.
//   - sinks: the TRACE_SINK identifiers under test.
//
// Return values: none.
func useOTLPSinkConfig(t *testing.T, sinks ...string) {
	t.Helper()

	prevSinks, prevMode, prevRate := config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate
	prevEnabled, prevEndpoint := config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint

	config.TraceSinks = sinks
	config.TraceWriteMode = config.TraceWriteModeBatched
	config.TraceSampleRate = 1
	config.OpenTelemetryEnabled = true
	config.OpenTelemetryEndpoint = "collector:4318"

	t.Cleanup(func() {
		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate = prevSinks, prevMode, prevRate
		config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint = prevEnabled, prevEndpoint
	})
}

// TestInitSinksRejectsOTLPWithoutInitializedProvider verifies startup fails when
// TRACE_SINK asks for otlp while the process still carries OpenTelemetry's
// global no-op provider, instead of installing a sink that reports
// span_record_failed on every request forever.
func TestInitSinksRejectsOTLPWithoutInitializedProvider(t *testing.T) {
	useOTLPSinkConfig(t, config.TraceSinkOTLP)
	// OTEL_ENABLED is true and the endpoint is set: only the missing provider
	// distinguishes this from a healthy configuration.
	t.Cleanup(telemetry.SetProviderInitializedForTest(false))

	err := InitSinks(context.Background())
	require.Error(t, err, "an otlp sink built on the no-op provider must fail startup")
	require.Contains(t, err.Error(), "initialized OpenTelemetry provider")

	_, installed := Sink().(otlpSink)
	require.False(t, installed, "a rejected configuration must not install the sink anyway")
}

// TestInitSinksRejectsOTLPFanOutWithoutInitializedProvider verifies the same
// rejection for the db,otlp fan-out form, where a working SQL sink could
// otherwise mask the fact that nothing is exported.
func TestInitSinksRejectsOTLPFanOutWithoutInitializedProvider(t *testing.T) {
	useOTLPSinkConfig(t, config.TraceSinkDB, config.TraceSinkOTLP)
	t.Cleanup(telemetry.SetProviderInitializedForTest(false))

	require.Error(t, InitSinks(context.Background()))
}

// TestInitSinksValidatesEveryChildBeforeStartingWorkers verifies an invalid
// later fan-out child cannot leave SQL writer goroutines behind.
func TestInitSinksValidatesEveryChildBeforeStartingWorkers(t *testing.T) {
	useOTLPSinkConfig(t, config.TraceSinkDB, config.TraceSinkOTLP)
	t.Cleanup(telemetry.SetProviderInitializedForTest(false))

	before := sqlSinkWorkerCount.Load()
	require.Error(t, InitSinks(context.Background()))
	require.Equal(t, before, sqlSinkWorkerCount.Load(),
		"a rejected fan-out must not start a partial SQL sink")
}

// TestInitSinksAcceptsOTLPWithInitializedProvider verifies the check gates on
// the provider actually being installed, and nothing else: the identical
// configuration succeeds once telemetry reports a real provider.
func TestInitSinksAcceptsOTLPWithInitializedProvider(t *testing.T) {
	useOTLPSinkConfig(t, config.TraceSinkOTLP)
	t.Cleanup(telemetry.SetProviderInitializedForTest(true))
	t.Cleanup(SetSinkForTest(nil))

	require.NoError(t, InitSinks(context.Background()))
	_, installed := Sink().(otlpSink)
	require.True(t, installed)
}

// TestInitSinksRejectsNoneCombinedWithAnotherSink verifies the ambiguous
// combination is refused at sink construction as well as in the environment
// matrix. config.TraceSinks is a package variable, so a direct assignment could
// otherwise reintroduce "db,none" -- which traceDisabled()'s len(sinks)==1 check
// reads as "db plus a sink that drops every record" rather than "tracing off".
func TestInitSinksRejectsNoneCombinedWithAnotherSink(t *testing.T) {
	useOTLPSinkConfig(t, config.TraceSinkDB, config.TraceSinkNone)
	t.Cleanup(telemetry.SetProviderInitializedForTest(true))

	err := InitSinks(context.Background())
	require.Error(t, err, "db,none is ambiguous and must not start")
	require.Contains(t, err.Error(), "validate trace sink combination")
}

// TestInitSinksRejectsEmptySinkList verifies an empty TRACE_SINK is refused
// rather than silently degraded into a drop-everything sink.
func TestInitSinksRejectsEmptySinkList(t *testing.T) {
	useOTLPSinkConfig(t)
	t.Cleanup(telemetry.SetProviderInitializedForTest(true))

	require.Error(t, InitSinks(context.Background()))
}
