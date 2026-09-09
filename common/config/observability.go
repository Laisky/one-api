// Package config provides centralized configuration management for one-api.
//
// This file holds the observability data-tiering knobs introduced by
// docs/proposals/20260905_observability-data-tiering.md. Every variable here is
// additive: with no environment set, the resolved values reproduce the
// pre-proposal behavior of a single-node, single-database deployment.

package config

import (
	"strings"
	"time"

	"github.com/Laisky/one-api/common/env"
)

// Observability profiles. The profile only selects DEFAULTS; every individual
// variable below remains independently overridable by its own environment
// variable, which always wins over the profile.
const (
	// ObservabilityProfileStandalone is the zero-dependency default: everything
	// stays in the primary SQL database and nothing is sampled away.
	ObservabilityProfileStandalone = "standalone"
	// ObservabilityProfileScaled keeps a single SQL database but trades trace
	// completeness for survivability at high request rates.
	ObservabilityProfileScaled = "scaled"
	// ObservabilityProfileExternal moves peripheral telemetry out of the
	// database entirely and emits it over OTLP.
	ObservabilityProfileExternal = "external"
)

// Trace sink identifiers accepted by TRACE_SINK. Multiple sinks may be combined
// with commas (for example "db,otlp") to fan a completed trace out to both.
const (
	// TraceSinkDB persists completed traces into the `traces` table.
	TraceSinkDB = "db"
	// TraceSinkOTLP emits completed traces as OpenTelemetry span events and
	// writes nothing to SQL.
	TraceSinkOTLP = "otlp"
	// TraceSinkNone drops completed traces after metrics are recorded.
	TraceSinkNone = "none"
)

// Trace write modes accepted by TRACE_WRITE_MODE.
const (
	// TraceWriteModeBatched accumulates a request's trace in memory and hands
	// the finished record to an asynchronous batching writer.
	TraceWriteModeBatched = "batched"
	// TraceWriteModeSync restores the pre-proposal behavior of writing every
	// trace mutation to the database synchronously on the request goroutine.
	// It exists as a debugging and rollback escape hatch, not for production.
	TraceWriteModeSync = "sync"
)

var (
	// ObservabilityProfile selects the default tier for every observability
	// knob in this file.
	//
	// Environment variable: OBSERVABILITY_PROFILE
	// Default: "standalone"
	// Allowed values: "standalone", "scaled", "external"
	ObservabilityProfile = normalizeProfile(env.String("OBSERVABILITY_PROFILE", ObservabilityProfileStandalone))

	// TraceSinks lists the sinks a completed trace is handed to, in order.
	//
	// Environment variable: TRACE_SINK (comma-separated)
	// Default: "db" (standalone, scaled), "otlp" (external)
	TraceSinks = parseTraceSinks(env.String("TRACE_SINK",
		profileString(ObservabilityProfile, TraceSinkDB, TraceSinkDB, TraceSinkOTLP)))

	// TraceWriteMode selects between in-memory accumulation with a single
	// batched write and the legacy per-mutation synchronous write path.
	//
	// The standalone default is "sync", the pre-proposal behavior. Batched
	// writes are the headline optimization, but they change something an
	// existing deployment can observe: a trace row does not exist until the
	// request ends, so the timeline of an in-flight request -- a five-minute
	// streaming relay, say -- is no longer queryable while it runs. That is a
	// capability today's users have, so taking it away is an explicit decision
	// (OBSERVABILITY_PROFILE=scaled, or TRACE_WRITE_MODE=batched) rather than
	// something an upgrade does to them.
	//
	// Environment variable: TRACE_WRITE_MODE
	// Default: "sync" for standalone, "batched" for scaled and external
	// Allowed values: "batched", "sync"
	TraceWriteMode = normalizeWriteMode(env.String("TRACE_WRITE_MODE",
		profileString(ObservabilityProfile, TraceWriteModeSync, TraceWriteModeBatched, TraceWriteModeBatched)))

	// TraceSampleRate is the probability in [0, 1] that an ordinary completed
	// trace is persisted. Traces kept by an always-sample rule bypass it.
	//
	// Environment variable: TRACE_SAMPLE_RATE
	// Default: 1.0 (standalone, external), 0.05 (scaled)
	TraceSampleRate = clampUnitInterval(env.Float64("TRACE_SAMPLE_RATE",
		profileFloat(ObservabilityProfile, 1.0, 0.05, 1.0)))

	// TraceAlwaysSampleErrors keeps every trace whose HTTP status is >= 400
	// regardless of TRACE_SAMPLE_RATE.
	//
	// Environment variable: TRACE_ALWAYS_SAMPLE_ERRORS
	// Default: true
	TraceAlwaysSampleErrors = env.Bool("TRACE_ALWAYS_SAMPLE_ERRORS", true)

	// TraceAlwaysSampleSlowMs keeps every trace whose total duration reaches
	// this many milliseconds, regardless of TRACE_SAMPLE_RATE. 0 disables the
	// rule.
	//
	// Environment variable: TRACE_ALWAYS_SAMPLE_SLOW_MS
	// Default: 0 (standalone, external), 5000 (scaled)
	// Unit: milliseconds
	TraceAlwaysSampleSlowMs = nonNegative(env.Int("TRACE_ALWAYS_SAMPLE_SLOW_MS",
		profileInt(ObservabilityProfile, 0, 5000, 0)))

	// TraceBatchSize is the maximum number of trace rows written per INSERT.
	//
	// Environment variable: TRACE_BATCH_SIZE
	// Default: 500
	TraceBatchSize = positiveOr(env.Int("TRACE_BATCH_SIZE", 500), 500)

	// TraceFlushIntervalMs bounds how long a partially filled batch waits
	// before it is written anyway.
	//
	// Environment variable: TRACE_FLUSH_INTERVAL_MS
	// Default: 1000
	// Unit: milliseconds
	TraceFlushIntervalMs = positiveOr(env.Int("TRACE_FLUSH_INTERVAL_MS", 1000), 1000)

	// TraceQueueSize bounds the number of completed traces buffered between the
	// request path and the writers. A full queue drops the newest record and
	// increments the drop counter; traces are best-effort by definition.
	//
	// Environment variable: TRACE_QUEUE_SIZE
	// Default: 20000 (standalone, external), 50000 (scaled)
	TraceQueueSize = positiveOr(env.Int("TRACE_QUEUE_SIZE",
		profileInt(ObservabilityProfile, 20000, 50000, 20000)), 20000)

	// TraceWriterCount is the number of goroutines draining the queue.
	//
	// Environment variable: TRACE_WRITER_COUNT
	// Default: 2 (standalone, external), 4 (scaled)
	TraceWriterCount = positiveOr(env.Int("TRACE_WRITER_COUNT",
		profileInt(ObservabilityProfile, 2, 4, 2)), 2)
)

// normalizeProfile lowercases and validates an observability profile name.
//
// Parameters:
//   - raw: the raw OBSERVABILITY_PROFILE value, possibly empty or misspelled.
//
// Return values:
//   - string: one of the ObservabilityProfile* constants; unrecognized input
//     falls back to standalone so a typo can never silently enable sampling.
func normalizeProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ObservabilityProfileScaled:
		return ObservabilityProfileScaled
	case ObservabilityProfileExternal:
		return ObservabilityProfileExternal
	default:
		return ObservabilityProfileStandalone
	}
}

// normalizeWriteMode lowercases and validates a trace write mode.
//
// Parameters:
//   - raw: the raw TRACE_WRITE_MODE value.
//
// Return values:
//   - string: one of the TraceWriteMode* constants; unrecognized input falls
//     back to batched.
func normalizeWriteMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), TraceWriteModeSync) {
		return TraceWriteModeSync
	}
	return TraceWriteModeBatched
}

// parseTraceSinks splits a comma-separated TRACE_SINK value into an ordered,
// de-duplicated list of known sink identifiers.
//
// Parameters:
//   - raw: the raw TRACE_SINK value, for example "db" or "db,otlp".
//
// Return values:
//   - []string: recognized sink identifiers in the order given. An empty or
//     fully unrecognized value yields []string{TraceSinkDB} so a typo degrades
//     to today's behavior rather than silently discarding all traces.
func parseTraceSinks(raw string) []string {
	seen := make(map[string]bool, 3)
	sinks := make([]string, 0, 3)
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		switch name {
		case TraceSinkDB, TraceSinkOTLP, TraceSinkNone:
		default:
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		sinks = append(sinks, name)
	}
	if len(sinks) == 0 {
		return []string{TraceSinkDB}
	}
	return sinks
}

// TraceSinkEnabled reports whether the given sink identifier is active.
//
// Parameters:
//   - name: one of the TraceSink* constants.
//
// Return values:
//   - bool: true when the resolved TRACE_SINK list contains name.
func TraceSinkEnabled(name string) bool {
	for _, s := range TraceSinks {
		if s == name {
			return true
		}
	}
	return false
}

// profileInt picks a per-profile default integer.
//
// Parameters:
//   - profile: the resolved observability profile.
//   - standalone, scaled, external: the default for each profile.
//
// Return values:
//   - int: the default matching profile.
func profileInt(profile string, standalone, scaled, external int) int {
	switch profile {
	case ObservabilityProfileScaled:
		return scaled
	case ObservabilityProfileExternal:
		return external
	default:
		return standalone
	}
}

// profileFloat picks a per-profile default float.
//
// Parameters:
//   - profile: the resolved observability profile.
//   - standalone, scaled, external: the default for each profile.
//
// Return values:
//   - float64: the default matching profile.
func profileFloat(profile string, standalone, scaled, external float64) float64 {
	switch profile {
	case ObservabilityProfileScaled:
		return scaled
	case ObservabilityProfileExternal:
		return external
	default:
		return standalone
	}
}

// profileString picks a per-profile default string.
//
// Parameters:
//   - profile: the resolved observability profile.
//   - standalone, scaled, external: the default for each profile.
//
// Return values:
//   - string: the default matching profile.
func profileString(profile string, standalone, scaled, external string) string {
	switch profile {
	case ObservabilityProfileScaled:
		return scaled
	case ObservabilityProfileExternal:
		return external
	default:
		return standalone
	}
}

// clampUnitInterval bounds a probability into [0, 1].
//
// Parameters:
//   - v: the raw probability.
//
// Return values:
//   - float64: v clamped to [0, 1].
func clampUnitInterval(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// nonNegative clamps a negative value to zero.
//
// Parameters:
//   - v: the raw value.
//
// Return values:
//   - int: v, or 0 when v is negative.
func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// positiveOr returns v when it is positive and fallback otherwise.
//
// Parameters:
//   - v: the raw value.
//   - fallback: the value used when v is not positive.
//
// Return values:
//   - int: a strictly positive value.
func positiveOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

// Phase 0 containment knobs (proposal
// docs/proposals/20260905_observability-data-tiering.md, W0.1-W0.5).

// Record-log line formats accepted by LOG_RECORD_LINE_FORMAT.
const (
	// LogRecordLineFull emits the pre-proposal "record log" INFO line, which
	// carries the rendered content string and created_at.
	LogRecordLineFull = "full"
	// LogRecordLineCompact drops content and created_at from the INFO line,
	// keeping the correlators and billing numbers. The full form remains
	// available at DEBUG level.
	LogRecordLineCompact = "compact"
)

// Application log sink identifiers accepted by APP_LOG_SINK.
const (
	// AppLogSinkFile writes only to the rotating file in the log directory.
	AppLogSinkFile = "file"
	// AppLogSinkStdout writes only to stdout/stderr. This is the correct choice
	// under Kubernetes, where the platform already collects and rotates
	// container output, and it removes the local-disk growth problem outright.
	AppLogSinkStdout = "stdout"
	// AppLogSinkBoth writes to stdout/stderr and to the rotating file.
	AppLogSinkBoth = "both"
)

var (
	// TraceExcludedPathPrefixes lists request path prefixes that are never
	// traced. Without it every static SPA asset, health probe, and metrics
	// scrape produces a trace row.
	//
	// The standalone default is EMPTY, because excluding a path silently removes
	// trace coverage an existing deployment has: an operator may be counting on
	// health-probe or /api/status timings being recorded. The scaled and
	// external profiles exclude the usual non-relay noise, where the volume
	// saving is the point.
	//
	// Environment variable: TRACE_EXCLUDED_PATH_PREFIXES (comma-separated)
	// Default: "" for standalone;
	//   "/api/status,/metrics,/health,/static,/assets,/favicon" for scaled and external
	// Set to "-" to disable the skip list explicitly.
	TraceExcludedPathPrefixes = parsePathPrefixes(env.String("TRACE_EXCLUDED_PATH_PREFIXES",
		profileString(ObservabilityProfile, "",
			"/api/status,/metrics,/health,/static,/assets,/favicon",
			"/api/status,/metrics,/health,/static,/assets,/favicon")))

	// RetentionDeleteBatchSize bounds how many rows a single retention DELETE
	// statement removes. An unbounded DELETE over a table with hundreds of
	// millions of expired rows opens one enormous transaction: a multi-terabyte
	// WAL burst plus table bloat on PostgreSQL, and a gap-locking stall on
	// MySQL.
	//
	// Environment variable: RETENTION_DELETE_BATCH_SIZE
	// Default: 5000
	RetentionDeleteBatchSize = positiveOr(env.Int("RETENTION_DELETE_BATCH_SIZE", 5000), 5000)

	// RetentionDeletePauseMs is how long a sweeper waits between chunks so it
	// yields the database to live traffic.
	//
	// 10 ms was chosen from a measured sweep over 0/10/25/50/100/200 ms on
	// PostgreSQL 17, MySQL 8.4 and SQLite, with a fixed 400 requests/second of
	// concurrent foreground load (see
	// docs/benchmarks/20260905_observability-phase0-phase1.md section 5.2):
	//
	//   - PostgreSQL: any pause >= 10 ms eliminates foreground degradation
	//     entirely (0.00% of concurrent requests slowed past 25 ms, versus 1.64%
	//     at pause 0 and 11.76% for the unbounded DELETE). Pausing longer buys
	//     nothing and costs throughput.
	//   - MySQL: 10 ms gives both the lowest degraded-request count in the sweep
	//     and the highest delete throughput.
	//   - Throughput at 10 ms is 1.6-4.5x higher than at 100 ms, which is what
	//     keeps an hourly sweep ahead of arrivals at high request rates.
	//
	// Yielding matters -- pause 0 is measurably worse on PostgreSQL -- but the
	// yield only has to be long enough to let a waiting transaction through.
	//
	// Environment variable: RETENTION_DELETE_PAUSE_MS
	// Default: 10
	// Unit: milliseconds
	RetentionDeletePauseMs = nonNegative(env.Int("RETENTION_DELETE_PAUSE_MS", 10))

	// RetentionSweepIntervalMinutes is how often background retention workers
	// run.
	//
	// The standalone default is 1440 minutes -- the historical 24-hour cadence
	// every retention worker used before this work. Sweeping hourly makes each
	// sweep smaller, which is the point at high volume, but cadence is
	// observable behavior: it changes when deletions happen and how often the
	// workers touch the database. An upgrade that changes it without being
	// asked violates the unchanged-configuration contract in section 2.1 of
	// docs/proposals/20260905_observability-data-tiering.md, which names this
	// specific gap as a G1 blocker. The scaled and external profiles opt in to
	// the hourly cadence.
	//
	// Environment variable: RETENTION_SWEEP_INTERVAL_MINUTES
	// Default: 1440 (24h) for standalone, 60 for scaled and external
	// Unit: minutes
	RetentionSweepIntervalMinutes = positiveOr(env.Int("RETENTION_SWEEP_INTERVAL_MINUTES",
		profileInt(ObservabilityProfile, 1440, 60, 60)), 1440)

	// LogMaxTotalSizeMB caps the total size of the log directory. When the
	// directory exceeds it, the retention worker deletes oldest-first until it
	// is back under the ceiling. This bounds disk even when a single day of
	// logs exceeds the volume, which day-based retention alone cannot do.
	//
	// Environment variable: LOG_MAX_TOTAL_SIZE_MB
	// Default: 0 (unlimited) for standalone, 20480 for scaled, 10240 for external
	// Unit: megabytes
	LogMaxTotalSizeMB = nonNegative(env.Int("LOG_MAX_TOTAL_SIZE_MB",
		profileInt(ObservabilityProfile, 0, 20480, 10240)))

	// LogMinFreeDiskMB is the free-space floor on the log volume. Below it the
	// retention worker purges oldest-first and, if that is not enough, raises
	// the effective log level so the process stops writing INFO. A gateway that
	// dies from a full disk is worse than a gateway that stops writing INFO.
	//
	// It is OFF by default under the standalone profile. It deletes files the
	// operator chose to keep and it changes the process log level, neither of
	// which an upgrade may do without being asked. The scaled and external
	// profiles enable it, because reaching them is an explicit decision.
	//
	// Environment variable: LOG_MIN_FREE_DISK_MB
	// Default: 0 (disabled) for standalone, 1024 for scaled and external
	// Unit: megabytes
	LogMinFreeDiskMB = nonNegative(env.Int("LOG_MIN_FREE_DISK_MB",
		profileInt(ObservabilityProfile, 0, 1024, 1024)))

	// AppLogSink selects where application logs are written.
	//
	// Environment variable: APP_LOG_SINK
	// Default: "both" (current behavior)
	// Allowed values: "file", "stdout", "both"
	AppLogSink = normalizeAppLogSink(env.String("APP_LOG_SINK", AppLogSinkBoth))

	// LogSampleInitial is how many entries with the same level and message are
	// kept in each sampling tick before thinning starts. 0 disables sampling.
	//
	// Sampling only applies below WARN: a repeated warning or error is a signal
	// about scale and must never be thinned away.
	//
	// Environment variable: LOG_SAMPLE_INITIAL
	// Default: 0 (disabled) for standalone, 100 for scaled and external
	LogSampleInitial = nonNegative(env.Int("LOG_SAMPLE_INITIAL",
		profileInt(ObservabilityProfile, 0, 100, 100)))

	// LogSampleThereafter is the thinning factor applied after the first
	// LogSampleInitial entries in a tick: one in every LogSampleThereafter is
	// kept. 0 drops everything after the initial budget.
	//
	// Environment variable: LOG_SAMPLE_THEREAFTER
	// Default: 100
	LogSampleThereafter = nonNegative(env.Int("LOG_SAMPLE_THEREAFTER", 100))

	// LogSampleTickMs is the sampling window length.
	//
	// Environment variable: LOG_SAMPLE_TICK_MS
	// Default: 1000
	// Unit: milliseconds
	LogSampleTickMs = positiveOr(env.Int("LOG_SAMPLE_TICK_MS", 1000), 1000)

	// LogRecordLineFormat selects the shape of the once-per-billed-request
	// "record log" INFO line.
	//
	// It defaults to "full" -- the pre-proposal shape -- under the standalone
	// profile, because an operator's log-parsing pipeline may depend on the
	// fields the compact form drops, and an upgrade must not silently change an
	// output contract. The scaled and external profiles default to "compact",
	// where the volume saving matters and reaching the profile is an explicit
	// decision. The full form is always available at DEBUG level.
	//
	// Environment variable: LOG_RECORD_LINE_FORMAT
	// Default: "full" for standalone, "compact" for scaled and external
	// Allowed values: "full", "compact"
	LogRecordLineFormat = normalizeRecordLineFormat(env.String("LOG_RECORD_LINE_FORMAT",
		profileString(ObservabilityProfile, LogRecordLineFull, LogRecordLineCompact, LogRecordLineCompact)))

	// DashboardCacheTTLSec is how long a computed dashboard payload and the
	// site-wide quota aggregate stay reusable.
	//
	// It is OFF by default under the standalone profile: an existing deployment
	// with Redis configured would otherwise start serving dashboard aggregates
	// up to a minute stale where they had always been live, without asking.
	//
	// Environment variable: DASHBOARD_CACHE_TTL_SEC
	// Default: 0 (disabled) for standalone, 60 for scaled and external
	// Unit: seconds
	DashboardCacheTTLSec = nonNegative(env.Int("DASHBOARD_CACHE_TTL_SEC",
		profileInt(ObservabilityProfile, 0, 60, 60)))

	// DashboardMaxSitewideRangeDays caps the date range a root user may request
	// for SITE-WIDE dashboard statistics. Per-user ranges are unaffected.
	//
	// Until the Phase-2 rollups exist, a site-wide aggregate scans every consume
	// log in the window, so an unbounded range is a guaranteed timeout at scale.
	// The standalone default keeps today's 365-day limit; the scaled and
	// external profiles opt into protection.
	//
	// Environment variable: DASHBOARD_MAX_SITEWIDE_RANGE_DAYS
	// Default: 365 for standalone, 31 for scaled and external
	// Unit: days
	DashboardMaxSitewideRangeDays = positiveOr(env.Int("DASHBOARD_MAX_SITEWIDE_RANGE_DAYS",
		profileInt(ObservabilityProfile, 365, 31, 31)), 365)
)

// parsePathPrefixes splits a comma-separated prefix list.
//
// Parameters:
//   - raw: the raw value; the single entry "-" disables the list entirely.
//
// Return values:
//   - []string: trimmed, non-empty prefixes, or nil when the list is disabled.
func parsePathPrefixes(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "-" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	prefixes := make([]string, 0, len(parts))
	for _, part := range parts {
		if p := strings.TrimSpace(part); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	return prefixes
}

// normalizeRecordLineFormat lowercases and validates a record-log line format.
//
// Parameters:
//   - raw: the raw LOG_RECORD_LINE_FORMAT value.
//
// Return values:
//   - string: one of the LogRecordLine* constants; unrecognized input falls back
//     to the pre-proposal full form.
func normalizeRecordLineFormat(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), LogRecordLineCompact) {
		return LogRecordLineCompact
	}
	return LogRecordLineFull
}

// normalizeAppLogSink lowercases and validates an application log sink name.
//
// Parameters:
//   - raw: the raw APP_LOG_SINK value.
//
// Return values:
//   - string: one of the AppLogSink* constants; unrecognized input falls back
//     to "both", which is the pre-proposal behavior.
func normalizeAppLogSink(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case AppLogSinkFile:
		return AppLogSinkFile
	case AppLogSinkStdout:
		return AppLogSinkStdout
	default:
		return AppLogSinkBoth
	}
}

// RetentionDeletePause returns the configured pause between retention chunks.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the pause; zero when pausing is disabled.
func RetentionDeletePause() time.Duration {
	return durationFromInt(RetentionDeletePauseMs, time.Millisecond)
}

// RetentionSweepInterval returns how often retention workers run.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the sweep interval.
func RetentionSweepInterval() time.Duration {
	return durationFromInt(RetentionSweepIntervalMinutes, time.Minute)
}

// DashboardCacheTTL returns how long a cached dashboard aggregate stays valid.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the TTL; zero when caching is disabled.
func DashboardCacheTTL() time.Duration {
	return durationFromInt(DashboardCacheTTLSec, time.Second)
}
