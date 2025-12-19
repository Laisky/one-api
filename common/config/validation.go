// Package config provides centralized configuration management for one-api.
//
// This file contains validation functions for environment variables with
// value constraints. Validation is performed during package initialization
// to fail fast on misconfiguration.

package config

import (
	"fmt"
	"slices"
	"strings"
)

// =============================================================================
// VALIDATION ERROR TYPES
// =============================================================================
// Custom error types for configuration validation failures.

// ConfigValidationError represents a configuration validation failure.
type ConfigValidationError struct {
	Variable    string   // Environment variable name
	Value       any      // Current value
	Constraint  string   // Description of the constraint
	AllowedVals []string // Optional list of allowed values
}

// Error implements the error interface for ConfigValidationError.
func (e *ConfigValidationError) Error() string {
	if len(e.AllowedVals) > 0 {
		return fmt.Sprintf("invalid configuration for %s: got %v, %s (allowed: %v)",
			e.Variable, e.Value, e.Constraint, e.AllowedVals)
	}
	return fmt.Sprintf("invalid configuration for %s: got %v, %s",
		e.Variable, e.Value, e.Constraint)
}

// =============================================================================
// INDIVIDUAL VALIDATORS
// =============================================================================
// Functions that validate specific environment variables.

// ValidateGinMode validates the GIN_MODE environment variable.
// Allowed values: "debug", "release", "test", or empty string.
func ValidateGinMode(value string) error {
	if value == "" {
		return nil // Empty is valid (uses Gin default)
	}
	allowed := []string{"debug", "release", "test"}
	if !slices.Contains(allowed, value) {
		return &ConfigValidationError{
			Variable:    "GIN_MODE",
			Value:       value,
			Constraint:  "must be a valid Gin mode",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateAutoDetectAPIFormatAction validates AUTO_DETECT_API_FORMAT_ACTION.
// Allowed values: "transparent", "redirect".
func ValidateAutoDetectAPIFormatAction(value string) error {
	allowed := []string{"transparent", "redirect"}
	if !slices.Contains(allowed, value) {
		return &ConfigValidationError{
			Variable:    "AUTO_DETECT_API_FORMAT_ACTION",
			Value:       value,
			Constraint:  "must be a valid action",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateLogRotationInterval validates LOG_ROTATION_INTERVAL.
// Allowed values: "hourly", "daily", "weekly".
func ValidateLogRotationInterval(value string) error {
	allowed := []string{"hourly", "daily", "weekly"}
	if !slices.Contains(allowed, value) {
		return &ConfigValidationError{
			Variable:    "LOG_ROTATION_INTERVAL",
			Value:       value,
			Constraint:  "must be a valid rotation interval",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateTheme validates the THEME environment variable.
// Allowed values: "default", "berry", "air", "modern".
func ValidateTheme(value string) error {
	if !ValidThemes[value] {
		allowed := make([]string, 0, len(ValidThemes))
		for k := range ValidThemes {
			allowed = append(allowed, k)
		}
		return &ConfigValidationError{
			Variable:    "THEME",
			Value:       value,
			Constraint:  "must be a valid theme name",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateGeminiSafetySetting validates GEMINI_SAFETY_SETTING.
// Allowed values: "BLOCK_NONE", "BLOCK_LOW_AND_ABOVE", "BLOCK_MEDIUM_AND_ABOVE", "BLOCK_ONLY_HIGH".
func ValidateGeminiSafetySetting(value string) error {
	allowed := []string{
		"BLOCK_NONE",
		"BLOCK_LOW_AND_ABOVE",
		"BLOCK_MEDIUM_AND_ABOVE",
		"BLOCK_ONLY_HIGH",
	}
	if !slices.Contains(allowed, value) {
		return &ConfigValidationError{
			Variable:    "GEMINI_SAFETY_SETTING",
			Value:       value,
			Constraint:  "must be a valid Gemini safety setting",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateGeminiVersion validates GEMINI_VERSION.
// Allowed values: "v1", "v1beta".
func ValidateGeminiVersion(value string) error {
	allowed := []string{"v1", "v1beta"}
	if !slices.Contains(allowed, value) {
		return &ConfigValidationError{
			Variable:    "GEMINI_VERSION",
			Value:       value,
			Constraint:  "must be a valid Gemini API version",
			AllowedVals: allowed,
		}
	}
	return nil
}

// ValidateOpenTelemetryConfig ensures OTEL_EXPORTER_OTLP_ENDPOINT is provided when OpenTelemetry is enabled.
func ValidateOpenTelemetryConfig(enabled bool, endpoint string) error {
	if !enabled {
		return nil
	}

	if strings.TrimSpace(endpoint) == "" {
		return &ConfigValidationError{
			Variable:   "OTEL_EXPORTER_OTLP_ENDPOINT",
			Value:      endpoint,
			Constraint: "must be set when OTEL_ENABLED is true",
		}
	}

	return nil
}

// =============================================================================
// NUMERIC VALIDATORS
// =============================================================================
// Functions that validate numeric environment variables.

// ValidatePositiveInt validates that an integer value is positive (> 0).
func ValidatePositiveInt(varName string, value int) error {
	if value <= 0 {
		return &ConfigValidationError{
			Variable:   varName,
			Value:      value,
			Constraint: "must be a positive integer (> 0)",
		}
	}
	return nil
}

// ValidateNonNegativeInt validates that an integer value is non-negative (>= 0).
func ValidateNonNegativeInt(varName string, value int) error {
	if value < 0 {
		return &ConfigValidationError{
			Variable:   varName,
			Value:      value,
			Constraint: "must be a non-negative integer (>= 0)",
		}
	}
	return nil
}

// ValidateIntRange validates that an integer is within a specified range [min, max].
func ValidateIntRange(varName string, value, min, max int) error {
	if value < min || value > max {
		return &ConfigValidationError{
			Variable:   varName,
			Value:      value,
			Constraint: fmt.Sprintf("must be between %d and %d (inclusive)", min, max),
		}
	}
	return nil
}

// ValidateFloatRange validates that a float64 is within a specified range [min, max].
func ValidateFloatRange(varName string, value, min, max float64) error {
	if value < min || value > max {
		return &ConfigValidationError{
			Variable:   varName,
			Value:      value,
			Constraint: fmt.Sprintf("must be between %.2f and %.2f (inclusive)", min, max),
		}
	}
	return nil
}

// =============================================================================
// URL & STRING VALIDATORS
// =============================================================================
// Functions that validate URL and string format environment variables.

// ValidateURLFormat validates that a string is a valid URL format if non-empty.
// Does not make HTTP requests, just validates the format.
func ValidateURLFormat(varName, value string) error {
	if value == "" {
		return nil // Empty is valid (disabled)
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return &ConfigValidationError{
			Variable:   varName,
			Value:      value,
			Constraint: "must be a valid URL starting with http:// or https://",
		}
	}
	return nil
}

// ValidateTokenKeyPrefix validates TOKEN_KEY_PREFIX.
// Must be non-empty and typically ends with a separator character.
func ValidateTokenKeyPrefix(value string) error {
	if value == "" {
		return &ConfigValidationError{
			Variable:   "TOKEN_KEY_PREFIX",
			Value:      value,
			Constraint: "must be a non-empty string",
		}
	}
	return nil
}

// =============================================================================
// BATCH VALIDATION
// =============================================================================
// Functions that run all validations and collect errors.

// ValidationResult holds the results of a batch validation run.
type ValidationResult struct {
	Errors []error
}

// HasErrors returns true if any validation errors occurred.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Error returns a combined error message for all validation failures.
func (r *ValidationResult) Error() string {
	if !r.HasErrors() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("configuration validation failed:\n")
	for _, err := range r.Errors {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

// ValidateAllEnvVars validates all environment variables with constraints.
// Returns a ValidationResult containing any errors found.
func ValidateAllEnvVars() *ValidationResult {
	result := &ValidationResult{}

	// String enumeration validators
	if err := ValidateGinMode(GinMode); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateAutoDetectAPIFormatAction(AutoDetectAPIFormatAction); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateLogRotationInterval(LogRotationInterval); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateTheme(Theme); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateGeminiSafetySetting(GeminiSafetySetting); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateGeminiVersion(GeminiVersion); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateOpenTelemetryConfig(OpenTelemetryEnabled, OpenTelemetryEndpoint); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Positive integer validators
	if err := ValidatePositiveInt("MAX_ITEMS_PER_PAGE", MaxItemsPerPage); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("SHUTDOWN_TIMEOUT", ShutdownTimeoutSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("COOKIE_MAXAGE_HOURS", CookieMaxAgeHours); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("SQL_MAX_IDLE_CONNS", SQLMaxIdleConns); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("SQL_MAX_OPEN_CONNS", SQLMaxOpenConns); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("SQL_MAX_LIFETIME", SQLMaxLifetimeSeconds); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("SQLITE_BUSY_TIMEOUT", SQLiteBusyTimeout); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("BILLING_TIMEOUT", BillingTimeoutSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("STREAMING_BILLING_INTERVAL", StreamingBillingIntervalSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("EXTERNAL_BILLING_DEFAULT_TIMEOUT", ExternalBillingDefaultTimeoutSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("EXTERNAL_BILLING_MAX_TIMEOUT", ExternalBillingMaxTimeoutSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("USER_CONTENT_REQUEST_TIMEOUT", UserContentRequestTimeout); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("IDLE_TIMEOUT", IdleTimeout); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("DEFAULT_MAX_TOKEN", DefaultMaxToken); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("TEST_MAX_TOKENS", TestMaxTokens); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Non-negative integer validators (can be 0 to disable)
	if err := ValidateNonNegativeInt("RELAY_TIMEOUT", RelayTimeout); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("SYNC_FREQUENCY", SyncFrequency); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("BATCH_UPDATE_INTERVAL", BatchUpdateInterval); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("BATCH_UPDATE_TIMEOUT", BatchUpdateTimeoutSec); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("METRIC_QUEUE_SIZE", MetricQueueSize); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("METRIC_SUCCESS_CHAN_SIZE", MetricSuccessChanSize); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("METRIC_FAIL_CHAN_SIZE", MetricFailChanSize); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateNonNegativeInt("PRECONSUME_TOKEN_FOR_BACKGROUND_REQUEST", PreconsumeTokenForBackgroundRequest); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Float range validators
	if err := ValidateFloatRange("METRIC_SUCCESS_RATE_THRESHOLD", MetricSuccessRateThreshold, 0.0, 1.0); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Rate limit validators (must be positive)
	if err := ValidatePositiveInt("GLOBAL_API_RATE_LIMIT", GlobalApiRateLimitNum); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("GLOBAL_WEB_RATE_LIMIT", GlobalWebRateLimitNum); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("GLOBAL_RELAY_RATE_LIMIT", GlobalRelayRateLimitNum); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidatePositiveInt("CRITICAL_RATE_LIMIT", CriticalRateLimitNum); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// String format validators
	if err := ValidateTokenKeyPrefix(TokenKeyPrefix); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// URL format validators (optional URLs that must be valid if set)
	if err := ValidateURLFormat("FRONTEND_BASE_URL", FrontendBaseURL); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateURLFormat("RELAY_PROXY", RelayProxy); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateURLFormat("USER_CONTENT_REQUEST_PROXY", UserContentRequestProxy); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateURLFormat("LOG_PUSH_API", LogPushAPI); err != nil {
		result.Errors = append(result.Errors, err)
	}
	if err := ValidateURLFormat("API_BASE", APIBase); err != nil {
		result.Errors = append(result.Errors, err)
	}

	return result
}

// MustValidateEnvVars validates all environment variables and panics if any are invalid.
// Call this during initialization to fail fast on misconfiguration.
func MustValidateEnvVars() {
	result := ValidateAllEnvVars()
	if result.HasErrors() {
		panic(result.Error())
	}
}
