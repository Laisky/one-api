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
	require.Equal(t, 1440, RetentionSweepIntervalMinutes,
		"retention workers must keep their historical 24-hour cadence; when deletions happen is observable behavior")
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

// TestCursorCapabilityIsOffByDefault verifies an upgrade does not silently move
// any existing deployment onto the keyset log routes.
//
// The keyset order needs an access path the shipped schema does not have. On
// MySQL 8.4 the first cursor page is a full table scan plus a filesort of every
// row — measured at 5.9 s on 2,000,000 rows against 0.8 ms for the legacy offset
// page it would replace (docs/benchmarks/20260906_w24-cursor-plans.md). A
// default-on capability would therefore make the log page dramatically slower
// for existing operators, which is exactly what these guarantees forbid.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorCapabilityIsOffByDefault(t *testing.T) {
	require.False(t, LogCursorEnabled,
		"LOG_CURSOR_ENABLED must stay off until W2.5 establishes the access paths")
}

// TestCursorBudgetsAreBoundedByDefault verifies that an operator who does enable
// the capability still gets bounded work without configuring anything.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorBudgetsAreBoundedByDefault(t *testing.T) {
	require.Positive(t, LogCursorMaxResponseBytes, "a page must be bounded in bytes")
	require.Positive(t, LogCountProbeMaxRows, "counting must be bounded in rows")
	require.Positive(t, LogCountProbeTimeoutMs, "counting must be bounded in time")
	require.Positive(t, LogCountProbeMaxConcurrent, "counting must be bounded in concurrency")
	require.LessOrEqual(t, LogCountExactMaxConcurrent, LogCountProbeMaxConcurrent,
		"the unbounded-cost count must not be allowed more concurrency than the bounded one")
}

// TestStandaloneDefaultsAddNoNewRejection verifies no default introduces a way
// for a request that used to succeed to start failing.
//
// A resource BOUND is compatible when it cannot engage in healthy operation. A
// BUDGET that refuses work is different in kind: it turns load into an error
// the caller never saw before, so it stays opt-in even though it would protect
// the database. That distinction is the line between the two default policies
// in section 3.1, and it is easy to erode by accident.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestStandaloneDefaultsAddNoNewRejection(t *testing.T) {
	require.Zero(t, DashboardMaxConcurrentAggregates,
		"a concurrency budget refuses dashboard requests; coalescing already removes the duplicate work, "+
			"so the budget must stay opt-in rather than introduce a new user-visible failure mode")
}

// TestBoundedResourceDefaultsAreCompatible documents why the bounds that DO
// default to enabled are not compatibility breaks.
//
// Each one replaces an unbounded resource whose exhaustion kills the process.
// They are sized so that reaching them already means the deployment is
// malfunctioning: a gateway does not hold 200000 concurrent in-flight requests,
// and does not write a single 4 GiB log file, in healthy operation. The
// alternative to the bound is not "the old behavior", it is an out-of-memory
// kill or a full disk -- which destroys far more of an operator's data than the
// bound ever sheds.
//
// The assertions are lower bounds, not exact values: tuning a limit upward is
// not a compatibility question, but dropping one to a level a real deployment
// could reach would be.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBoundedResourceDefaultsAreCompatible(t *testing.T) {
	require.GreaterOrEqual(t, TraceMaxActiveRecorders, 100000,
		"the active-recorder bound must sit far above real concurrency, so it replaces an OOM rather than dropping traces")
	require.GreaterOrEqual(t, LogMaxActiveFileSizeMB, 1024,
		"the active log file ceiling must sit far above any healthy single file, so rotation replaces a full disk")
	require.GreaterOrEqual(t, TraceMaxRecordBytes, 65536,
		"the per-record bound must not truncate an ordinary trace")
	require.GreaterOrEqual(t, TraceMaxExternalCalls, 256,
		"the external-call bound must not truncate an ordinary retry or tool sequence")

	require.LessOrEqual(t, LogDiskCheckIntervalSec, 60,
		"the disk guard must sample fast enough to matter: at 16 MB/s a 1 GB reserve lasts about 62 seconds")
}
