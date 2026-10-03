// Package config provides centralized configuration management for one-api.
//
// This file holds the bounded-resource knobs required to close the W0 and W1
// acceptance gates of docs/proposals/20260905_observability-data-tiering.md.
// They exist because the Phase 0/1 release shipped the telemetry pipeline
// without the limits that make its resource use provable: an application log
// file with no size ceiling, a disk guard that only ran at the slow retention
// cadence, an unbounded per-trace recorder, and a batch writer that could drain
// an entire 50000-record queue into one slice.
//
// Every variable here is additive. With no environment set, the standalone
// profile keeps the pre-existing behavior wherever a limit is observable, and
// only enables a bound where its absence is a defect rather than a contract.

package config

import (
	"math"
	"time"

	"github.com/Laisky/one-api/common/env"
)

var (
	// LogMaxActiveFileSizeMB caps the size of the CURRENTLY OPEN application log
	// file, rotating it when the ceiling is reached.
	//
	// LogMaxTotalSizeMB cannot do this: it only deletes already-rotated files,
	// so a single day of output at 16 MB/s grows one active file past any
	// directory budget with nothing able to reclaim it (W0.2). Time-based
	// rotation alone has the same hole, because the window is wall-clock, not
	// bytes.
	//
	// It is ON under every profile, generously sized. Leaving it off by default
	// would ship the defect: the default deployment is exactly the one with no
	// operator watching the log volume. 4 GiB is far above any healthy single
	// file -- a deployment that reaches it is already malfunctioning -- so the
	// rotation this triggers is only ever the alternative to filling the disk,
	// never a routine event that changes the file names a log pipeline sees.
	//
	// Environment variable: LOG_MAX_ACTIVE_FILE_SIZE_MB
	// Default: 4096 for standalone, 2048 for scaled, 1024 for external
	// Unit: mebibytes (1 << 20)
	LogMaxActiveFileSizeMB = nonNegative(env.Int("LOG_MAX_ACTIVE_FILE_SIZE_MB",
		profileInt(ObservabilityProfile, 4096, 2048, 1024)))

	// LogDiskCheckIntervalSec is how often the disk-pressure guard samples free
	// space and the active-file size.
	//
	// This is deliberately NOT the retention sweep interval. Retention deletes
	// expired data and is inherently slow work; disk pressure is a survival
	// signal that must be sampled far faster than data expires. At a measured
	// 16 MB/s of application output, a 1 GB reserve lasts about 62.5 seconds, so
	// a guard tied to the sweep cadence -- 24 hours standalone, 1 hour scaled --
	// cannot protect it (W0.3).
	//
	// Environment variable: LOG_DISK_CHECK_INTERVAL_SEC
	// Default: 5
	// Unit: seconds
	LogDiskCheckIntervalSec = positiveOr(env.Int("LOG_DISK_CHECK_INTERVAL_SEC", 5), 5)

	// LogEmergencyMaxBytesPerSec is the byte budget application logging keeps
	// once disk headroom is exhausted.
	//
	// Raising the log level is not a bound: WARN and ERROR remain fully enabled,
	// so an error storm -- the exact condition that accompanies a failing
	// gateway -- still fills the volume. In emergency mode the writer admits at
	// most this many bytes per second across ALL levels and counts what it
	// suppressed (W0.4).
	//
	// Environment variable: LOG_EMERGENCY_MAX_BYTES_PER_SEC
	// Default: 1048576 (1 MiB/s)
	// Unit: bytes per second
	LogEmergencyMaxBytesPerSec = positiveOr(env.Int("LOG_EMERGENCY_MAX_BYTES_PER_SEC", 1<<20), 1<<20)

	// LogDiskRecoveryMarginPct is the hysteresis band between entering and
	// leaving disk-pressure emergency mode.
	//
	// Escalating at `free < floor` and restoring at `free >= floor` uses one
	// threshold, so free space oscillating around the floor flaps the log level
	// and the emergency policy. Recovery instead requires
	// `free >= floor * (1 + pct/100)` (W0.4).
	//
	// Environment variable: LOG_DISK_RECOVERY_MARGIN_PCT
	// Default: 20
	// Unit: percent of the configured free-disk floor
	LogDiskRecoveryMarginPct = nonNegative(env.Int("LOG_DISK_RECOVERY_MARGIN_PCT", 20))

	// DashboardMaxConcurrentAggregates bounds how many dashboard aggregate
	// computations may run at once on this node.
	//
	// The TTL cache amortizes repeated reads but does nothing for a cold or
	// expired key: every concurrent miss runs all six aggregate queries, and
	// with Redis unavailable the cache is disabled outright so EVERY request
	// does. Misses are coalesced per cache key regardless of this setting; the
	// budget additionally caps distinct concurrent scopes (W0.6).
	//
	// It is unlimited by default under standalone, where rejecting a dashboard
	// request would be a new user-visible failure mode.
	//
	// Environment variable: DASHBOARD_MAX_CONCURRENT_AGGREGATES
	// Default: 0 (unlimited) for standalone, 2 for scaled and external
	DashboardMaxConcurrentAggregates = nonNegative(env.Int("DASHBOARD_MAX_CONCURRENT_AGGREGATES",
		profileInt(ObservabilityProfile, 0, 2, 2)))

	// TraceMaxRecordBytes bounds the retained size of a single in-memory trace
	// record: timestamps, external-call entries, and every retained string.
	//
	// At 10000 RPS with a 60-second mean streaming lifetime there are roughly
	// 600000 active recorders, so an unbounded per-record size makes the memory
	// model unprovable (W1, "Recorder memory"). Exceeding the bound truncates
	// and is counted; it never drops the trace or fails the request.
	//
	// Environment variable: TRACE_MAX_RECORD_BYTES
	// Default: 262144 for standalone, 65536 for scaled and external
	// Minimum: 1024
	// Unit: bytes
	TraceMaxRecordBytes = positiveOr(env.Int("TRACE_MAX_RECORD_BYTES",
		profileInt(ObservabilityProfile, 262144, 65536, 65536)), 262144)

	// TraceMaxExternalCalls bounds how many external-call entries one trace
	// retains. A relay that retries across many channels, or a tool loop, can
	// otherwise append without limit for the whole request lifetime.
	//
	// Environment variable: TRACE_MAX_EXTERNAL_CALLS
	// Default: 1024 for standalone, 256 for scaled and external
	TraceMaxExternalCalls = positiveOr(env.Int("TRACE_MAX_EXTERNAL_CALLS",
		profileInt(ObservabilityProfile, 1024, 256, 256)), 1024)

	// TraceMaxActiveRecorders bounds how many in-flight requests may hold a
	// trace recorder simultaneously. Beyond the bound a request runs normally
	// but records no trace, which is counted as an explicit admission drop.
	//
	// The completed-record queue alone does not bound trace memory: it only
	// holds FINISHED records. Long-lived streaming requests accumulate on the
	// active side, which nothing observed before this bound (W1, "Active
	// requests").
	//
	// It is bounded under EVERY profile. "Unlimited" is not the safer default,
	// it is the defect: an unbounded active set is an out-of-memory kill, and a
	// gateway that dies loses far more trace coverage than a bound that sheds
	// the tail of a pathological spike. 200000 concurrent in-flight requests is
	// orders of magnitude beyond what a standalone deployment sees, so the
	// limit never engages in healthy operation -- it only replaces the crash.
	// Set 0 to restore the unbounded pre-proposal behavior.
	//
	// Environment variable: TRACE_MAX_ACTIVE_RECORDERS
	// Default: 200000 for every profile; 0 disables the bound
	TraceMaxActiveRecorders = nonNegative(env.Int("TRACE_MAX_ACTIVE_RECORDERS",
		profileInt(ObservabilityProfile, 200000, 200000, 200000)))

	// TraceBatchMaxBytes bounds the flush-local buffer of the SQL trace writer.
	//
	// TRACE_BATCH_SIZE bounds rows per INSERT but not the bytes one writer
	// materializes: a flush that drains the whole queue builds a slice of up to
	// TRACE_QUEUE_SIZE records before writing any of them. This caps the bytes a
	// single drain may accumulate (W1, "SQL batching").
	//
	// Environment variable: TRACE_BATCH_MAX_BYTES
	// Default: 8388608 (8 MiB)
	// Unit: bytes
	TraceBatchMaxBytes = positiveOr(env.Int("TRACE_BATCH_MAX_BYTES", 8<<20), 8<<20)
)

// LogMaxActiveFileSizeBytes returns the active application log file ceiling.
//
// Parameters: none.
//
// Return values:
//   - int64: the ceiling in bytes; zero when size rotation is disabled.
func LogMaxActiveFileSizeBytes() int64 {
	return mebibytesToBytes(LogMaxActiveFileSizeMB)
}

// LogMaxTotalSizeBytes returns the log-directory retention ceiling in bytes.
//
// Parameters: none.
//
// Return values:
//   - int64: the byte ceiling, or zero when directory-size retention is disabled.
func LogMaxTotalSizeBytes() int64 {
	return mebibytesToBytes(LogMaxTotalSizeMB)
}

// LogMinFreeDiskBytes returns the configured free-space floor in bytes.
//
// Parameters: none.
//
// Return values:
//   - int64: the byte floor, or zero when the free-space guard is disabled.
func LogMinFreeDiskBytes() int64 {
	return mebibytesToBytes(LogMinFreeDiskMB)
}

// LogDiskCheckInterval returns how often the disk-pressure guard samples.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the sampling interval; always strictly positive.
func LogDiskCheckInterval() time.Duration {
	return durationFromInt(LogDiskCheckIntervalSec, time.Second)
}

// LogDiskRecoveryFloorBytes returns the free-space level at which the disk
// guard leaves emergency mode, applying the configured hysteresis margin above
// the entry floor.
//
// Parameters:
//   - floorBytes: the configured free-disk floor, in bytes.
//
// Return values:
//   - int64: the recovery threshold in bytes; equals floorBytes when the margin
//     is zero.
func LogDiskRecoveryFloorBytes(floorBytes int64) int64 {
	if floorBytes <= 0 || LogDiskRecoveryMarginPct <= 0 {
		return floorBytes
	}
	// Divide before multiplying, and check the sum: `floorBytes*pct` overflows
	// int64 for a large floor, and a wrapped negative product would return a
	// recovery threshold BELOW the entry floor -- silently disabling the very
	// hysteresis this function exists to provide, in the one direction no test
	// of ordinary values would catch.
	marginPct := int64(LogDiskRecoveryMarginPct)
	base := floorBytes / 100
	if base > 0 && marginPct > math.MaxInt64/base {
		return math.MaxInt64
	}
	margin := base * marginPct
	if margin > math.MaxInt64-floorBytes {
		return math.MaxInt64
	}
	return floorBytes + margin
}

// mebibytesToBytes converts a non-negative mebibyte count into a byte ceiling.
//
// Parameters:
//   - mebibytes: a configured size in mebibytes; zero disables the ceiling.
//
// Return values:
//   - int64: the corresponding byte count, saturated at MaxInt64 when a
//     direct package-variable assignment bypassed raw environment validation.
func mebibytesToBytes(mebibytes int) int64 {
	if mebibytes <= 0 {
		return 0
	}
	const bytesPerMebibyte = int64(1 << 20)
	if int64(mebibytes) > math.MaxInt64/bytesPerMebibyte {
		return math.MaxInt64
	}
	return int64(mebibytes) * bytesPerMebibyte
}

// durationFromInt converts an integer setting to a duration without allowing a
// direct package-variable assignment to wrap it negative.
//
// Parameters:
//   - value: the configured unit count.
//   - unit: the duration represented by one count.
//
// Return values:
//   - time.Duration: the requested duration, saturated at its maximum value.
func durationFromInt(value int, unit time.Duration) time.Duration {
	if value <= 0 || unit <= 0 {
		return 0
	}
	if int64(value) > math.MaxInt64/int64(unit) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(value) * unit
}
