package config

// Backward-compatibility guarantees for the observability data-tiering work
// (docs/proposals/20260905_observability-data-tiering.md).
//
// The rule this file pins: an existing operator who upgrades WITHOUT changing
// their configuration must observe no data deleted, no output contract changed,
// and no capability removed. Every optimization that would violate that is
// reachable only through OBSERVABILITY_PROFILE or its own variable.
//
// These assertions are deliberately about the STANDALONE defaults. If a future
// change makes one of them fail, that change is a compatibility break and needs
// a release note, not a test edit.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStandaloneDefaultsDeleteNothing verifies no default causes an upgrade to
// remove files or rows the previous version kept.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestStandaloneDefaultsDeleteNothing(t *testing.T) {
	require.Equal(t, ObservabilityProfileStandalone, ObservabilityProfile,
		"these guarantees describe the standalone profile")

	require.Zero(t, LogRetentionDays,
		"log file retention must stay off by default: enabling it deletes files an existing operator kept")
	require.Zero(t, LogMaxTotalSizeMB,
		"the log directory size ceiling must stay off by default")
	require.Zero(t, LogMinFreeDiskMB,
		"the free-disk guard deletes files and raises the log level; it must stay off by default")
}

// TestStandaloneDefaultsPreserveOutputContracts verifies no default changes what
// an existing consumer of logs or dashboards sees.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestStandaloneDefaultsPreserveOutputContracts(t *testing.T) {
	require.Equal(t, LogRecordLineFull, LogRecordLineFormat,
		"the per-request log line must keep its pre-proposal fields: a log pipeline may parse them")
	require.Zero(t, LogSampleInitial,
		"log sampling drops entries; it must stay off by default")
	require.Equal(t, AppLogSinkBoth, AppLogSink,
		"log destinations must be unchanged by default")
	require.Zero(t, DashboardCacheTTLSec,
		"dashboard aggregates must stay live by default rather than becoming up to a minute stale")
	require.Equal(t, 365, DashboardMaxSitewideRangeDays,
		"the site-wide dashboard range must keep the existing 365-day limit")
}

// TestStandaloneDefaultsPreserveTraceBehaviour verifies tracing keeps working
// exactly as it did, including for requests that are still running.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestStandaloneDefaultsPreserveTraceBehaviour(t *testing.T) {
	require.Equal(t, TraceWriteModeSync, TraceWriteMode,
		"batched writes make an in-flight request's timeline unqueryable; that must be opt-in")
	require.Empty(t, TraceExcludedPathPrefixes,
		"excluding paths removes trace coverage an existing deployment has; that must be opt-in")
	require.Equal(t, []string{TraceSinkDB}, TraceSinks,
		"traces must still go to the database by default")
	require.InDelta(t, 1.0, TraceSampleRate, 1e-9,
		"no trace may be sampled away by default")
}

// TestScaledProfileOptsIntoTheOptimizations verifies the optimizations are
// actually reachable, so the compatible defaults are not simply a revert.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestScaledProfileOptsIntoTheOptimizations(t *testing.T) {
	require.Equal(t, TraceWriteModeBatched, profileString(ObservabilityProfileScaled,
		TraceWriteModeSync, TraceWriteModeBatched, TraceWriteModeBatched))
	require.Equal(t, LogRecordLineCompact, profileString(ObservabilityProfileScaled,
		LogRecordLineFull, LogRecordLineCompact, LogRecordLineCompact))
	require.Equal(t, 3, profileInt(ObservabilityProfileScaled, 0, 3, 1), "scaled enables log retention")
	require.Equal(t, 100, profileInt(ObservabilityProfileScaled, 0, 100, 100), "scaled enables log sampling")
	require.Equal(t, 60, profileInt(ObservabilityProfileScaled, 0, 60, 60), "scaled enables dashboard caching")
	require.InDelta(t, 0.05, profileFloat(ObservabilityProfileScaled, 1.0, 0.05, 1.0), 1e-9,
		"scaled enables trace sampling")
}
