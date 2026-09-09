// Package config provides centralized configuration management for one-api.
//
// This file holds the VALID-COMBINATION layer of observability configuration
// validation: the complete matrix of docs/proposals/20260905_observability-data-tiering.md
// section 3.2, plus the W0.2 rotation conflict.
//
// The raw layer in observability_env.go answers "is this value one an operator
// could have meant?". This layer answers "do the values, together, describe a
// pipeline the process can actually implement?". They are separate because a
// combination can be invalid while every individual value is valid --
// TRACE_SINK=db,none is two known sinks that contradict each other -- and
// because a combination can be reached from profile defaults alone, with no
// variable set at all (OBSERVABILITY_PROFILE=external selects the otlp sink,
// which then requires OTEL_ENABLED=true that the profile does not set).
//
// SECTION 3.2, ROW BY ROW
//
//	db      | sync    | any   | accept; TRACE_SAMPLE_RATE must be 1
//	                            -> ValidateSyncTraceConfiguration
//	none    | sync    | any   | accept; TRACE_SAMPLE_RATE must be 1
//	                            -> ValidateSyncTraceConfiguration
//	db      | batched | any   | accept
//	none    | batched | any   | accept, with no SQL fallback
//	                            -> enforced in common/tracing (traceDisabled)
//	otlp,   | batched | true  | require endpoint and an initialized provider
//	db,otlp |         |       | -> ValidateOpenTelemetryConfig (presence),
//	                            ValidateOTLPEndpointFormat (parseability);
//	                            provider initialization is NOT verifiable from
//	                            this package, see the note on
//	                            ValidateOTLPEndpointFormat
//	*otlp*  | sync    | any   | reject -> ValidateSyncTraceConfiguration
//	*otlp*  | batched | false | reject -> ValidateTraceSinkOpenTelemetryConfig
//	none + another sink,      | reject -> ValidateTraceSinkCombination
//	unknown explicit values   | reject -> ValidateObservabilityRawInput

package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// =============================================================================
// RESOLUTION
// =============================================================================

// ResolvedObservability holds the effective values a combination check needs.
//
// It carries only the settings that participate in a cross-field rule, so the
// per-profile default tables are not duplicated wholesale; every other knob is
// covered by the raw range checks. The resolution below mirrors the package
// variable initializers exactly, so this describes the configuration the
// process runs on rather than an idealized reading of the environment.
type ResolvedObservability struct {
	// Profile is the resolved OBSERVABILITY_PROFILE.
	Profile string
	// TraceSinks is the resolved, ordered TRACE_SINK list.
	TraceSinks []string
	// TraceWriteMode is the resolved TRACE_WRITE_MODE.
	TraceWriteMode string
	// TraceSampleRate is the resolved TRACE_SAMPLE_RATE.
	TraceSampleRate float64
	// OpenTelemetryEnabled is the resolved OTEL_ENABLED.
	OpenTelemetryEnabled bool
	// OpenTelemetryInsecure is the resolved OTEL_EXPORTER_OTLP_INSECURE switch.
	OpenTelemetryInsecure bool
	// OpenTelemetryEndpoint is the resolved OTEL_EXPORTER_OTLP_ENDPOINT with any
	// http:// or https:// prefix removed, matching the package variable the OTLP
	// exporters are configured with.
	OpenTelemetryEndpoint string
	// OpenTelemetryEndpointRaw is the endpoint exactly as configured, which is
	// what format validation must inspect: the scheme is part of what an
	// operator can get wrong.
	OpenTelemetryEndpointRaw string
	// OnlyOneLogFile is the resolved ONLY_ONE_LOG_FILE.
	OnlyOneLogFile bool
	// LogMaxActiveFileSizeMB is the resolved LOG_MAX_ACTIVE_FILE_SIZE_MB.
	LogMaxActiveFileSizeMB int
	// LogMaxActiveFileSizeExplicit reports whether LOG_MAX_ACTIVE_FILE_SIZE_MB
	// was set by the operator rather than supplied by the profile. Only an
	// explicit request can be a contradiction; a default the operator never
	// chose must not fail their startup.
	LogMaxActiveFileSizeExplicit bool
	// AppLogSink is the resolved LOCAL application-log destination.
	AppLogSink string
	// AppLogOTLPEnabled reports whether APP_LOG_SINK asked for the additive
	// OTLP application-log bridge.
	AppLogOTLPEnabled bool
}

// ResolveObservabilityEnv computes the effective observability configuration
// from a raw input set.
//
// It applies the same profile defaults, readers and normalizers the package
// variables use, so a value it returns is a value the process would run on.
// Invalid raw input resolves the way production resolves it -- silently to a
// default -- because rejecting is the raw layer's job; running the combination
// checks on an idealized value would describe a process that does not exist.
//
// Parameters:
//   - in: the raw input set.
//
// Return values:
//   - ResolvedObservability: the effective configuration.
func ResolveObservabilityEnv(in ObservabilityEnv) ResolvedObservability {
	profile := normalizeProfile(in.stringOr(EnvObservabilityProfile, ObservabilityProfileStandalone))

	endpointRaw := strings.TrimSpace(in.stringOr(EnvOpenTelemetryEndpoint, ""))

	return ResolvedObservability{
		Profile: profile,
		TraceSinks: parseTraceSinks(in.stringOr(EnvTraceSink,
			profileString(profile, TraceSinkDB, TraceSinkDB, TraceSinkOTLP))),
		TraceWriteMode: normalizeWriteMode(in.stringOr(EnvTraceWriteMode,
			profileString(profile, TraceWriteModeSync, TraceWriteModeBatched, TraceWriteModeBatched))),
		TraceSampleRate: clampUnitInterval(in.floatOr(EnvTraceSampleRate,
			profileFloat(profile, 1.0, 0.05, 1.0))),
		OpenTelemetryEnabled:     in.boolOr(EnvOpenTelemetryEnabled, false),
		OpenTelemetryInsecure:    in.boolOr(EnvOpenTelemetryInsecure, true),
		OpenTelemetryEndpoint:    normalizeOTLPEndpoint(endpointRaw),
		OpenTelemetryEndpointRaw: endpointRaw,
		OnlyOneLogFile:           in.boolOr(EnvOnlyOneLogFile, false),
		LogMaxActiveFileSizeMB: nonNegative(in.intOr(EnvLogMaxActiveFileSizeMB,
			profileInt(profile, 4096, 2048, 1024))),
		LogMaxActiveFileSizeExplicit: explicitlySet(in, EnvLogMaxActiveFileSizeMB),
		AppLogSink:                   normalizeAppLogSink(in.stringOr(EnvAppLogSink, AppLogSinkBoth)),
		AppLogOTLPEnabled:            appLogSinkHasOTLP(in.stringOr(EnvAppLogSink, AppLogSinkBoth)),
	}
}

// =============================================================================
// ENTRY POINTS
// =============================================================================

// ValidateObservabilityEnv runs both validation layers over one raw input set.
//
// This is the function the startup path and the matrix tests share. When the
// raw layer rejects anything it returns those errors alone: the combination
// checks would then be describing values normalization substituted, and
// reporting "TRACE_SINK=db" to an operator who wrote "TRACE_SINK=cassandra" is
// worse than saying nothing.
//
// Parameters:
//   - in: the raw input set.
//
// Return values:
//   - []error: every rejection; empty when the configuration is valid.
func ValidateObservabilityEnv(in ObservabilityEnv) []error {
	if errs := ValidateObservabilityRawInput(in); len(errs) > 0 {
		return errs
	}
	return ValidateObservabilityCombination(ResolveObservabilityEnv(in))
}

// ValidateObservabilityCombination enforces the section 3.2 matrix and the W0.2
// rotation conflict against an effective configuration.
//
// Parameters:
//   - resolved: the effective configuration, from ResolveObservabilityEnv.
//
// Return values:
//   - []error: every rejected combination; empty when the matrix is satisfied.
func ValidateObservabilityCombination(resolved ResolvedObservability) []error {
	var errs []error

	if err := ValidateTraceSinkCombination(resolved.TraceSinks); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateSyncTraceConfiguration(resolved.TraceWriteMode, resolved.TraceSinks,
		resolved.TraceSampleRate); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if err := ValidateTraceSinkOpenTelemetryConfig(resolved.TraceSinks,
		resolved.OpenTelemetryEnabled); err != nil {
		errs = append(errs, errors.WithStack(err))
	}

	// Endpoint presence first, then shape: a missing endpoint is already
	// reported by the presence check, and reporting "" as a malformed host on
	// top of it helps nobody.
	if err := ValidateOpenTelemetryConfig(resolved.OpenTelemetryEnabled,
		resolved.OpenTelemetryEndpoint); err != nil {
		errs = append(errs, errors.WithStack(err))
	} else if resolved.OpenTelemetryEnabled {
		if err := ValidateOTLPEndpointFormat(resolved.OpenTelemetryEndpointRaw); err != nil {
			errs = append(errs, err)
		} else if err := ValidateOTLPTransportSecurity(resolved.OpenTelemetryEndpointRaw,
			resolved.OpenTelemetryInsecure); err != nil {
			errs = append(errs, err)
		}
	}

	if err := ValidateAppLogSinkOpenTelemetryConfig(resolved.AppLogOTLPEnabled,
		resolved.OpenTelemetryEnabled); err != nil {
		errs = append(errs, errors.WithStack(err))
	}

	if err := ValidateLogFileSizeRotationConflict(resolved.OnlyOneLogFile,
		resolved.LogMaxActiveFileSizeMB, resolved.LogMaxActiveFileSizeExplicit); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// normalizeOTLPEndpoint returns the authority the OTLP HTTP exporters accept.
//
// Parameters:
//   - raw: an OTLP endpoint with an optional HTTP scheme.
//
// Return values:
//   - string: the trimmed endpoint with an HTTP or HTTPS scheme removed,
//     regardless of scheme letter case.
func normalizeOTLPEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	scheme, authority, found := strings.Cut(endpoint, "://")
	if found && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")) {
		return authority
	}
	return endpoint
}

// =============================================================================
// COMBINATION VALIDATORS
// =============================================================================

// ValidateTraceSinkCombination rejects a sink list that cannot describe one
// unambiguous destination for a completed trace.
//
// Two cases exist. An empty list has no meaning at all. A list combining `none`
// with another sink is the ambiguous configuration section 3.2 names
// explicitly: parseTraceSinks accepts "db,none", and traceDisabled() in
// common/tracing requires the list to be EXACTLY [none], so the process quietly
// runs as "db, plus a sink that drops every record" -- while an operator who
// wrote it plainly meant to turn tracing off. Rejecting is the only reading
// that cannot be wrong.
//
// Parameters:
//   - sinks: the resolved TRACE_SINK identifiers.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the combination is invalid.
func ValidateTraceSinkCombination(sinks []string) error {
	if len(sinks) == 0 {
		return errors.WithStack(&ConfigValidationError{
			Variable:    EnvTraceSink,
			Value:       "",
			Constraint:  "must name at least one sink",
			AllowedVals: []string{TraceSinkDB, TraceSinkOTLP, TraceSinkNone},
		})
	}

	for _, sink := range sinks {
		if sink != TraceSinkNone || len(sinks) == 1 {
			continue
		}
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvTraceSink,
			Value:      strings.Join(sinks, ","),
			Constraint: "none cannot be combined with another sink: it means \"record no trace\", which contradicts every sink it is listed with; use none alone to disable tracing",
		})
	}

	return nil
}

// ValidateOTLPEndpointFormat rejects an OTLP endpoint the exporters cannot use.
//
// OTEL_EXPORTER_OTLP_ENDPOINT reaches otlptracehttp.WithEndpoint and
// otlpmetrichttp.WithEndpoint, which take a host:port authority. The package
// variable strips an http:// or https:// prefix and nothing else, so any other
// scheme, a path, or a non-numeric port travels into the exporter unchanged and
// fails later as a transport error, or -- worse -- looks like a collector
// outage. Section 3.2 requires the otlp rows to have a usable endpoint before
// the process claims it exports anything.
//
// WHAT THIS CANNOT CHECK. The same row also requires a "successfully
// initialized provider". Provider construction happens in common/telemetry and
// the sink is built in common/tracing; from configuration alone a syntactically
// valid endpoint whose provider later fails to initialize is indistinguishable
// from a working one. Enforcing that belongs where the provider is built.
//
// Parameters:
//   - value: the raw OTEL_EXPORTER_OTLP_ENDPOINT value, scheme included.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when the endpoint is unusable.
func ValidateOTLPEndpointFormat(value string) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil // Presence is ValidateOpenTelemetryConfig's rule.
	}

	authority := raw
	if scheme, rest, found := strings.Cut(raw, "://"); found {
		switch strings.ToLower(scheme) {
		case "http", "https":
			authority = rest
		default:
			return errors.WithStack(&ConfigValidationError{
				Variable:    EnvOpenTelemetryEndpoint,
				Value:       redactedOTLPEndpoint,
				Constraint:  "uses an unsupported scheme; the OTLP exporters are HTTP and take a host:port authority",
				AllowedVals: []string{"http", "https", "no scheme"},
			})
		}
	}

	// Credentials in the endpoint are a secret, so the value is never echoed
	// back in this rejection.
	if strings.Contains(authority, "@") {
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryEndpoint,
			Value:      redactedOTLPEndpoint,
			Constraint: "must not embed credentials; configure collector authentication through OTEL_EXPORTER_OTLP_HEADERS instead",
		})
	}

	if strings.ContainsAny(authority, "/?#") {
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryEndpoint,
			Value:      redactedOTLPEndpoint,
			Constraint: "must be a host:port authority with no path, query or fragment; the exporter appends the OTLP path itself",
		})
	}
	if strings.ContainsAny(authority, " \t\r\n") {
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryEndpoint,
			Value:      redactedOTLPEndpoint,
			Constraint: "must not contain whitespace",
		})
	}

	host, port, err := splitEndpointAuthority(authority)
	if err != nil {
		return errors.Wrapf(err, "parse %s", EnvOpenTelemetryEndpoint)
	}
	if host == "" {
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryEndpoint,
			Value:      redactedOTLPEndpoint,
			Constraint: "must name a host",
		})
	}
	if port != "" {
		number, convErr := strconv.Atoi(port)
		if convErr != nil {
			return errors.WithStack(&ConfigValidationError{
				Variable:   EnvOpenTelemetryEndpoint,
				Value:      redactedOTLPEndpoint,
				Constraint: "must use a numeric port between 1 and 65535",
			})
		}
		if number < 1 || number > 65535 {
			return errors.WithStack(&ConfigValidationError{
				Variable:   EnvOpenTelemetryEndpoint,
				Value:      redactedOTLPEndpoint,
				Constraint: fmt.Sprintf("port %d must be between 1 and 65535 (inclusive)", number),
			})
		}
	}

	return nil
}

const redactedOTLPEndpoint = "<redacted>"

// ValidateOTLPTransportSecurity rejects an endpoint scheme that contradicts
// OTEL_EXPORTER_OTLP_INSECURE, which controls the TLS mode the exporters use.
//
// Parameters:
//   - endpoint: the raw endpoint, including any scheme.
//   - insecure: whether the OTLP HTTP exporters skip TLS verification.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when an explicit HTTP scheme and
//     TLS mode contradict each other; nil when the endpoint has no scheme or is
//     consistent with the configured transport.
func ValidateOTLPTransportSecurity(endpoint string, insecure bool) error {
	scheme, _, found := strings.Cut(strings.TrimSpace(endpoint), "://")
	if !found {
		return nil
	}

	switch {
	case strings.EqualFold(scheme, "https") && insecure:
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryInsecure,
			Value:      insecure,
			Constraint: "must be false when OTEL_EXPORTER_OTLP_ENDPOINT uses https://",
		})
	case strings.EqualFold(scheme, "http") && !insecure:
		return errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryInsecure,
			Value:      insecure,
			Constraint: "must be true when OTEL_EXPORTER_OTLP_ENDPOINT uses http://",
		})
	default:
		return nil
	}
}

// splitEndpointAuthority splits a host[:port] authority, understanding the
// bracketed form IPv6 literals require.
//
// Parameters:
//   - authority: the endpoint with any scheme already removed.
//
// Return values:
//   - string: the host, without brackets.
//   - string: the port, empty when none was given.
//   - error: a wrapped *ConfigValidationError when the authority is malformed.
func splitEndpointAuthority(authority string) (string, string, error) {
	if strings.HasPrefix(authority, "[") {
		end := strings.Index(authority, "]")
		if end < 0 {
			return "", "", errors.WithStack(&ConfigValidationError{
				Variable:   EnvOpenTelemetryEndpoint,
				Value:      redactedOTLPEndpoint,
				Constraint: "has an unterminated IPv6 literal; use [::1]:4318",
			})
		}
		host := authority[1:end]
		switch rest := authority[end+1:]; {
		case rest == "":
			return host, "", nil
		case strings.HasPrefix(rest, ":"):
			return host, rest[1:], nil
		default:
			return "", "", errors.WithStack(&ConfigValidationError{
				Variable:   EnvOpenTelemetryEndpoint,
				Value:      redactedOTLPEndpoint,
				Constraint: "must be [host]:port after an IPv6 literal",
			})
		}
	}

	switch strings.Count(authority, ":") {
	case 0:
		return authority, "", nil
	case 1:
		host, port, _ := strings.Cut(authority, ":")
		return host, port, nil
	default:
		return "", "", errors.WithStack(&ConfigValidationError{
			Variable:   EnvOpenTelemetryEndpoint,
			Value:      redactedOTLPEndpoint,
			Constraint: "has too many colons; wrap an IPv6 literal in brackets, as in [::1]:4318",
		})
	}
}

// ValidateLogFileSizeRotationConflict rejects an active-file size ceiling that
// no rotation can enforce (proposal W0.2).
//
// ONLY_ONE_LOG_FILE keeps a single, never-rotated file. LOG_MAX_ACTIVE_FILE_SIZE_MB
// is a ceiling implemented BY rotating that file when it is reached. Together
// they promise a bound the writer cannot deliver: it would have to either
// truncate the operator's log or ignore the setting. The proposal is explicit
// that this must "reject the combination rather than promise an impossible
// bound".
//
// Only an EXPLICIT ceiling is a contradiction. The ceiling is equally
// unenforceable when it arrives from a profile default, but the remedies differ
// completely: an operator who wrote LOG_MAX_ACTIVE_FILE_SIZE_MB next to
// ONLY_ONE_LOG_FILE asked for two incompatible things and needs to be told,
// whereas an operator who only wrote ONLY_ONE_LOG_FILE=true asked for exactly
// one thing and must keep booting. Rejecting on the effective value would break
// every existing OBSERVABILITY_PROFILE=scaled deployment that sets
// ONLY_ONE_LOG_FILE, since that profile supplies 2048 MiB on its own -- an
// upgrade may not do that. In the default case the ceiling is simply inert and
// common/logger emits a WARN naming the effect and the fix.
//
// Parameters:
//   - onlyOneLogFile: the resolved ONLY_ONE_LOG_FILE.
//   - maxActiveFileSizeMB: the resolved LOG_MAX_ACTIVE_FILE_SIZE_MB.
//   - explicitlySet: whether the ceiling came from the environment rather than
//     from a profile default.
//
// Return values:
//   - error: a wrapped *ConfigValidationError when both were explicitly asked
//     for; nil when the ceiling is merely a default that cannot apply.
func ValidateLogFileSizeRotationConflict(onlyOneLogFile bool, maxActiveFileSizeMB int, explicitlySet bool) error {
	if !onlyOneLogFile || maxActiveFileSizeMB == 0 || !explicitlySet {
		return nil
	}

	return errors.WithStack(&ConfigValidationError{
		Variable: EnvLogMaxActiveFileSizeMB,
		Value:    maxActiveFileSizeMB,
		Constraint: fmt.Sprintf("cannot be enforced while %s is true, because that disables rotation entirely; set %s=0 to keep a single unbounded file, or %s=false to let the ceiling rotate it",
			EnvOnlyOneLogFile, EnvLogMaxActiveFileSizeMB, EnvOnlyOneLogFile),
	})
}

// explicitlySet reports whether a variable was provided by the environment
// rather than defaulted.
//
// Parameters:
//   - in: the raw input set.
//   - name: the environment variable name.
//
// Return values:
//   - bool: true when the operator set the variable to a non-empty value.
func explicitlySet(in ObservabilityEnv, name string) bool {
	_, ok := in.Lookup(name)
	return ok
}
