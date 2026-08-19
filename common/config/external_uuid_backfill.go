// Package config provides centralized configuration management for one-api.
//
// This file defines the external UUID backfill settings consumed by the
// background catch-up worker and by finalizer-mode DDL. The settings are
// documented in docs/proposals/20260715_incremental-uuid-backfill.md sections
// 6.7 and 6.8 (work items UUID-020 and UUID-042).
//
// Unlike most settings in config.go, these values are not read with the
// silently-defaulting env helpers. The proposal requires that invalid settings
// fail configuration loading, so every value is parsed strictly and validated
// against an inclusive range. An unset variable always uses its default.
//
// Durations accept Go duration strings ("30s", "5m", "1h"), matching the
// defaults quoted by the proposal, rather than the bare-seconds integers used
// by the older CHANNEL_SUSPEND_SECONDS_* variables whose names carry the unit.

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// =============================================================================
// EXTERNAL UUID BACKFILL: ENVIRONMENT VARIABLE NAMES
// =============================================================================

const (
	// EnvExternalUUIDBackfillMaxRowsPerCycle names the per-cycle row budget variable.
	EnvExternalUUIDBackfillMaxRowsPerCycle = "EXTERNAL_UUID_BACKFILL_MAX_ROWS_PER_CYCLE"
	// EnvExternalUUIDBackfillMaxCycleDuration names the per-cycle deadline variable.
	EnvExternalUUIDBackfillMaxCycleDuration = "EXTERNAL_UUID_BACKFILL_MAX_CYCLE_DURATION"
	// EnvExternalUUIDBackfillActiveInterval names the active-backlog reschedule variable.
	EnvExternalUUIDBackfillActiveInterval = "EXTERNAL_UUID_BACKFILL_ACTIVE_INTERVAL"
	// EnvExternalUUIDBackfillIdleInterval names the no-work reschedule variable.
	EnvExternalUUIDBackfillIdleInterval = "EXTERNAL_UUID_BACKFILL_IDLE_INTERVAL"
	// EnvExternalUUIDBackfillLockTimeout names the finalizer lock-acquisition variable.
	EnvExternalUUIDBackfillLockTimeout = "EXTERNAL_UUID_BACKFILL_LOCK_TIMEOUT"
	// EnvExternalUUIDBackfillDDLTimeout names the finalizer DDL statement timeout variable.
	EnvExternalUUIDBackfillDDLTimeout = "EXTERNAL_UUID_BACKFILL_DDL_TIMEOUT"
	// EnvExternalUUIDBackfillAllowBlockingDDL names the blocking-DDL opt-in variable.
	EnvExternalUUIDBackfillAllowBlockingDDL = "EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL"
	// EnvExternalUUIDBackfillAutoFinalize names the automatic-completion variable.
	EnvExternalUUIDBackfillAutoFinalize = "EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE"
	// EnvExternalUUIDBackfillAutoFinalizeIdlePasses names the quiescence threshold variable.
	EnvExternalUUIDBackfillAutoFinalizeIdlePasses = "EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE_IDLE_PASSES"
)

// =============================================================================
// EXTERNAL UUID BACKFILL: DEFAULTS AND RANGES
// =============================================================================

const (
	// DefaultExternalUUIDBackfillMaxRowsPerCycle is the default per-cycle row budget.
	DefaultExternalUUIDBackfillMaxRowsPerCycle = 10000
	// MinExternalUUIDBackfillMaxRowsPerCycle is the smallest accepted row budget.
	MinExternalUUIDBackfillMaxRowsPerCycle = 1000
	// MaxExternalUUIDBackfillMaxRowsPerCycle is the largest accepted row budget.
	MaxExternalUUIDBackfillMaxRowsPerCycle = 1000000

	// DefaultExternalUUIDBackfillMaxCycleDuration is the default per-cycle deadline.
	DefaultExternalUUIDBackfillMaxCycleDuration = 30 * time.Second
	// MinExternalUUIDBackfillMaxCycleDuration is the shortest accepted cycle deadline.
	MinExternalUUIDBackfillMaxCycleDuration = time.Second
	// MaxExternalUUIDBackfillMaxCycleDuration is the longest accepted cycle deadline.
	MaxExternalUUIDBackfillMaxCycleDuration = 30 * time.Minute

	// DefaultExternalUUIDBackfillActiveInterval is the default active-backlog delay.
	DefaultExternalUUIDBackfillActiveInterval = 5 * time.Second
	// MinExternalUUIDBackfillActiveInterval is the shortest accepted active delay.
	// Zero is permitted so an operator can drain a backlog without pausing.
	MinExternalUUIDBackfillActiveInterval = time.Duration(0)
	// MaxExternalUUIDBackfillActiveInterval is the longest accepted active delay.
	MaxExternalUUIDBackfillActiveInterval = 5 * time.Minute

	// DefaultExternalUUIDBackfillIdleInterval is the default no-work recheck delay.
	DefaultExternalUUIDBackfillIdleInterval = 5 * time.Minute
	// MinExternalUUIDBackfillIdleInterval is the shortest accepted idle delay.
	MinExternalUUIDBackfillIdleInterval = 5 * time.Second
	// MaxExternalUUIDBackfillIdleInterval is the longest accepted idle delay.
	MaxExternalUUIDBackfillIdleInterval = time.Hour

	// DefaultExternalUUIDBackfillLockTimeout is the default lock-acquisition timeout.
	DefaultExternalUUIDBackfillLockTimeout = 5 * time.Second
	// MinExternalUUIDBackfillLockTimeout is the shortest accepted lock timeout.
	MinExternalUUIDBackfillLockTimeout = time.Second
	// MaxExternalUUIDBackfillLockTimeout is the longest accepted lock timeout.
	MaxExternalUUIDBackfillLockTimeout = 5 * time.Minute

	// DefaultExternalUUIDBackfillDDLTimeout is the default DDL statement timeout.
	DefaultExternalUUIDBackfillDDLTimeout = 30 * time.Minute
	// MinExternalUUIDBackfillDDLTimeout is the shortest accepted DDL timeout.
	MinExternalUUIDBackfillDDLTimeout = time.Minute
	// MaxExternalUUIDBackfillDDLTimeout is the longest accepted DDL timeout.
	MaxExternalUUIDBackfillDDLTimeout = 24 * time.Hour

	// DefaultExternalUUIDBackfillAllowBlockingDDL is the default blocking-DDL policy.
	DefaultExternalUUIDBackfillAllowBlockingDDL = false

	// DefaultExternalUUIDBackfillAutoFinalize is the default automatic-completion policy.
	// The migration must run and complete automatically on a default deployment, so this
	// defaults to enabled.
	DefaultExternalUUIDBackfillAutoFinalize = true

	// DefaultExternalUUIDBackfillAutoFinalizeIdlePasses is how many consecutive no-work
	// catch-up passes the background worker must observe before it finalizes automatically.
	// With the default five-minute idle interval this is roughly fifteen minutes of observed
	// quiescence, standing in for the drained mixed-writer window on a default deployment.
	DefaultExternalUUIDBackfillAutoFinalizeIdlePasses = 3
	// MinExternalUUIDBackfillAutoFinalizeIdlePasses is the smallest accepted threshold.
	MinExternalUUIDBackfillAutoFinalizeIdlePasses = 1
	// MaxExternalUUIDBackfillAutoFinalizeIdlePasses is the largest accepted threshold.
	MaxExternalUUIDBackfillAutoFinalizeIdlePasses = 1000
)

// =============================================================================
// EXTERNAL UUID BACKFILL: SETTINGS
// =============================================================================
// Values are populated by LoadExternalUUIDBackfillSettings during package
// initialization. Each variable holds its documented default until a successful
// load replaces it, so a package that never calls the loader still observes a
// usable configuration.

var (
	// ExternalUUIDBackfillMaxRowsPerCycle bounds the target rows one catch-up cycle
	// may examine, counted globally across roles, phases, tables, and columns.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_MAX_ROWS_PER_CYCLE
	// Default: 10000
	// Range: 1000..1000000 (inclusive)
	ExternalUUIDBackfillMaxRowsPerCycle = DefaultExternalUUIDBackfillMaxRowsPerCycle

	// ExternalUUIDBackfillMaxCycleDuration bounds the wall-clock duration of one
	// catch-up cycle. It is applied as a context deadline checked by every query,
	// update, and inter-batch transition.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_MAX_CYCLE_DURATION
	// Default: 30s
	// Range: 1s..30m (inclusive)
	ExternalUUIDBackfillMaxCycleDuration = DefaultExternalUUIDBackfillMaxCycleDuration

	// ExternalUUIDBackfillActiveInterval delays the next catch-up cycle after a
	// cycle that still observed backlog.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_ACTIVE_INTERVAL
	// Default: 5s
	// Range: 0s..5m (inclusive)
	ExternalUUIDBackfillActiveInterval = DefaultExternalUUIDBackfillActiveInterval

	// ExternalUUIDBackfillIdleInterval delays the next catch-up cycle after a full
	// no-work pass during the mixed-writer window.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_IDLE_INTERVAL
	// Default: 5m
	// Range: 5s..1h (inclusive)
	ExternalUUIDBackfillIdleInterval = DefaultExternalUUIDBackfillIdleInterval

	// ExternalUUIDBackfillLockTimeout bounds how long finalizer DDL waits to acquire
	// a table lock before returning a retryable failure.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_LOCK_TIMEOUT
	// Default: 5s
	// Range: 1s..5m (inclusive)
	ExternalUUIDBackfillLockTimeout = DefaultExternalUUIDBackfillLockTimeout

	// ExternalUUIDBackfillDDLTimeout bounds the execution of one finalizer DDL
	// statement, such as unique-index promotion.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_DDL_TIMEOUT
	// Default: 30m
	// Range: 1m..24h (inclusive)
	ExternalUUIDBackfillDDLTimeout = DefaultExternalUUIDBackfillDDLTimeout

	// ExternalUUIDBackfillAllowBlockingDDL permits blocking MySQL DDL during
	// finalization. It must only be enabled inside an operator-approved maintenance
	// window; otherwise MySQL finalization fails without a blocking fallback and is
	// retried later.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL
	// Default: false
	ExternalUUIDBackfillAllowBlockingDDL = DefaultExternalUUIDBackfillAllowBlockingDDL

	// ExternalUUIDBackfillAutoFinalize lets the background worker finalize the
	// migration automatically once catch-up has observed sustained quiescence, so a
	// default deployment completes without any operator flag. Disabling it restores
	// the operator-driven lifecycle where only EXTERNAL_UUID_BACKFILL_FINALIZER
	// finalizes.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE
	// Default: true
	ExternalUUIDBackfillAutoFinalize = DefaultExternalUUIDBackfillAutoFinalize

	// ExternalUUIDBackfillAutoFinalizeIdlePasses is how many consecutive no-work
	// catch-up passes must be observed before automatic finalization is attempted.
	//
	// Environment variable: EXTERNAL_UUID_BACKFILL_AUTO_FINALIZE_IDLE_PASSES
	// Default: 3
	// Range: 1..1000 (inclusive)
	ExternalUUIDBackfillAutoFinalizeIdlePasses = DefaultExternalUUIDBackfillAutoFinalizeIdlePasses
)

// =============================================================================
// EXTERNAL UUID BACKFILL: LOADER
// =============================================================================

// LoadExternalUUIDBackfillSettings reads, parses, and validates every external
// UUID backfill environment variable, then publishes the results to the package
// level settings. An unset variable uses its documented default and never fails.
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
func LoadExternalUUIDBackfillSettings() error {
	maxRowsPerCycle, err := loadValidatedInt(
		EnvExternalUUIDBackfillMaxRowsPerCycle,
		DefaultExternalUUIDBackfillMaxRowsPerCycle,
		MinExternalUUIDBackfillMaxRowsPerCycle,
		MaxExternalUUIDBackfillMaxRowsPerCycle,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	maxCycleDuration, err := loadValidatedDuration(
		EnvExternalUUIDBackfillMaxCycleDuration,
		DefaultExternalUUIDBackfillMaxCycleDuration,
		MinExternalUUIDBackfillMaxCycleDuration,
		MaxExternalUUIDBackfillMaxCycleDuration,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	activeInterval, err := loadValidatedDuration(
		EnvExternalUUIDBackfillActiveInterval,
		DefaultExternalUUIDBackfillActiveInterval,
		MinExternalUUIDBackfillActiveInterval,
		MaxExternalUUIDBackfillActiveInterval,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	idleInterval, err := loadValidatedDuration(
		EnvExternalUUIDBackfillIdleInterval,
		DefaultExternalUUIDBackfillIdleInterval,
		MinExternalUUIDBackfillIdleInterval,
		MaxExternalUUIDBackfillIdleInterval,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	lockTimeout, err := loadValidatedDuration(
		EnvExternalUUIDBackfillLockTimeout,
		DefaultExternalUUIDBackfillLockTimeout,
		MinExternalUUIDBackfillLockTimeout,
		MaxExternalUUIDBackfillLockTimeout,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	ddlTimeout, err := loadValidatedDuration(
		EnvExternalUUIDBackfillDDLTimeout,
		DefaultExternalUUIDBackfillDDLTimeout,
		MinExternalUUIDBackfillDDLTimeout,
		MaxExternalUUIDBackfillDDLTimeout,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	allowBlockingDDL, err := loadValidatedBool(
		EnvExternalUUIDBackfillAllowBlockingDDL,
		DefaultExternalUUIDBackfillAllowBlockingDDL,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	autoFinalize, err := loadValidatedBool(
		EnvExternalUUIDBackfillAutoFinalize,
		DefaultExternalUUIDBackfillAutoFinalize,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	autoFinalizeIdlePasses, err := loadValidatedInt(
		EnvExternalUUIDBackfillAutoFinalizeIdlePasses,
		DefaultExternalUUIDBackfillAutoFinalizeIdlePasses,
		MinExternalUUIDBackfillAutoFinalizeIdlePasses,
		MaxExternalUUIDBackfillAutoFinalizeIdlePasses,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	ExternalUUIDBackfillMaxRowsPerCycle = maxRowsPerCycle
	ExternalUUIDBackfillMaxCycleDuration = maxCycleDuration
	ExternalUUIDBackfillActiveInterval = activeInterval
	ExternalUUIDBackfillIdleInterval = idleInterval
	ExternalUUIDBackfillLockTimeout = lockTimeout
	ExternalUUIDBackfillDDLTimeout = ddlTimeout
	ExternalUUIDBackfillAllowBlockingDDL = allowBlockingDDL
	ExternalUUIDBackfillAutoFinalize = autoFinalize
	ExternalUUIDBackfillAutoFinalizeIdlePasses = autoFinalizeIdlePasses

	return nil
}

// MustLoadExternalUUIDBackfillSettings loads the external UUID backfill settings
// and panics when the configuration is invalid. It mirrors MustValidateEnvVars so
// package initialization fails fast on misconfiguration.
//
// Parameters: none.
//
// Return values: none.
func MustLoadExternalUUIDBackfillSettings() {
	if err := LoadExternalUUIDBackfillSettings(); err != nil {
		panic(fmt.Sprintf("configuration validation failed:\n  - %s", err.Error()))
	}
}

// =============================================================================
// EXTERNAL UUID BACKFILL: STRICT ENV PARSERS
// =============================================================================
// These helpers deliberately reject malformed input instead of falling back to a
// default the way common/env does, because the proposal requires invalid backfill
// settings to fail configuration loading rather than run with a silent default.

// loadValidatedInt reads an integer environment variable and validates it against
// an inclusive range.
//
// Parameters:
//   - name: environment variable name.
//   - defaultValue: value used when the variable is unset or empty.
//   - minValue: smallest accepted value, inclusive.
//   - maxValue: largest accepted value, inclusive.
//
// Return values:
//   - int: validated value.
//   - error: wrapped error naming the variable, its value, and the range.
func loadValidatedInt(name string, defaultValue, minValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "parse %s value %q: must be an integer between %d and %d (inclusive)",
			name, raw, minValue, maxValue)
	}

	if value < minValue || value > maxValue {
		return 0, errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      value,
			Constraint: fmt.Sprintf("must be between %d and %d (inclusive)", minValue, maxValue),
		})
	}

	return value, nil
}

// loadValidatedDuration reads a Go duration environment variable ("30s", "5m",
// "1h") and validates it against an inclusive range.
//
// Parameters:
//   - name: environment variable name.
//   - defaultValue: value used when the variable is unset or empty.
//   - minValue: shortest accepted duration, inclusive.
//   - maxValue: longest accepted duration, inclusive.
//
// Return values:
//   - time.Duration: validated value.
//   - error: wrapped error naming the variable, its value, and the range.
func loadValidatedDuration(name string, defaultValue, minValue, maxValue time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "parse %s value %q: must be a Go duration such as %q, between %s and %s (inclusive)",
			name, raw, defaultValue.String(), minValue, maxValue)
	}

	if value < minValue || value > maxValue {
		return 0, errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      value.String(),
			Constraint: fmt.Sprintf("must be a Go duration between %s and %s (inclusive)", minValue, maxValue),
		})
	}

	return value, nil
}

// loadValidatedBool reads a boolean environment variable, rejecting values that
// Go cannot parse so an operator typo cannot silently disable a setting.
//
// Parameters:
//   - name: environment variable name.
//   - defaultValue: value used when the variable is unset or empty.
//
// Return values:
//   - bool: validated value.
//   - error: wrapped error naming the variable, its value, and the accepted values.
func loadValidatedBool(name string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errors.Wrapf(err, "parse %s value %q: must be a boolean (true or false)", name, raw)
	}

	return value, nil
}
