// Package config provides centralized configuration management for one-api.
//
// This file holds the RAW-INPUT layer of observability configuration
// validation required by docs/proposals/20260905_observability-data-tiering.md
// section 3.1 (final paragraph) and section 3.2 (W1).
//
// WHY A SEPARATE RAW LAYER EXISTS
//
// Every observability setting in this package is read through the silently
// defaulting helpers in common/env and then normalized: normalizeProfile turns
// an unknown profile into standalone, normalizeWriteMode turns an unknown mode
// into batched, parseTraceSinks discards unknown sink tokens and resolves an
// all-invalid list to "db", clampUnitInterval folds an out-of-range sample rate
// into [0, 1], and positiveOr/nonNegative replace an out-of-range integer with
// its default. env.Bool treats every value other than "true" as false, so
// TRACE_ALWAYS_SAMPLE_ERRORS=1 silently disables the error rule.
//
// The consequence is that a validator which inspects the resolved package
// variable can never fire: by the time it runs, the invalid value has already
// been replaced by a valid one. The proposal calls this out explicitly -- "This
// is not raw-input fail-fast validation. W1 must reject explicitly invalid
// observability values before normalization can silently change their meaning."
//
// So there are two layers, and they are deliberately both kept:
//
//  1. RAW (this file): the exact environment string is parsed with the same
//     semantics the production reader uses, and an explicitly set value that
//     the reader would silently reinterpret is REJECTED. This is what fails
//     startup.
//  2. NORMALIZED (observability.go): the normalizers keep their fallbacks as a
//     last line of defense for callers that assign the package variables
//     directly (tests, and common/tracing's own re-validation), so a value that
//     never passed through the raw layer still cannot crash the process or
//     silently discard every trace.
//
// UNSET VS. EXPLICITLY INVALID
//
// An unset variable -- absent, or exactly the empty string -- keeps today's
// default silently; that is the documented behavior of every reader in
// common/env and this file preserves it. A value that is present and non-empty
// is "explicitly set" and must be valid. A whitespace-only value is therefore
// rejected rather than treated as unset: every current reader would resolve it
// to something the operator did not write (env.Bool(" ") is false even when the
// documented default is true), which is exactly the silent reinterpretation
// this layer exists to prevent.

package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// =============================================================================
// OBSERVABILITY ENVIRONMENT VARIABLE NAMES
// =============================================================================
// Named constants so the specification tables, the validators, and the tests
// all refer to one spelling of each variable.

const (
	// EnvObservabilityProfile names the profile selector.
	EnvObservabilityProfile = "OBSERVABILITY_PROFILE"
	// EnvTraceSink names the comma-separated completed-trace sink list.
	EnvTraceSink = "TRACE_SINK"
	// EnvTraceWriteMode names the trace write-mode selector.
	EnvTraceWriteMode = "TRACE_WRITE_MODE"
	// EnvTraceSampleRate names the completed-trace sampling probability.
	EnvTraceSampleRate = "TRACE_SAMPLE_RATE"
	// EnvTraceAlwaysSampleErrors names the keep-every-error selection rule.
	EnvTraceAlwaysSampleErrors = "TRACE_ALWAYS_SAMPLE_ERRORS"
	// EnvTraceAlwaysSampleSlowMs names the keep-every-slow-request rule.
	EnvTraceAlwaysSampleSlowMs = "TRACE_ALWAYS_SAMPLE_SLOW_MS"
	// EnvTraceBatchSize names the rows-per-INSERT bound.
	EnvTraceBatchSize = "TRACE_BATCH_SIZE"
	// EnvTraceFlushIntervalMs names the partial-batch flush deadline.
	EnvTraceFlushIntervalMs = "TRACE_FLUSH_INTERVAL_MS"
	// EnvTraceQueueSize names the completed-record queue bound.
	EnvTraceQueueSize = "TRACE_QUEUE_SIZE"
	// EnvTraceWriterCount names the number of queue-draining writers.
	EnvTraceWriterCount = "TRACE_WRITER_COUNT"
	// EnvTraceRetentionDays names the trace-row retention horizon.
	EnvTraceRetentionDays = "TRACE_RETENTION_DAYS"
	// EnvTraceMaxRecordBytes names the per-record memory bound.
	EnvTraceMaxRecordBytes = "TRACE_MAX_RECORD_BYTES"
	// EnvTraceMaxExternalCalls names the per-record external-call bound.
	EnvTraceMaxExternalCalls = "TRACE_MAX_EXTERNAL_CALLS"
	// EnvTraceMaxActiveRecorders names the in-flight recorder admission bound.
	EnvTraceMaxActiveRecorders = "TRACE_MAX_ACTIVE_RECORDERS"
	// EnvTraceBatchMaxBytes names the flush-local buffer byte bound.
	EnvTraceBatchMaxBytes = "TRACE_BATCH_MAX_BYTES"
	// EnvAsyncTaskRetentionDays names the async-task retention horizon.
	EnvAsyncTaskRetentionDays = "ASYNC_TASK_RETENTION_DAYS"
	// EnvRetentionDeleteBatchSize names the retention chunk size.
	EnvRetentionDeleteBatchSize = "RETENTION_DELETE_BATCH_SIZE"
	// EnvRetentionDeletePauseMs names the pause between retention chunks.
	EnvRetentionDeletePauseMs = "RETENTION_DELETE_PAUSE_MS"
	// EnvRetentionSweepIntervalMinutes names the retention sweep cadence.
	EnvRetentionSweepIntervalMinutes = "RETENTION_SWEEP_INTERVAL_MINUTES"
	// EnvAppLogSink names the application log destination selector.
	EnvAppLogSink = "APP_LOG_SINK"
	// EnvLogRecordLineFormat names the record-log line shape selector.
	EnvLogRecordLineFormat = "LOG_RECORD_LINE_FORMAT"
	// EnvAppLogOTLPMinLevel names the OTLP application-log severity floor.
	EnvAppLogOTLPMinLevel = "LOG_OTLP_MIN_LEVEL"
	// EnvAppLogOTLPQueueSize names the OTLP application-log record ceiling.
	EnvAppLogOTLPQueueSize = "LOG_OTLP_QUEUE_SIZE"
	// EnvAppLogOTLPQueueMaxMB names the OTLP application-log byte ceiling.
	EnvAppLogOTLPQueueMaxMB = "LOG_OTLP_QUEUE_MAX_MB"
	// EnvAppLogOTLPBatchSize names the OTLP application-log export batch size.
	EnvAppLogOTLPBatchSize = "LOG_OTLP_BATCH_SIZE"
	// EnvAppLogOTLPExportIntervalMs names the OTLP application-log flush cadence.
	EnvAppLogOTLPExportIntervalMs = "LOG_OTLP_EXPORT_INTERVAL_MS"
	// EnvAppLogOTLPExportTimeoutMs names the OTLP application-log export timeout.
	EnvAppLogOTLPExportTimeoutMs = "LOG_OTLP_EXPORT_TIMEOUT_MS"
	// EnvAppLogOTLPMaxAttributes names the per-record attribute ceiling.
	EnvAppLogOTLPMaxAttributes = "LOG_OTLP_MAX_ATTRIBUTES"
	// EnvAppLogOTLPMaxAttributeValueBytes names the per-attribute value ceiling.
	EnvAppLogOTLPMaxAttributeValueBytes = "LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES"
	// EnvLogRetentionDays names the application log-file retention horizon.
	EnvLogRetentionDays = "LOG_RETENTION_DAYS"
	// EnvLogMaxTotalSizeMB names the log-directory size ceiling.
	EnvLogMaxTotalSizeMB = "LOG_MAX_TOTAL_SIZE_MB"
	// EnvLogMinFreeDiskMB names the free-disk floor.
	EnvLogMinFreeDiskMB = "LOG_MIN_FREE_DISK_MB"
	// EnvLogMaxActiveFileSizeMB names the active log-file size ceiling.
	EnvLogMaxActiveFileSizeMB = "LOG_MAX_ACTIVE_FILE_SIZE_MB"
	// EnvLogDiskCheckIntervalSec names the disk-pressure sampling cadence.
	EnvLogDiskCheckIntervalSec = "LOG_DISK_CHECK_INTERVAL_SEC"
	// EnvLogEmergencyMaxBytesPerSec names the emergency-mode byte budget.
	EnvLogEmergencyMaxBytesPerSec = "LOG_EMERGENCY_MAX_BYTES_PER_SEC"
	// EnvLogDiskRecoveryMarginPct names the disk-guard hysteresis margin.
	EnvLogDiskRecoveryMarginPct = "LOG_DISK_RECOVERY_MARGIN_PCT"
	// EnvLogSampleInitial names the per-tick unsampled log budget.
	EnvLogSampleInitial = "LOG_SAMPLE_INITIAL"
	// EnvLogSampleThereafter names the log thinning factor.
	EnvLogSampleThereafter = "LOG_SAMPLE_THEREAFTER"
	// EnvLogSampleTickMs names the log sampling window.
	EnvLogSampleTickMs = "LOG_SAMPLE_TICK_MS"
	// EnvDashboardCacheTTLSec names the dashboard aggregate cache TTL.
	EnvDashboardCacheTTLSec = "DASHBOARD_CACHE_TTL_SEC"
	// EnvDashboardMaxSitewideRangeDays names the site-wide range cap.
	EnvDashboardMaxSitewideRangeDays = "DASHBOARD_MAX_SITEWIDE_RANGE_DAYS"
	// EnvDashboardMaxConcurrentAggregates names the aggregate concurrency budget.
	EnvDashboardMaxConcurrentAggregates = "DASHBOARD_MAX_CONCURRENT_AGGREGATES"
	// EnvOnlyOneLogFile names the single-log-file switch.
	EnvOnlyOneLogFile = "ONLY_ONE_LOG_FILE"
	// EnvOpenTelemetryEnabled names the OpenTelemetry provider switch.
	EnvOpenTelemetryEnabled = "OTEL_ENABLED"
	// EnvOpenTelemetryEndpoint names the OTLP collector endpoint.
	EnvOpenTelemetryEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	// EnvOpenTelemetryInsecure names the OTLP HTTP transport-security switch.
	EnvOpenTelemetryInsecure = "OTEL_EXPORTER_OTLP_INSECURE"
)

// =============================================================================
// RAW INPUT SET
// =============================================================================

// ObservabilityEnv is an immutable snapshot of the raw observability
// environment strings.
//
// It exists so validation can run against an explicit input set instead of the
// process environment. Package-level variables are resolved once at import, so
// a test that only mutates os.Environ can never re-drive the initialization
// path; a test that builds an ObservabilityEnv can drive every row of the
// section 3.2 matrix without a subprocess.
type ObservabilityEnv struct {
	// values holds only the entries that are present and non-empty, keyed by
	// environment variable name and stored verbatim (never trimmed), because
	// the readers this layer mirrors do not trim either.
	values map[string]string
}

// ObservabilityEnvFromOS snapshots the observability variables of the process
// environment.
//
// Parameters: none.
//
// Return values:
//   - ObservabilityEnv: the raw input set actually seen by the package
//     variables in observability.go, observability_limits.go and config.go.
func ObservabilityEnvFromOS() ObservabilityEnv {
	values := make(map[string]string, len(observabilityEnvNames))
	for _, name := range observabilityEnvNames {
		if raw := os.Getenv(name); raw != "" {
			values[name] = raw
		}
	}
	return ObservabilityEnv{values: values}
}

// ObservabilityEnvFromMap builds a raw input set from explicit values.
//
// Parameters:
//   - values: environment variable name to raw value. A key mapped to the empty
//     string counts as unset, matching every reader in common/env.
//
// Return values:
//   - ObservabilityEnv: the raw input set.
func ObservabilityEnvFromMap(values map[string]string) ObservabilityEnv {
	snapshot := make(map[string]string, len(values))
	for name, raw := range values {
		if raw != "" {
			snapshot[name] = raw
		}
	}
	return ObservabilityEnv{values: snapshot}
}

// Lookup returns the raw value of one variable.
//
// Parameters:
//   - name: the environment variable name.
//
// Return values:
//   - string: the verbatim value; empty when unset.
//   - bool: true when the variable is explicitly set to a non-empty value.
func (e ObservabilityEnv) Lookup(name string) (string, bool) {
	raw, ok := e.values[name]
	return raw, ok
}

// observabilityEnvNames lists every variable this layer reads, so the OS
// snapshot and the documentation stay in one place.
var observabilityEnvNames = func() []string {
	names := []string{
		EnvObservabilityProfile,
		EnvTraceSink,
		EnvTraceSampleRate,
		EnvOpenTelemetryEndpoint,
		EnvAppLogSink,
	}
	for _, spec := range observabilityEnumSpecs {
		names = append(names, spec.name)
	}
	for _, spec := range observabilityIntSpecs {
		names = append(names, spec.name)
	}
	names = append(names, observabilityBoolNames...)
	return names
}()

// =============================================================================
// SPECIFICATIONS
// =============================================================================

// observabilityEnumSpec describes a setting whose value must be one of a fixed
// set of names.
type observabilityEnumSpec struct {
	// name is the environment variable name.
	name string
	// allowed lists the accepted lowercase values.
	allowed []string
}

// observabilityEnumSpecs lists every observability setting normalized by a
// switch that silently falls back. TRACE_SINK and APP_LOG_SINK are absent
// because they are lists, validated by validateRawTraceSinks and
// validateRawAppLogSink.
var observabilityEnumSpecs = []observabilityEnumSpec{
	{name: EnvTraceWriteMode, allowed: []string{TraceWriteModeBatched, TraceWriteModeSync}},
	{name: EnvLogRecordLineFormat, allowed: []string{LogRecordLineFull, LogRecordLineCompact}},
	{name: EnvAppLogOTLPMinLevel, allowed: []string{
		AppLogOTLPLevelDebug, AppLogOTLPLevelInfo, AppLogOTLPLevelWarn, AppLogOTLPLevelError,
	}},
}

// observabilityIntSpec describes a setting whose value must be an integer at or
// above a lower bound. The bound mirrors the clamp the production reader
// applies: minValue 1 for positiveOr, 0 for nonNegative.
type observabilityIntSpec struct {
	// name is the environment variable name.
	name string
	// minValue is the smallest accepted value, inclusive.
	minValue int
	// maxValue is the largest accepted value, inclusive. Zero means no upper
	// bound is needed because every downstream conversion is already safe.
	maxValue int
}

// observabilityIntSpecs lists every observability integer whose out-of-range or
// unparseable value is silently replaced by a default today.
var observabilityIntSpecs = []observabilityIntSpec{
	{name: EnvTraceAlwaysSampleSlowMs, minValue: 0},
	{name: EnvTraceBatchSize, minValue: 1},
	{name: EnvTraceFlushIntervalMs, minValue: 1, maxValue: maxDurationMilliseconds},
	{name: EnvTraceQueueSize, minValue: 1},
	{name: EnvTraceWriterCount, minValue: 1},
	{name: EnvTraceRetentionDays, minValue: 0, maxValue: maxRetentionDays},
	{name: EnvTraceMaxRecordBytes, minValue: MinTraceRecordBytes},
	{name: EnvTraceMaxExternalCalls, minValue: 1},
	{name: EnvTraceMaxActiveRecorders, minValue: 0},
	{name: EnvTraceBatchMaxBytes, minValue: 1},
	{name: EnvAsyncTaskRetentionDays, minValue: 0, maxValue: maxRetentionDays},
	{name: EnvRetentionDeleteBatchSize, minValue: 1},
	{name: EnvRetentionDeletePauseMs, minValue: 0, maxValue: maxDurationMilliseconds},
	{name: EnvRetentionSweepIntervalMinutes, minValue: 1, maxValue: maxDurationMinutes},
	{name: EnvLogRetentionDays, minValue: 0, maxValue: maxRetentionDays},
	{name: EnvLogMaxTotalSizeMB, minValue: 0, maxValue: maxMebibytes},
	{name: EnvLogMinFreeDiskMB, minValue: 0, maxValue: maxMebibytes},
	{name: EnvLogMaxActiveFileSizeMB, minValue: 0, maxValue: maxMebibytes},
	{name: EnvLogDiskCheckIntervalSec, minValue: 1, maxValue: maxDurationSeconds},
	{name: EnvLogEmergencyMaxBytesPerSec, minValue: 1},
	{name: EnvLogDiskRecoveryMarginPct, minValue: 0},
	{name: EnvLogSampleInitial, minValue: 0},
	{name: EnvLogSampleThereafter, minValue: 0},
	{name: EnvLogSampleTickMs, minValue: 1, maxValue: maxDurationMilliseconds},
	{name: EnvDashboardCacheTTLSec, minValue: 0, maxValue: maxDurationSeconds},
	{name: EnvDashboardMaxSitewideRangeDays, minValue: 1},
	{name: EnvDashboardMaxConcurrentAggregates, minValue: 0},
	{name: EnvAppLogOTLPQueueSize, minValue: 1},
	{name: EnvAppLogOTLPQueueMaxMB, minValue: 1, maxValue: maxMebibytes},
	{name: EnvAppLogOTLPBatchSize, minValue: 1},
	{name: EnvAppLogOTLPExportIntervalMs, minValue: 1, maxValue: maxDurationMilliseconds},
	{name: EnvAppLogOTLPExportTimeoutMs, minValue: 1, maxValue: maxDurationMilliseconds},
	{name: EnvAppLogOTLPMaxAttributes, minValue: 1},
	{name: EnvAppLogOTLPMaxAttributeValueBytes, minValue: 1},
}

// observabilityBoolNames lists every observability boolean read through
// env.Bool, which resolves anything other than "true" to false without a
// diagnostic. "1", "yes" and "on" are therefore silent disablers and must be
// rejected rather than accepted with a different meaning.
var observabilityBoolNames = []string{
	EnvTraceAlwaysSampleErrors,
	EnvOnlyOneLogFile,
	EnvOpenTelemetryEnabled,
	EnvOpenTelemetryInsecure,
}

const (
	// MinTraceRecordBytes is the smallest configurable recorder byte budget.
	// It leaves room for fixed recorder state and bounded identifying fields;
	// smaller values cannot describe an enforceable total-record ceiling.
	MinTraceRecordBytes = 1024
	// maxMebibytes is the greatest mebibyte count that can be converted to an
	// int64 byte count without overflow.
	maxMebibytes = int(math.MaxInt64 >> 20)
	// maxDurationMilliseconds is the greatest millisecond count representable
	// by time.Duration.
	maxDurationMilliseconds = int(math.MaxInt64 / int64(time.Millisecond))
	// maxDurationSeconds is the greatest second count representable by
	// time.Duration.
	maxDurationSeconds = int(math.MaxInt64 / int64(time.Second))
	// maxDurationMinutes is the greatest minute count representable by
	// time.Duration.
	maxDurationMinutes = int(math.MaxInt64 / int64(time.Minute))
	// maxRetentionDays is the greatest day count representable by a duration.
	maxRetentionDays = int(math.MaxInt64 / int64(24*time.Hour))
)

// observabilityBoolAllowed lists the only accepted boolean spellings. It is
// exactly the set env.Bool interprets correctly: it lowercases its input and
// compares against "true", so "true"/"TRUE" mean true and "false"/"FALSE" mean
// false, while every other spelling would silently mean false.
var observabilityBoolAllowed = []string{"true", "false"}

// =============================================================================
// RAW VALIDATION
// =============================================================================

// ValidateObservabilityRawInput rejects every explicitly set observability
// value that normalization would silently reinterpret.
//
// It validates the raw environment strings, not the resolved package variables,
// and it parses each one with the exact semantics of the reader that produced
// the package variable, so a value this function accepts is a value the process
// actually runs on.
//
// Parameters:
//   - in: the raw input set.
//
// Return values:
//   - []error: one wrapped *ConfigValidationError per rejected variable, naming
//     the variable, the offending value and the allowed values; empty when
//     every explicitly set value is valid.
func ValidateObservabilityRawInput(in ObservabilityEnv) []error {
	var errs []error

	if err := validateRawEnum(in, EnvObservabilityProfile, []string{
		ObservabilityProfileStandalone,
		ObservabilityProfileScaled,
		ObservabilityProfileExternal,
	}); err != nil {
		errs = append(errs, err)
	}
	for _, spec := range observabilityEnumSpecs {
		if err := validateRawEnum(in, spec.name, spec.allowed); err != nil {
			errs = append(errs, err)
		}
	}

	if err := validateRawAppLogSink(in); err != nil {
		errs = append(errs, err)
	}

	if err := validateRawTraceSinks(in); err != nil {
		errs = append(errs, err)
	}

	if err := validateRawUnitInterval(in, EnvTraceSampleRate); err != nil {
		errs = append(errs, err)
	}

	for _, spec := range observabilityIntSpecs {
		if err := validateRawInt(in, spec.name, spec.minValue, spec.maxValue); err != nil {
			errs = append(errs, err)
		}
	}

	for _, name := range observabilityBoolNames {
		if err := validateRawBool(in, name); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// validateRawEnum rejects an explicitly set value outside a fixed set.
//
// The comparison mirrors the normalizers in observability.go, which trim and
// lowercase before switching, so " SCALED " remains valid.
//
// Parameters:
//   - in: the raw input set.
//   - name: the environment variable name.
//   - allowed: the accepted lowercase values.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the value is set and unknown.
func validateRawEnum(in ObservabilityEnv, name string, allowed []string) error {
	raw, ok := in.Lookup(name)
	if !ok {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, candidate := range allowed {
		if normalized == candidate {
			return nil
		}
	}

	return errors.WithStack(&ConfigValidationError{
		Variable:    name,
		Value:       raw,
		Constraint:  "must be one of the documented values; it is not normalized to a default",
		AllowedVals: allowed,
	})
}

// validateRawTraceSinks rejects an explicitly set TRACE_SINK list containing an
// unknown token or naming no sink at all.
//
// parseTraceSinks discards unknown tokens and resolves an all-invalid list to
// "db", so without this check TRACE_SINK=cassandra starts a process that writes
// traces to SQL while the operator believes it exports them elsewhere. The
// "none combined with another sink" rejection is a combination rule and lives
// in ValidateTraceSinkCombination, which also sees profile-derived lists.
//
// Parameters:
//   - in: the raw input set.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the list is set and invalid.
func validateRawTraceSinks(in ObservabilityEnv) error {
	raw, ok := in.Lookup(EnvTraceSink)
	if !ok {
		return nil
	}

	allowed := []string{TraceSinkDB, TraceSinkOTLP, TraceSinkNone}
	named := 0
	for _, part := range strings.Split(raw, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			// A stray separator ("db,") is not ambiguous: parseTraceSinks skips
			// it and so does this check.
			continue
		}
		known := false
		for _, candidate := range allowed {
			if token == candidate {
				known = true
				break
			}
		}
		if !known {
			return errors.WithStack(&ConfigValidationError{
				Variable:    EnvTraceSink,
				Value:       raw,
				Constraint:  fmt.Sprintf("names unknown sink %q; unknown sinks are not discarded", token),
				AllowedVals: allowed,
			})
		}
		named++
	}

	if named == 0 {
		return errors.WithStack(&ConfigValidationError{
			Variable:    EnvTraceSink,
			Value:       raw,
			Constraint:  "must name at least one sink",
			AllowedVals: allowed,
		})
	}

	return nil
}

// validateRawAppLogSink rejects an explicitly set APP_LOG_SINK that
// normalizeAppLogSink would silently reinterpret.
//
// APP_LOG_SINK became a list when the optional OTLP application-log bridge was
// added, so it needs the same treatment TRACE_SINK gets: the normalizer folds
// anything it does not recognize into "both", which would turn a typo such as
// "stdou,otlp" into a configuration that writes files the operator asked it not
// to write and exports nothing.
//
// The accepted grammar is one local destination, optionally plus the additive
// "otlp" token, in either order. A bare "otlp" is rejected on purpose: the
// bridge drops records when its queue is full, before the provider is
// installed, and after it is shut down, so a deployment with no local sink
// would have no record of its own log loss.
//
// Parameters:
//   - in: the raw input set.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the value is set and invalid.
func validateRawAppLogSink(in ObservabilityEnv) error {
	raw, ok := in.Lookup(EnvAppLogSink)
	if !ok {
		return nil
	}

	allowed := []string{AppLogSinkFile, AppLogSinkStdout, AppLogSinkBoth,
		AppLogSinkBoth + "," + AppLogSinkOTLP,
		AppLogSinkStdout + "," + AppLogSinkOTLP,
		AppLogSinkFile + "," + AppLogSinkOTLP}

	reject := func(constraint string) error {
		return errors.WithStack(&ConfigValidationError{
			Variable:    EnvAppLogSink,
			Value:       raw,
			Constraint:  constraint,
			AllowedVals: allowed,
		})
	}

	local := 0
	otlp := 0
	for _, part := range strings.Split(raw, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			// A stray separator ("both,") is skipped by the parser and here.
			continue
		}
		switch token {
		case AppLogSinkFile, AppLogSinkStdout, AppLogSinkBoth:
			local++
		case AppLogSinkOTLP:
			otlp++
		default:
			return reject(fmt.Sprintf("names unknown sink %q; unknown sinks are not discarded", token))
		}
	}

	switch {
	case local == 0 && otlp == 0:
		return reject("must name at least one sink")
	case local == 0:
		return reject("names only the additive \"otlp\" sink; it must accompany a local sink so log loss stays recorded somewhere")
	case local > 1:
		return reject("names more than one local sink; use \"both\" for stdout and file together")
	case otlp > 1:
		return reject("repeats the \"otlp\" sink")
	}

	return nil
}

// validateRawUnitInterval rejects an explicitly set probability that is not a
// finite number in [0, 1].
//
// clampUnitInterval folds 5 into 1 and -1 into 0, so an operator who typed a
// percentage instead of a probability currently gets full capture with no
// diagnostic.
//
// Parameters:
//   - in: the raw input set.
//   - name: the environment variable name.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the value is set and invalid.
func validateRawUnitInterval(in ObservabilityEnv, name string) error {
	raw, ok := in.Lookup(name)
	if !ok {
		return nil
	}

	// env.Float64 parses the value verbatim, so this parse must too: a value
	// that only parses after trimming would be validated here and silently
	// replaced by the default there.
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return errors.Wrapf(err, "parse %s value %q: must be a number between 0 and 1 (inclusive)", name, raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      raw,
			Constraint: "must be a finite number between 0 and 1 (inclusive)",
		})
	}
	if value < 0 || value > 1 {
		return errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      raw,
			Constraint: "must be between 0 and 1 (inclusive); it is not clamped into range",
		})
	}

	return nil
}

// validateRawInt rejects an explicitly set integer that is unparseable or
// outside its documented range.
//
// Parameters:
//   - in: the raw input set.
//   - name: the environment variable name.
//   - minValue: the smallest accepted value, inclusive.
//   - maxValue: the greatest accepted value, inclusive; zero disables the
//     upper bound.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the value is set and invalid.
func validateRawInt(in ObservabilityEnv, name string, minValue, maxValue int) error {
	raw, ok := in.Lookup(name)
	if !ok {
		return nil
	}

	// env.Int parses the value verbatim and falls back to the default on any
	// error, so this parse mirrors it exactly.
	value, err := strconv.Atoi(raw)
	if err != nil {
		return errors.Wrapf(err, "parse %s value %q: must be an integer >= %d", name, raw, minValue)
	}
	if value < minValue {
		return errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      value,
			Constraint: fmt.Sprintf("must be an integer >= %d; it is not replaced by the default", minValue),
		})
	}
	if maxValue > 0 && value > maxValue {
		return errors.WithStack(&ConfigValidationError{
			Variable:   name,
			Value:      value,
			Constraint: fmt.Sprintf("must be an integer between %d and %d (inclusive); larger values overflow downstream conversions", minValue, maxValue),
		})
	}

	return nil
}

// validateRawBool rejects an explicitly set boolean that env.Bool would read as
// false without saying so.
//
// env.Bool compares the lowercased value against "true", so "1", "yes", "on"
// and " true " all mean false. That silently disables TRACE_ALWAYS_SAMPLE_ERRORS
// and silently turns OTEL_ENABLED off, which the section 3.2 matrix then
// rejects for an unrelated-looking reason.
//
// Parameters:
//   - in: the raw input set.
//   - name: the environment variable name.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the value is set and is not
//     "true" or "false" in any letter case.
func validateRawBool(in ObservabilityEnv, name string) error {
	raw, ok := in.Lookup(name)
	if !ok {
		return nil
	}

	switch strings.ToLower(raw) {
	case "true", "false":
		return nil
	}

	return errors.WithStack(&ConfigValidationError{
		Variable:    name,
		Value:       raw,
		Constraint:  "must be spelled true or false; any other value is read as false without a diagnostic",
		AllowedVals: observabilityBoolAllowed,
	})
}

// =============================================================================
// READERS THAT MIRROR PRODUCTION RESOLUTION
// =============================================================================
// These deliberately fall back the way common/env does. They describe what the
// process WILL run on, so the combination checks in observability_matrix.go
// examine the same values the package variables hold. Rejection is the raw
// layer's job, never theirs.

// stringOr returns the raw value or a default, mirroring env.String.
//
// Parameters:
//   - name: the environment variable name.
//   - defaultValue: the value used when the variable is unset.
//
// Return values:
//   - string: the resolved value.
func (e ObservabilityEnv) stringOr(name, defaultValue string) string {
	if raw, ok := e.Lookup(name); ok {
		return raw
	}
	return defaultValue
}

// intOr returns the parsed value or a default, mirroring env.Int.
//
// Parameters:
//   - name: the environment variable name.
//   - defaultValue: the value used when the variable is unset or unparseable.
//
// Return values:
//   - int: the resolved value.
func (e ObservabilityEnv) intOr(name string, defaultValue int) int {
	raw, ok := e.Lookup(name)
	if !ok {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

// floatOr returns the parsed value or a default, mirroring env.Float64.
//
// Parameters:
//   - name: the environment variable name.
//   - defaultValue: the value used when the variable is unset or unparseable.
//
// Return values:
//   - float64: the resolved value.
func (e ObservabilityEnv) floatOr(name string, defaultValue float64) float64 {
	raw, ok := e.Lookup(name)
	if !ok {
		return defaultValue
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

// boolOr returns the parsed value or a default, mirroring env.Bool including
// its "anything but true is false" behavior.
//
// Parameters:
//   - name: the environment variable name.
//   - defaultValue: the value used when the variable is unset.
//
// Return values:
//   - bool: the resolved value.
func (e ObservabilityEnv) boolOr(name string, defaultValue bool) bool {
	raw, ok := e.Lookup(name)
	if !ok {
		return defaultValue
	}
	return strings.ToLower(raw) == "true"
}
