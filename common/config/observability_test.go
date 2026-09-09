package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TWO LAYERS, AND WHICH ONE THESE TESTS PIN
//
// Observability configuration is validated in two places and the difference is
// deliberate (proposal sections 3.1 and 3.2, W1):
//
//  1. The RAW layer (observability_env.go) inspects the environment string an
//     operator actually set. An explicitly set invalid value is REJECTED there,
//     before normalization can change its meaning. That is the contract these
//     tests assert with ValidateObservabilityRawInput, and it is what fails
//     startup.
//  2. The NORMALIZER layer (observability.go) keeps its fail-safe fallbacks as a
//     last line of defense, for values that never passed through the raw layer:
//     package variables assigned directly by a test, or by common/tracing's own
//     re-validation. Nothing invalid reaches it in production anymore, but if
//     something did, it must still resolve to something harmless rather than
//     panic or silently discard every trace.
//
// So the normalizer assertions below are NOT the release contract. Each one is
// paired with the raw-layer rejection that is.

// TestNormalizeProfile verifies both layers for OBSERVABILITY_PROFILE: an
// explicitly misspelled profile is rejected, and the normalizer still resolves
// an unvalidated one to standalone rather than to a sampling profile.
func TestNormalizeProfile(t *testing.T) {
	// Layer 1: the raw input is rejected; a typo never becomes a valid profile.
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvObservabilityProfile: "scaledd"})),
		"an explicitly misspelled profile must be rejected, not normalized")
	require.Empty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvObservabilityProfile: " SCALED "})),
		"case and surrounding space are normalized by the reader, so they are not rejected")

	// Layer 2: the fallback survives for directly assigned values.
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile(""))
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile("  standalone "))
	require.Equal(t, ObservabilityProfileScaled, normalizeProfile("SCALED"))
	require.Equal(t, ObservabilityProfileExternal, normalizeProfile("external"))
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile("scaledd"),
		"the last-resort fallback stays standalone, which cannot enable sampling")
}

// TestParseTraceSinks verifies both layers for TRACE_SINK: an explicitly unknown
// sink is rejected, and the normalizer still degrades an unvalidated list to db
// rather than discarding every trace.
func TestParseTraceSinks(t *testing.T) {
	// Layer 1: unknown tokens are rejected instead of being discarded, so
	// TRACE_SINK=cassandra can never start a process that quietly writes SQL.
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceSink: "cassandra"})),
		"an unknown sink must be rejected, not discarded")
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceSink: "db,cassandra"})),
		"one unknown token must reject the whole list")
	require.Empty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceSink: " db , otlp "})),
		"spacing and case are normalized, not rejected")

	// Layer 1 also rejects the ambiguous combination, which is a matrix rule
	// because it is reachable from profile defaults as well as from a variable.
	require.Error(t, ValidateTraceSinkCombination([]string{TraceSinkDB, TraceSinkNone}),
		"none combined with another sink is ambiguous and must be rejected")
	require.NoError(t, ValidateTraceSinkCombination([]string{TraceSinkNone}),
		"none alone is the documented way to disable trace recording")

	// Layer 2: parsing, de-duplication and the fail-safe fallback.
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks("db"))
	require.Equal(t, []string{TraceSinkDB, TraceSinkOTLP}, parseTraceSinks(" db , otlp "))
	require.Equal(t, []string{TraceSinkOTLP}, parseTraceSinks("otlp,otlp"))
	require.Equal(t, []string{TraceSinkNone}, parseTraceSinks("none"))
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks(""), "an empty value must not discard traces")
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks("cassandra"),
		"the last-resort fallback keeps writing traces rather than discarding them")
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks("db,cassandra"))
}

// TestParsePathPrefixes verifies the never-trace list, including the explicit
// disable form.
func TestParsePathPrefixes(t *testing.T) {
	require.Equal(t, []string{"/metrics", "/health"}, parsePathPrefixes(" /metrics , /health "))
	require.Nil(t, parsePathPrefixes(""))
	require.Nil(t, parsePathPrefixes("-"), "a single dash disables the skip list")
	require.Equal(t, []string{"/a"}, parsePathPrefixes("/a,,"))
}

// TestNormalizeAppLogSink verifies both layers for APP_LOG_SINK: an explicitly
// unknown sink is rejected, and the normalizer still resolves an unvalidated one
// to today's behavior.
func TestNormalizeAppLogSink(t *testing.T) {
	// Layer 1: rejected, because "syslog" silently becoming "both" writes to a
	// destination the operator did not choose.
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvAppLogSink: "syslog"})))
	require.Empty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvAppLogSink: " FILE "})))

	// Layer 2: the fallback survives for directly assigned values.
	require.Equal(t, AppLogSinkBoth, normalizeAppLogSink(""))
	require.Equal(t, AppLogSinkBoth, normalizeAppLogSink("both"))
	require.Equal(t, AppLogSinkFile, normalizeAppLogSink(" FILE "))
	require.Equal(t, AppLogSinkStdout, normalizeAppLogSink("stdout"))
	require.Equal(t, AppLogSinkBoth, normalizeAppLogSink("syslog"))
}

// TestProfileDefaultSelectors verifies the per-profile default helpers.
func TestProfileDefaultSelectors(t *testing.T) {
	require.Equal(t, 1, profileInt(ObservabilityProfileStandalone, 1, 2, 3))
	require.Equal(t, 2, profileInt(ObservabilityProfileScaled, 1, 2, 3))
	require.Equal(t, 3, profileInt(ObservabilityProfileExternal, 1, 2, 3))

	require.InDelta(t, 1.0, profileFloat(ObservabilityProfileStandalone, 1.0, 0.05, 1.0), 1e-9)
	require.InDelta(t, 0.05, profileFloat(ObservabilityProfileScaled, 1.0, 0.05, 1.0), 1e-9)

	require.Equal(t, TraceSinkDB, profileString(ObservabilityProfileScaled, TraceSinkDB, TraceSinkDB, TraceSinkOTLP))
	require.Equal(t, TraceSinkOTLP, profileString(ObservabilityProfileExternal, TraceSinkDB, TraceSinkDB, TraceSinkOTLP))
}

// TestNumericClamps verifies both layers for the numeric knobs: an explicitly
// out-of-range value is rejected, and the clamp helpers still bound an
// unvalidated one.
func TestNumericClamps(t *testing.T) {
	// Layer 1: an out-of-range or unparseable number is rejected rather than
	// silently clamped or replaced by its default.
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceSampleRate: "5"})),
		"a percentage typed as a probability must be rejected, not clamped to 1")
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceBatchSize: "0"})),
		"a non-positive batch size must be rejected, not replaced by 500")
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceAlwaysSampleSlowMs: "-1"})),
		"a negative threshold must be rejected, not clamped to 0")

	// Layer 2: the clamps survive for directly assigned values.
	require.InDelta(t, 0.0, clampUnitInterval(-1), 1e-9)
	require.InDelta(t, 1.0, clampUnitInterval(2), 1e-9)
	require.InDelta(t, 0.5, clampUnitInterval(0.5), 1e-9)

	require.Equal(t, 0, nonNegative(-5))
	require.Equal(t, 5, nonNegative(5))

	require.Equal(t, 10, positiveOr(0, 10))
	require.Equal(t, 10, positiveOr(-1, 10))
	require.Equal(t, 3, positiveOr(3, 10))
}

// TestObservabilityValidators verifies the second-layer checks that inspect an
// already-resolved value.
//
// These can only fire for a value assigned directly to a package variable: on
// the environment path the raw layer has already rejected anything they would
// catch. TestObservabilityConfigurationMatrix covers the environment path.
func TestObservabilityValidators(t *testing.T) {
	require.NotEmpty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(
		map[string]string{EnvTraceWriteMode: "async"})),
		"an unknown write mode must be rejected, not normalized to batched")

	require.NoError(t, ValidateObservabilityProfile(ObservabilityProfileScaled))
	require.Error(t, ValidateObservabilityProfile("turbo"))

	require.NoError(t, ValidateTraceWriteMode(TraceWriteModeSync))
	require.Error(t, ValidateTraceWriteMode("async"))

	require.NoError(t, ValidateAppLogSink(AppLogSinkStdout))
	require.Error(t, ValidateAppLogSink("syslog"))
}

// TestDurationHelpers verifies the millisecond and minute conversions.
func TestDurationHelpers(t *testing.T) {
	prevPause, prevSweep, prevTTL := RetentionDeletePauseMs, RetentionSweepIntervalMinutes, DashboardCacheTTLSec
	t.Cleanup(func() {
		RetentionDeletePauseMs, RetentionSweepIntervalMinutes, DashboardCacheTTLSec = prevPause, prevSweep, prevTTL
	})

	RetentionDeletePauseMs = 250
	RetentionSweepIntervalMinutes = 30
	DashboardCacheTTLSec = 90

	require.Equal(t, "250ms", RetentionDeletePause().String())
	require.Equal(t, "30m0s", RetentionSweepInterval().String())
	require.Equal(t, "1m30s", DashboardCacheTTL().String())
}
