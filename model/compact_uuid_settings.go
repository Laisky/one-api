package model

// This file is the model package's accessor layer over the compact UUID storage settings
// (AUTO-011). Keeping the accessors here rather than reading config directly from the
// coordinator means the bounds documented in the proposal are enforced in exactly one place,
// and a test can hold a setting without reaching into the config package's globals.

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// compactAutoMigrateEnabled reports whether the automatic compact worker may mutate state.
//
// It defaults to true: unset configuration must reach validated completion with no operator
// action. A false value is an emergency pause, not a supported steady state — it mutates no
// schema or markers, but a paused deployment cannot satisfy automatic-completion acceptance.
// Parameters: none.
//
// Return values:
//   - bool: true when automatic compact migration is enabled.
func compactAutoMigrateEnabled() bool {
	return config.CompactUUIDAutoMigrate
}

// compactBatchSize returns the configured per-statement row batch ceiling.
// Parameters: none.
//
// Return values:
//   - int: maximum rows one repair statement may carry before the bind cap is applied.
func compactBatchSize() int {
	return config.CompactUUIDBatchSize
}

// compactRowBudget returns the examined-row ceiling for one cycle, counted globally across
// roles, tables, and columns.
// Parameters: none.
//
// Return values:
//   - int: maximum target rows one cycle may examine.
func compactRowBudget() int {
	return config.CompactUUIDMaxRowsPerCycle
}

// compactCycleDuration returns the wall-clock ceiling for one cycle's row reconciliation.
//
// DDL and full validation deliberately run outside this budget under compactDDLTimeout and
// compactValidationTimeout, so neither is silently truncated by the row budget.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum duration of one cycle's row reconciliation.
func compactCycleDuration() time.Duration {
	return config.CompactUUIDMaxCycleDuration
}

// compactActiveInterval returns the delay before rescheduling a cycle that observed backlog.
// Parameters: none.
//
// Return values:
//   - time.Duration: delay after an active cycle.
func compactActiveInterval() time.Duration {
	return config.CompactUUIDActiveInterval
}

// compactIdleInterval returns the no-work reschedule delay and the per-process audit cadence.
// Parameters: none.
//
// Return values:
//   - time.Duration: delay after an idle cycle and between health audits.
func compactIdleInterval() time.Duration {
	return config.CompactUUIDIdleInterval
}

// compactHealthTTL returns how long one process may trust its last successful audit.
//
// The proposal fixes this at twice the idle interval: a process that has missed two audit
// cycles has no current evidence that compact objects are intact, so it must fall back to
// legacy predicates rather than keep trusting a stale pass.
// Parameters: none.
//
// Return values:
//   - time.Duration: health expiry window.
func compactHealthTTL() time.Duration {
	return 2 * compactIdleInterval()
}

// compactRetryInterval returns the base delay for the bounded full-jitter backoff.
// Parameters: none.
//
// Return values:
//   - time.Duration: retry base delay.
func compactRetryInterval() time.Duration {
	return config.CompactUUIDRetryInterval
}

// compactLockTimeout returns the bounded ownership lock-acquisition timeout.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum time spent acquiring the ownership lock, capped at five seconds.
func compactLockTimeout() time.Duration {
	return config.CompactUUIDLockTimeout
}

// compactDDLTimeout returns the bounded statement timeout for one compact DDL statement.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum duration of one DDL statement.
func compactDDLTimeout() time.Duration {
	return config.CompactUUIDDDLTimeout
}

// compactValidationTimeout returns the bounded timeout for one global validation pass.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum duration of one validation or fingerprint pass.
func compactValidationTimeout() time.Duration {
	return config.CompactUUIDValidationTimeout
}

// compactLogger returns the context-aware structured logger for compact migration events.
// Parameters:
//   - ctx: context carrying the scoped logger.
//
// Return values:
//   - glog.Logger: the context's logger.
func compactLogger(ctx context.Context) glog.Logger {
	return logger.FromContext(ctx)
}

// withCompactLogger seeds a context with the application's configured logger.
//
// This is required rather than cosmetic: gmw.GetLogger never returns nil, it falls back to the
// shared library logger, so a context carrying no logger would silently route every compact
// migration event away from this application's configured sinks and level.
// Parameters:
//   - ctx: context to seed.
//
// Return values:
//   - context.Context: context carrying the configured logger.
func withCompactLogger(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return gmw.SetLogger(ctx, logger.Logger)
}
