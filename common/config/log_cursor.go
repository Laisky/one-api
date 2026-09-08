// Package config provides centralized configuration management for one-api.
//
// This file holds the settings for the additive keyset log-list capability
// (docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// Every setting here governs NEW routes only. The legacy offset endpoints keep
// their pagination, filters, sorts and exact `total` semantics unchanged, so no
// value in this file can alter what an existing client sees.

package config

import (
	"time"

	"github.com/Laisky/one-api/common/env"
)

var (
	// LogCursorEnabled controls whether the additive cursor routes answer.
	//
	// It is the rollback lever: turning it off makes both routes report the
	// capability as unavailable and leaves the legacy routes untouched.
	//
	// It defaults to OFF because the keyset order (created_at DESC, id DESC)
	// needs an access path the shipped schema does not have. Measured on a
	// 2,000,000-row corpus in docs/benchmarks/20260906_w24-cursor-plans.md,
	// MySQL 8.4 answers the very first cursor page with a full table scan and a
	// filesort of every row — 5.9 s, against 0.8 ms for the legacy offset page
	// it would replace. Enabling this by default would make the log page
	// dramatically slower for existing MySQL deployments, which is the opposite
	// of the intent. Enable it once W2.5 has established the access paths for
	// the target engine; Modern falls back to the legacy route when it is off,
	// so leaving it off changes nothing for anyone.
	//
	// Environment variable: LOG_CURSOR_ENABLED
	// Default: false
	LogCursorEnabled = env.Bool("LOG_CURSOR_ENABLED", false)

	// LogCursorTTLSec is how long an issued cursor remains usable.
	//
	// Environment variable: LOG_CURSOR_TTL_SEC
	// Default: 1800
	// Range: [60, 86400]
	// Unit: seconds
	LogCursorTTLSec = env.Int("LOG_CURSOR_TTL_SEC", 1800)

	// LogCursorMaxResponseBytes caps the encoded size of one cursor page.
	//
	// A page that would exceed it stops early and reports the truncation
	// explicitly; no field is ever silently shortened.
	//
	// Environment variable: LOG_CURSOR_MAX_RESPONSE_BYTES
	// Default: 4194304 (4 MiB); 0 disables the cap
	// Unit: bytes
	LogCursorMaxResponseBytes = env.Int("LOG_CURSOR_MAX_RESPONSE_BYTES", 4<<20)

	// LogCountProbeMaxRows is how many matching rows the bounded count probe
	// examines before reporting a lower bound instead of an exact value.
	//
	// Environment variable: LOG_COUNT_PROBE_MAX_ROWS
	// Default: 10000
	LogCountProbeMaxRows = env.Int("LOG_COUNT_PROBE_MAX_ROWS", 10000)

	// LogCountExactMaxRows bounds an explicitly requested exact count.
	//
	// Environment variable: LOG_COUNT_EXACT_MAX_ROWS
	// Default: 100000
	LogCountExactMaxRows = env.Int("LOG_COUNT_EXACT_MAX_ROWS", 100000)

	// LogCountProbeTimeoutMs bounds the bounded probe's database time.
	//
	// Exceeding it yields a count of quality "unavailable", which is a
	// first-class outcome rather than an error: the probe bounds the number of
	// MATCHING rows, not the number of rows examined, so a selective filter
	// over deep history can legitimately run out of budget.
	//
	// Environment variable: LOG_COUNT_PROBE_TIMEOUT_MS
	// Default: 3000
	// Unit: milliseconds
	LogCountProbeTimeoutMs = env.Int("LOG_COUNT_PROBE_TIMEOUT_MS", 3000)

	// LogCountExactTimeoutMs bounds an explicitly requested exact count.
	//
	// Environment variable: LOG_COUNT_EXACT_TIMEOUT_MS
	// Default: 10000
	// Unit: milliseconds
	LogCountExactTimeoutMs = env.Int("LOG_COUNT_EXACT_TIMEOUT_MS", 10000)

	// LogCountProbeMaxConcurrent bounds concurrent bounded probes per node.
	//
	// Environment variable: LOG_COUNT_PROBE_MAX_CONCURRENT
	// Default: 4
	LogCountProbeMaxConcurrent = env.Int("LOG_COUNT_PROBE_MAX_CONCURRENT", 4)

	// LogCountExactMaxConcurrent bounds concurrent exact counts per node.
	//
	// Environment variable: LOG_COUNT_EXACT_MAX_CONCURRENT
	// Default: 1
	LogCountExactMaxConcurrent = env.Int("LOG_COUNT_EXACT_MAX_CONCURRENT", 1)

	// LogCountCacheTTLSec is how long a computed count stays reusable.
	//
	// A cached count is always published with the instant it was taken, so a
	// client can never mistake a minute-old exact count for a live one.
	//
	// Environment variable: LOG_COUNT_CACHE_TTL_SEC
	// Default: 30; 0 disables caching
	// Unit: seconds
	LogCountCacheTTLSec = env.Int("LOG_COUNT_CACHE_TTL_SEC", 30)
)

// LogCursorTTL returns the cursor lifetime as a duration.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the configured lifetime.
func LogCursorTTL() time.Duration {
	return time.Duration(LogCursorTTLSec) * time.Second
}

// LogCountProbeTimeout returns the bounded probe's time budget.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the configured budget.
func LogCountProbeTimeout() time.Duration {
	return time.Duration(LogCountProbeTimeoutMs) * time.Millisecond
}

// LogCountExactTimeout returns the exact count's time budget.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the configured budget.
func LogCountExactTimeout() time.Duration {
	return time.Duration(LogCountExactTimeoutMs) * time.Millisecond
}

// LogCountCacheTTL returns how long a computed count stays reusable.
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the configured TTL; zero disables caching.
func LogCountCacheTTL() time.Duration {
	return time.Duration(LogCountCacheTTLSec) * time.Second
}

// validateLogCursorSettings checks every cursor and count setting.
//
// These fail startup rather than silently falling back, because a silently
// clamped budget is a budget the operator believes they configured and did not.
//
// Parameters: none.
//
// Return values:
//   - []error: one entry per invalid setting; empty when all are valid.
func validateLogCursorSettings() []error {
	var errs []error

	if err := ValidateIntRange("LOG_CURSOR_TTL_SEC", LogCursorTTLSec, 60, 86400); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateNonNegativeInt("LOG_CURSOR_MAX_RESPONSE_BYTES", LogCursorMaxResponseBytes); err != nil {
		errs = append(errs, err)
	}
	if err := ValidatePositiveInt("LOG_COUNT_EXACT_MAX_ROWS", LogCountExactMaxRows); err != nil {
		errs = append(errs, err)
	}
	// The probe must be able to distinguish "at least one full page" from
	// "exact", and must never be asked to look further than the exact budget.
	if err := ValidateIntRange("LOG_COUNT_PROBE_MAX_ROWS", LogCountProbeMaxRows,
		MaxItemsPerPage+1, max(LogCountExactMaxRows, MaxItemsPerPage+1)); err != nil {
		errs = append(errs, err)
	}
	if err := ValidatePositiveInt("LOG_COUNT_PROBE_TIMEOUT_MS", LogCountProbeTimeoutMs); err != nil {
		errs = append(errs, err)
	}
	if err := ValidatePositiveInt("LOG_COUNT_EXACT_TIMEOUT_MS", LogCountExactTimeoutMs); err != nil {
		errs = append(errs, err)
	}
	if err := ValidatePositiveInt("LOG_COUNT_PROBE_MAX_CONCURRENT", LogCountProbeMaxConcurrent); err != nil {
		errs = append(errs, err)
	}
	if err := ValidatePositiveInt("LOG_COUNT_EXACT_MAX_CONCURRENT", LogCountExactMaxConcurrent); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateNonNegativeInt("LOG_COUNT_CACHE_TTL_SEC", LogCountCacheTTLSec); err != nil {
		errs = append(errs, err)
	}

	return errs
}
