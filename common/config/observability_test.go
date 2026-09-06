package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeProfile verifies unknown profiles fall back to standalone, so a
// typo can never silently enable sampling or move telemetry off the database.
func TestNormalizeProfile(t *testing.T) {
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile(""))
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile("  standalone "))
	require.Equal(t, ObservabilityProfileScaled, normalizeProfile("SCALED"))
	require.Equal(t, ObservabilityProfileExternal, normalizeProfile("external"))
	require.Equal(t, ObservabilityProfileStandalone, normalizeProfile("scaledd"))
}

// TestParseTraceSinks verifies sink list parsing, de-duplication, and the
// fail-safe fallback.
func TestParseTraceSinks(t *testing.T) {
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks("db"))
	require.Equal(t, []string{TraceSinkDB, TraceSinkOTLP}, parseTraceSinks(" db , otlp "))
	require.Equal(t, []string{TraceSinkOTLP}, parseTraceSinks("otlp,otlp"))
	require.Equal(t, []string{TraceSinkNone}, parseTraceSinks("none"))
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks(""), "an empty value must not discard traces")
	require.Equal(t, []string{TraceSinkDB}, parseTraceSinks("cassandra"), "a typo must not discard traces")
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

// TestNormalizeAppLogSink verifies sink selection defaults to today's behavior.
func TestNormalizeAppLogSink(t *testing.T) {
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

// TestNumericClamps verifies the bounds helpers used by every knob.
func TestNumericClamps(t *testing.T) {
	require.InDelta(t, 0.0, clampUnitInterval(-1), 1e-9)
	require.InDelta(t, 1.0, clampUnitInterval(2), 1e-9)
	require.InDelta(t, 0.5, clampUnitInterval(0.5), 1e-9)

	require.Equal(t, 0, nonNegative(-5))
	require.Equal(t, 5, nonNegative(5))

	require.Equal(t, 10, positiveOr(0, 10))
	require.Equal(t, 10, positiveOr(-1, 10))
	require.Equal(t, 3, positiveOr(3, 10))
}

// TestObservabilityValidators verifies the fail-fast startup checks.
func TestObservabilityValidators(t *testing.T) {
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
