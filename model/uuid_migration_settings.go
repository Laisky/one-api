package model

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// uuidMigrationLogger returns the context-aware logger for migration events.
// Model helpers take their logger from context so a worker- or request-scoped logger keeps
// its fields. The bootstrap boundary seeds that logger with withUUIDMigrationLogger, so this
// never has to reach for a global.
// Parameters:
//   - ctx: context carrying the scoped logger.
//
// Return values:
//   - glog.Logger: the context's logger.
func uuidMigrationLogger(ctx context.Context) glog.Logger {
	return logger.FromContext(ctx)
}

// withUUIDMigrationLogger seeds a context with the application's configured logger.
//
// This is required, not cosmetic: gmw.GetLogger falls back to the shared library logger
// rather than returning nil, so a context that carries no logger would silently send every
// migration event to that shared logger instead of the sinks and level this application
// configured. The bootstrap boundary calls this once, and every phase then inherits it.
// Parameters:
//   - ctx: context to seed.
//
// Return values:
//   - context.Context: context carrying the configured logger.
func withUUIDMigrationLogger(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return gmw.SetLogger(ctx, logger.Logger)
}

// uuidCatchUpRowBudget returns the configured examined-row ceiling for one catch-up cycle.
// The budget is counted globally across roles, phases, tables, and columns in a cycle.
// Parameters: none.
//
// Return values:
//   - int: maximum target rows one cycle may examine.
func uuidCatchUpRowBudget() int {
	return config.ExternalUUIDBackfillMaxRowsPerCycle
}

// uuidCatchUpTimeBudget returns the configured wall-clock ceiling for one catch-up cycle.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum duration of one cycle.
func uuidCatchUpTimeBudget() time.Duration {
	return config.ExternalUUIDBackfillMaxCycleDuration
}

// uuidCatchUpActiveInterval returns the delay before rescheduling a cycle with backlog.
// Parameters: none.
//
// Return values:
//   - time.Duration: delay after an active cycle.
func uuidCatchUpActiveInterval() time.Duration {
	return config.ExternalUUIDBackfillActiveInterval
}

// uuidCatchUpIdleInterval returns the delay after a full no-work pass.
// Parameters: none.
//
// Return values:
//   - time.Duration: delay after an idle cycle.
func uuidCatchUpIdleInterval() time.Duration {
	return config.ExternalUUIDBackfillIdleInterval
}

// uuidLockTimeout returns the bounded lock-acquisition timeout for migration DDL.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum time a statement may wait for a table lock.
func uuidLockTimeout() time.Duration {
	return config.ExternalUUIDBackfillLockTimeout
}

// uuidDDLTimeout returns the bounded statement timeout for migration DDL.
// Parameters: none.
//
// Return values:
//   - time.Duration: maximum duration of one DDL statement.
func uuidDDLTimeout() time.Duration {
	return config.ExternalUUIDBackfillDDLTimeout
}

// uuidBlockingDDLAllowed reports whether the operator approved blocking DDL.
// It defaults to false. MySQL never falls back to a blocking ALTER without it; without the
// approval a MySQL server that cannot do LOCK=NONE keeps failing finalization retryably.
// Parameters: none.
//
// Return values:
//   - bool: true when blocking DDL is explicitly permitted.
func uuidBlockingDDLAllowed() bool {
	return config.ExternalUUIDBackfillAllowBlockingDDL
}

// uuidAutoFinalizeEnabled reports whether the background worker may finalize automatically
// after sustained quiescence. It defaults to true so a default deployment runs and completes
// the whole migration with no operator flag at all.
// Parameters: none.
//
// Return values:
//   - bool: true when automatic completion is enabled.
func uuidAutoFinalizeEnabled() bool {
	return config.ExternalUUIDBackfillAutoFinalize
}

// uuidAutoFinalizeIdlePasses returns how many consecutive no-work catch-up passes must be
// observed before automatic finalization is attempted.
// Parameters: none.
//
// Return values:
//   - int: quiescence threshold in idle passes.
func uuidAutoFinalizeIdlePasses() int {
	return config.ExternalUUIDBackfillAutoFinalizeIdlePasses
}
