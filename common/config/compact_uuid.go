// Package config provides centralized configuration management for one-api.
//
// This file defines the automatic compact UUID storage settings consumed by the
// background compact migration coordinator, its DDL, and its per-process health
// monitor. The settings are documented in
// docs/proposals/20260715_compact-uuid-storage.md section 8.7 (work item
// AUTO-011).
//
// Compact storage is deliberately zero-touch: every setting has a default that
// satisfies automatic acceptance, and there is no mode, cutover, or finalizer
// variable. COMPACT_UUID_AUTO_MIGRATE exists only as an emergency pause.
//
// As with the external UUID backfill settings, values are parsed strictly rather
// than with the silently-defaulting env helpers: the proposal requires invalid
// configuration to fail startup before the worker is created.

package config

import (
	"fmt"
	"time"

	"github.com/Laisky/errors/v2"
)

// =============================================================================
// COMPACT UUID STORAGE: ENVIRONMENT VARIABLE NAMES
// =============================================================================

const (
	// EnvCompactUUIDAutoMigrate names the emergency-pause variable.
	EnvCompactUUIDAutoMigrate = "COMPACT_UUID_AUTO_MIGRATE"
	// EnvCompactUUIDBatchSize names the per-statement row batch variable.
	EnvCompactUUIDBatchSize = "COMPACT_UUID_BATCH_SIZE"
	// EnvCompactUUIDMaxRowsPerCycle names the per-cycle row budget variable.
	EnvCompactUUIDMaxRowsPerCycle = "COMPACT_UUID_MAX_ROWS_PER_CYCLE"
	// EnvCompactUUIDMaxCycleDuration names the per-cycle row-reconciliation deadline variable.
	EnvCompactUUIDMaxCycleDuration = "COMPACT_UUID_MAX_CYCLE_DURATION"
	// EnvCompactUUIDActiveInterval names the active-backlog reschedule variable.
	EnvCompactUUIDActiveInterval = "COMPACT_UUID_ACTIVE_INTERVAL"
	// EnvCompactUUIDIdleInterval names the audit/no-work reschedule variable.
	EnvCompactUUIDIdleInterval = "COMPACT_UUID_IDLE_INTERVAL"
	// EnvCompactUUIDRetryInterval names the transient-failure backoff base variable.
	EnvCompactUUIDRetryInterval = "COMPACT_UUID_RETRY_INTERVAL"
	// EnvCompactUUIDLockTimeout names the ownership lock-acquisition variable.
	EnvCompactUUIDLockTimeout = "COMPACT_UUID_LOCK_TIMEOUT"
	// EnvCompactUUIDDDLTimeout names the DDL statement timeout variable.
	EnvCompactUUIDDDLTimeout = "COMPACT_UUID_DDL_TIMEOUT"
	// EnvCompactUUIDValidationTimeout names the global validation timeout variable.
	EnvCompactUUIDValidationTimeout = "COMPACT_UUID_VALIDATION_TIMEOUT"
)

// =============================================================================
// COMPACT UUID STORAGE: DEFAULTS AND RANGES
// =============================================================================

const (
	// DefaultCompactUUIDAutoMigrate is the default automatic-migration policy. Automatic
	// migration is enabled by default because unset configuration must pass zero-touch
	// acceptance; false is an emergency pause only.
	DefaultCompactUUIDAutoMigrate = true

	// DefaultCompactUUIDBatchSize is the default per-statement row batch.
	DefaultCompactUUIDBatchSize = 1000
	// MinCompactUUIDBatchSize is the smallest accepted batch size.
	MinCompactUUIDBatchSize = 1
	// MaxCompactUUIDBatchSize is the largest accepted batch size. It equals the proposal's
	// hard ceiling on materialized rows per query.
	MaxCompactUUIDBatchSize = 1000

	// DefaultCompactUUIDMaxRowsPerCycle is the default per-cycle row budget.
	DefaultCompactUUIDMaxRowsPerCycle = 10000
	// MinCompactUUIDMaxRowsPerCycle is the smallest accepted row budget.
	MinCompactUUIDMaxRowsPerCycle = 1000
	// MaxCompactUUIDMaxRowsPerCycle is the largest accepted row budget.
	MaxCompactUUIDMaxRowsPerCycle = 1000000

	// DefaultCompactUUIDMaxCycleDuration is the default row-reconciliation deadline. DDL and
	// full validation run outside this budget under their own timeouts.
	DefaultCompactUUIDMaxCycleDuration = 30 * time.Second
	// MinCompactUUIDMaxCycleDuration is the shortest accepted cycle deadline.
	MinCompactUUIDMaxCycleDuration = time.Second
	// MaxCompactUUIDMaxCycleDuration is the longest accepted cycle deadline.
	MaxCompactUUIDMaxCycleDuration = 30 * time.Minute

	// DefaultCompactUUIDActiveInterval is the default active-backlog delay.
	DefaultCompactUUIDActiveInterval = 5 * time.Second
	// MinCompactUUIDActiveInterval is the shortest accepted active delay.
	MinCompactUUIDActiveInterval = time.Second
	// MaxCompactUUIDActiveInterval is the longest accepted active delay.
	MaxCompactUUIDActiveInterval = 5 * time.Minute

	// DefaultCompactUUIDIdleInterval is the default no-work and audit recheck delay. It also
	// sets the health expiry window, which is twice this interval.
	DefaultCompactUUIDIdleInterval = 5 * time.Minute
	// MinCompactUUIDIdleInterval is the shortest accepted idle delay.
	MinCompactUUIDIdleInterval = 5 * time.Second
	// MaxCompactUUIDIdleInterval is the longest accepted idle delay.
	MaxCompactUUIDIdleInterval = time.Hour

	// DefaultCompactUUIDRetryInterval is the default base delay for exponential backoff.
	DefaultCompactUUIDRetryInterval = 30 * time.Second
	// MinCompactUUIDRetryInterval is the shortest accepted retry base.
	MinCompactUUIDRetryInterval = time.Second
	// MaxCompactUUIDRetryInterval is the longest accepted retry base.
	MaxCompactUUIDRetryInterval = 30 * time.Minute

	// DefaultCompactUUIDLockTimeout is the default ownership lock-acquisition timeout.
	DefaultCompactUUIDLockTimeout = 5 * time.Second
	// MinCompactUUIDLockTimeout is the shortest accepted lock timeout.
	MinCompactUUIDLockTimeout = time.Second
	// MaxCompactUUIDLockTimeout is the longest accepted lock timeout. The proposal caps lock
	// acquisition at five seconds, so this range is deliberately narrow.
	MaxCompactUUIDLockTimeout = 5 * time.Second

	// DefaultCompactUUIDDDLTimeout is the default DDL statement timeout.
	DefaultCompactUUIDDDLTimeout = 30 * time.Minute
	// MinCompactUUIDDDLTimeout is the shortest accepted DDL timeout.
	MinCompactUUIDDDLTimeout = time.Minute
	// MaxCompactUUIDDDLTimeout is the longest accepted DDL timeout.
	MaxCompactUUIDDDLTimeout = 24 * time.Hour

	// DefaultCompactUUIDValidationTimeout is the default global validation timeout.
	DefaultCompactUUIDValidationTimeout = 2 * time.Hour
	// MinCompactUUIDValidationTimeout is the shortest accepted validation timeout.
	MinCompactUUIDValidationTimeout = time.Minute
	// MaxCompactUUIDValidationTimeout is the longest accepted validation timeout.
	MaxCompactUUIDValidationTimeout = 24 * time.Hour
)

// =============================================================================
// COMPACT UUID STORAGE: SETTINGS
// =============================================================================
// Values are populated by LoadCompactUUIDSettings during package initialization.
// Each variable holds its documented default until a successful load replaces it,
// so a package that never calls the loader still observes a usable configuration.

var (
	// CompactUUIDAutoMigrate enables the automatic compact migration worker. It defaults to
	// true so an ordinary deployment expands, backfills, indexes, validates, and completes
	// with no operator action. Setting it to false is an emergency pause: it mutates no
	// schema, data, or markers and leaves legacy service fully functional, but a paused
	// deployment cannot satisfy automatic-completion acceptance.
	//
	// Environment variable: COMPACT_UUID_AUTO_MIGRATE
	// Default: true
	CompactUUIDAutoMigrate = DefaultCompactUUIDAutoMigrate

	// CompactUUIDBatchSize bounds how many rows one repair statement may carry. The
	// coordinator further reduces it so no statement exceeds 900 binds.
	//
	// Environment variable: COMPACT_UUID_BATCH_SIZE
	// Default: 1000
	// Range: 1..1000 (inclusive)
	CompactUUIDBatchSize = DefaultCompactUUIDBatchSize

	// CompactUUIDMaxRowsPerCycle bounds the target rows one cycle may examine, counted
	// globally across roles, tables, and columns.
	//
	// Environment variable: COMPACT_UUID_MAX_ROWS_PER_CYCLE
	// Default: 10000
	// Range: 1000..1000000 (inclusive)
	CompactUUIDMaxRowsPerCycle = DefaultCompactUUIDMaxRowsPerCycle

	// CompactUUIDMaxCycleDuration bounds the wall-clock duration of one cycle's row
	// reconciliation. DDL and full validation deliberately run outside this budget under
	// their own timeouts so neither is silently truncated.
	//
	// Environment variable: COMPACT_UUID_MAX_CYCLE_DURATION
	// Default: 30s
	// Range: 1s..30m (inclusive)
	CompactUUIDMaxCycleDuration = DefaultCompactUUIDMaxCycleDuration

	// CompactUUIDActiveInterval delays the next cycle after a cycle that observed backlog.
	//
	// Environment variable: COMPACT_UUID_ACTIVE_INTERVAL
	// Default: 5s
	// Range: 1s..5m (inclusive)
	CompactUUIDActiveInterval = DefaultCompactUUIDActiveInterval

	// CompactUUIDIdleInterval delays the next cycle after a no-work pass and sets the
	// per-process audit cadence. Health expires after twice this interval.
	//
	// Environment variable: COMPACT_UUID_IDLE_INTERVAL
	// Default: 5m
	// Range: 5s..1h (inclusive)
	CompactUUIDIdleInterval = DefaultCompactUUIDIdleInterval

	// CompactUUIDRetryInterval is the base delay for the bounded full-jitter backoff applied
	// after a transient database, lock, or DDL failure.
	//
	// Environment variable: COMPACT_UUID_RETRY_INTERVAL
	// Default: 30s
	// Range: 1s..30m (inclusive)
	CompactUUIDRetryInterval = DefaultCompactUUIDRetryInterval

	// CompactUUIDLockTimeout bounds ownership lock acquisition and compact DDL lock waits.
	//
	// Environment variable: COMPACT_UUID_LOCK_TIMEOUT
	// Default: 5s
	// Range: 1s..5s (inclusive)
	CompactUUIDLockTimeout = DefaultCompactUUIDLockTimeout

	// CompactUUIDDDLTimeout bounds the execution of one compact DDL statement.
	//
	// Environment variable: COMPACT_UUID_DDL_TIMEOUT
	// Default: 30m
	// Range: 1m..24h (inclusive)
	CompactUUIDDDLTimeout = DefaultCompactUUIDDDLTimeout

	// CompactUUIDValidationTimeout bounds one global validation or fingerprint pass.
	//
	// Environment variable: COMPACT_UUID_VALIDATION_TIMEOUT
	// Default: 2h
	// Range: 1m..24h (inclusive)
	CompactUUIDValidationTimeout = DefaultCompactUUIDValidationTimeout
)

// =============================================================================
// COMPACT UUID STORAGE: LOADER
// =============================================================================

// LoadCompactUUIDSettings reads, parses, and validates every compact UUID storage
// environment variable, then publishes the results to the package level settings.
// An unset variable uses its documented default and never fails.
//
// The load is all-or-nothing: every value is parsed and range-checked before any
// package variable is assigned, so a rejected configuration cannot leave the
// process running on partially applied settings. The function never terminates
// the process; callers decide how a terminal configuration error is reported.
//
// Parameters: none.
//
// Return values:
//   - error: wrapped validation error naming the offending environment variable,
//     its value, and the permitted range; nil when every setting is valid.
func LoadCompactUUIDSettings() error {
	autoMigrate, err := loadValidatedBool(EnvCompactUUIDAutoMigrate, DefaultCompactUUIDAutoMigrate)
	if err != nil {
		return errors.WithStack(err)
	}

	batchSize, err := loadValidatedInt(
		EnvCompactUUIDBatchSize,
		DefaultCompactUUIDBatchSize,
		MinCompactUUIDBatchSize,
		MaxCompactUUIDBatchSize,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	maxRowsPerCycle, err := loadValidatedInt(
		EnvCompactUUIDMaxRowsPerCycle,
		DefaultCompactUUIDMaxRowsPerCycle,
		MinCompactUUIDMaxRowsPerCycle,
		MaxCompactUUIDMaxRowsPerCycle,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	maxCycleDuration, err := loadValidatedDuration(
		EnvCompactUUIDMaxCycleDuration,
		DefaultCompactUUIDMaxCycleDuration,
		MinCompactUUIDMaxCycleDuration,
		MaxCompactUUIDMaxCycleDuration,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	activeInterval, err := loadValidatedDuration(
		EnvCompactUUIDActiveInterval,
		DefaultCompactUUIDActiveInterval,
		MinCompactUUIDActiveInterval,
		MaxCompactUUIDActiveInterval,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	idleInterval, err := loadValidatedDuration(
		EnvCompactUUIDIdleInterval,
		DefaultCompactUUIDIdleInterval,
		MinCompactUUIDIdleInterval,
		MaxCompactUUIDIdleInterval,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	retryInterval, err := loadValidatedDuration(
		EnvCompactUUIDRetryInterval,
		DefaultCompactUUIDRetryInterval,
		MinCompactUUIDRetryInterval,
		MaxCompactUUIDRetryInterval,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	lockTimeout, err := loadValidatedDuration(
		EnvCompactUUIDLockTimeout,
		DefaultCompactUUIDLockTimeout,
		MinCompactUUIDLockTimeout,
		MaxCompactUUIDLockTimeout,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	ddlTimeout, err := loadValidatedDuration(
		EnvCompactUUIDDDLTimeout,
		DefaultCompactUUIDDDLTimeout,
		MinCompactUUIDDDLTimeout,
		MaxCompactUUIDDDLTimeout,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	validationTimeout, err := loadValidatedDuration(
		EnvCompactUUIDValidationTimeout,
		DefaultCompactUUIDValidationTimeout,
		MinCompactUUIDValidationTimeout,
		MaxCompactUUIDValidationTimeout,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	CompactUUIDAutoMigrate = autoMigrate
	CompactUUIDBatchSize = batchSize
	CompactUUIDMaxRowsPerCycle = maxRowsPerCycle
	CompactUUIDMaxCycleDuration = maxCycleDuration
	CompactUUIDActiveInterval = activeInterval
	CompactUUIDIdleInterval = idleInterval
	CompactUUIDRetryInterval = retryInterval
	CompactUUIDLockTimeout = lockTimeout
	CompactUUIDDDLTimeout = ddlTimeout
	CompactUUIDValidationTimeout = validationTimeout

	return nil
}

// MustLoadCompactUUIDSettings loads the compact UUID storage settings and panics
// when the configuration is invalid. It mirrors MustLoadExternalUUIDBackfillSettings
// so package initialization fails fast, before any worker is created.
//
// Parameters: none.
//
// Return values: none.
func MustLoadCompactUUIDSettings() {
	if err := LoadCompactUUIDSettings(); err != nil {
		panic(fmt.Sprintf("configuration validation failed:\n  - %s", err.Error()))
	}
}
