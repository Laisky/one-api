package config

// The G1 initialization and malformed-input gate for observability
// configuration (proposal docs/proposals/20260905_observability-data-tiering.md
// section 3.2: "initialization and malformed-input tests must establish the
// complete matrix for G1").
//
// Every case here drives real environment values through the same entry point
// startup uses, ValidateObservabilityEnv. Package-level variables resolve once
// at import, so a test that only sets os.Environ cannot re-drive initialization;
// ObservabilityEnvFromMap supplies the raw input set instead, which is why the
// validation entry points take one rather than reading the globals.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// observabilityMatrixCase is one row of the section 3.2 matrix or one malformed
// input.
type observabilityMatrixCase struct {
	// name describes the configuration under test.
	name string
	// env is the raw environment; an empty value means unset.
	env map[string]string
	// accept is true when the configuration must start.
	accept bool
	// mentions lists substrings the rejection must contain, so a row cannot pass
	// by being rejected for an unrelated reason.
	mentions []string
	// forbids lists substrings the rejection must NOT contain, used to prove a
	// secret-valued setting is never echoed back.
	forbids []string
}

// otlpEndpoint is a syntactically valid collector authority used wherever a row
// needs OTEL_ENABLED=true.
const otlpEndpoint = "collector.observability.svc:4318"

// TestObservabilityConfigurationMatrix establishes the complete section 3.2
// matrix plus the malformed-input cases.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityConfigurationMatrix(t *testing.T) {
	t.Parallel()

	cases := []observabilityMatrixCase{
		// ------------------------------------------------------------------
		// Section 3.2 row 1: db | sync | false or true -> legacy SQL path,
		// sample rate must be 1.
		// ------------------------------------------------------------------
		{
			name:   "db sync otel disabled",
			env:    map[string]string{EnvTraceSink: TraceSinkDB, EnvTraceWriteMode: TraceWriteModeSync},
			accept: true,
		},
		{
			name: "db sync otel enabled",
			env: map[string]string{
				EnvTraceSink:             TraceSinkDB,
				EnvTraceWriteMode:        TraceWriteModeSync,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},
		{
			name: "db sync with sampling is rejected",
			env: map[string]string{
				EnvTraceSink:       TraceSinkDB,
				EnvTraceWriteMode:  TraceWriteModeSync,
				EnvTraceSampleRate: "0.5",
			},
			mentions: []string{EnvTraceWriteMode, "TRACE_SAMPLE_RATE"},
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 2: none | sync | false or true -> explicitly disable
		// local trace recording, sample rate must be 1.
		// ------------------------------------------------------------------
		{
			name:   "none sync otel disabled",
			env:    map[string]string{EnvTraceSink: TraceSinkNone, EnvTraceWriteMode: TraceWriteModeSync},
			accept: true,
		},
		{
			name: "none sync otel enabled",
			env: map[string]string{
				EnvTraceSink:             TraceSinkNone,
				EnvTraceWriteMode:        TraceWriteModeSync,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},
		{
			name: "none sync with sampling is rejected",
			env: map[string]string{
				EnvTraceSink:       TraceSinkNone,
				EnvTraceWriteMode:  TraceWriteModeSync,
				EnvTraceSampleRate: "0",
			},
			mentions: []string{EnvTraceWriteMode},
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 3: db | batched | false or true -> completed-record
		// SQL writer and local sampling.
		// ------------------------------------------------------------------
		{
			name: "db batched otel disabled with sampling",
			env: map[string]string{
				EnvTraceSink:       TraceSinkDB,
				EnvTraceWriteMode:  TraceWriteModeBatched,
				EnvTraceSampleRate: "0.05",
			},
			accept: true,
		},
		{
			name: "db batched otel enabled",
			env: map[string]string{
				EnvTraceSink:             TraceSinkDB,
				EnvTraceWriteMode:        TraceWriteModeBatched,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 4: none | batched | false or true -> explicit local
		// trace exclusion, no SQL fallback.
		// ------------------------------------------------------------------
		{
			name:   "none batched otel disabled",
			env:    map[string]string{EnvTraceSink: TraceSinkNone, EnvTraceWriteMode: TraceWriteModeBatched},
			accept: true,
		},
		{
			name: "none batched otel enabled",
			env: map[string]string{
				EnvTraceSink:             TraceSinkNone,
				EnvTraceWriteMode:        TraceWriteModeBatched,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 5: otlp or db,otlp | batched | true -> require an
		// endpoint and a successfully initialized provider.
		// ------------------------------------------------------------------
		{
			name: "otlp batched otel enabled with endpoint",
			env: map[string]string{
				EnvTraceSink:             TraceSinkOTLP,
				EnvTraceWriteMode:        TraceWriteModeBatched,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},
		{
			name: "db and otlp batched otel enabled with endpoint",
			env: map[string]string{
				EnvTraceSink:             "db,otlp",
				EnvTraceWriteMode:        TraceWriteModeBatched,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "http://" + otlpEndpoint,
			},
			accept: true,
		},
		{
			name: "otlp batched otel enabled without endpoint is rejected",
			env: map[string]string{
				EnvTraceSink:            TraceSinkOTLP,
				EnvTraceWriteMode:       TraceWriteModeBatched,
				EnvOpenTelemetryEnabled: "true",
			},
			mentions: []string{EnvOpenTelemetryEndpoint},
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 6: any sink including otlp | sync | any -> reject,
		// because sync bypasses the completed-record sink.
		// ------------------------------------------------------------------
		{
			name: "otlp sync is rejected",
			env: map[string]string{
				EnvTraceSink:             TraceSinkOTLP,
				EnvTraceWriteMode:        TraceWriteModeSync,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			mentions: []string{EnvTraceWriteMode},
		},
		{
			name: "db and otlp sync is rejected",
			env: map[string]string{
				EnvTraceSink:             "db,otlp",
				EnvTraceWriteMode:        TraceWriteModeSync,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			mentions: []string{EnvTraceWriteMode},
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 7: any sink including otlp | batched | false ->
		// reject, never count a no-op provider as export.
		// ------------------------------------------------------------------
		{
			name: "otlp batched otel disabled is rejected",
			env: map[string]string{
				EnvTraceSink:      TraceSinkOTLP,
				EnvTraceWriteMode: TraceWriteModeBatched,
			},
			mentions: []string{EnvOpenTelemetryEnabled},
		},
		{
			name: "db and otlp batched otel explicitly disabled is rejected",
			env: map[string]string{
				EnvTraceSink:            "db,otlp",
				EnvTraceWriteMode:       TraceWriteModeBatched,
				EnvOpenTelemetryEnabled: "false",
			},
			mentions: []string{EnvOpenTelemetryEnabled},
		},

		// ------------------------------------------------------------------
		// Section 3.2 row 8: none combined with another sink, or unknown
		// explicit values -> reject the ambiguous or invalid configuration.
		// ------------------------------------------------------------------
		{
			name:     "db combined with none is rejected",
			env:      map[string]string{EnvTraceSink: "db,none", EnvTraceWriteMode: TraceWriteModeBatched},
			mentions: []string{EnvTraceSink, "none"},
		},
		{
			name: "none combined with otlp is rejected",
			env: map[string]string{
				EnvTraceSink:             "none,otlp",
				EnvTraceWriteMode:        TraceWriteModeBatched,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			mentions: []string{EnvTraceSink, "none"},
		},
		{
			name:     "none listed last is still rejected",
			env:      map[string]string{EnvTraceSink: "db , none", EnvTraceWriteMode: TraceWriteModeBatched},
			mentions: []string{EnvTraceSink},
		},
		{
			name:     "a sink list naming nothing is rejected",
			env:      map[string]string{EnvTraceSink: ","},
			mentions: []string{EnvTraceSink},
		},

		// ------------------------------------------------------------------
		// Profiles: the same matrix must hold for a combination reached only
		// through profile defaults, with no trace variable set at all.
		// ------------------------------------------------------------------
		{
			name:   "standalone defaults",
			env:    nil,
			accept: true,
		},
		{
			name:   "scaled profile defaults",
			env:    map[string]string{EnvObservabilityProfile: ObservabilityProfileScaled},
			accept: true,
		},
		{
			name: "external profile without OTEL_ENABLED is rejected",
			env:  map[string]string{EnvObservabilityProfile: ObservabilityProfileExternal},
			// The external profile defaults TRACE_SINK to otlp but does not
			// enable OpenTelemetry, so row 7 rejects it until the operator says
			// so explicitly.
			mentions: []string{EnvOpenTelemetryEnabled},
		},
		{
			name: "external profile with a provider",
			env: map[string]string{
				EnvObservabilityProfile:  ObservabilityProfileExternal,
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint,
			},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Malformed input: unknown explicit values.
		// ------------------------------------------------------------------
		{
			name:     "unknown profile is rejected",
			env:      map[string]string{EnvObservabilityProfile: "scaledd"},
			mentions: []string{EnvObservabilityProfile, "scaledd", ObservabilityProfileStandalone},
		},
		{
			name:     "unknown write mode is rejected",
			env:      map[string]string{EnvTraceWriteMode: "async"},
			mentions: []string{EnvTraceWriteMode, "async", TraceWriteModeBatched},
		},
		{
			name:     "unknown sink is rejected",
			env:      map[string]string{EnvTraceSink: "cassandra"},
			mentions: []string{EnvTraceSink, "cassandra", TraceSinkDB},
		},
		{
			name:     "unknown sink inside a valid list is rejected",
			env:      map[string]string{EnvTraceSink: "db,cassandra"},
			mentions: []string{EnvTraceSink, "cassandra"},
		},
		{
			name:     "unknown application log sink is rejected",
			env:      map[string]string{EnvAppLogSink: "syslog"},
			mentions: []string{EnvAppLogSink, "syslog", AppLogSinkBoth},
		},
		{
			name:     "unknown record line format is rejected",
			env:      map[string]string{EnvLogRecordLineFormat: "verbose"},
			mentions: []string{EnvLogRecordLineFormat, "verbose", LogRecordLineFull},
		},
		{
			name:     "whitespace is not treated as unset",
			env:      map[string]string{EnvObservabilityProfile: " "},
			mentions: []string{EnvObservabilityProfile},
		},
		{
			name:   "an empty value counts as unset",
			env:    map[string]string{EnvObservabilityProfile: "", EnvTraceSink: "", EnvTraceSampleRate: ""},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Malformed input: out-of-range and non-numeric numbers.
		// ------------------------------------------------------------------
		{
			name:     "sample rate above one is rejected",
			env:      map[string]string{EnvTraceSampleRate: "1.5"},
			mentions: []string{EnvTraceSampleRate, "1.5"},
		},
		{
			name:     "negative sample rate is rejected",
			env:      map[string]string{EnvTraceSampleRate: "-0.1"},
			mentions: []string{EnvTraceSampleRate},
		},
		{
			name:     "a percentage typed as a sample rate is rejected",
			env:      map[string]string{EnvTraceSampleRate: "5"},
			mentions: []string{EnvTraceSampleRate},
		},
		{
			name:     "non-numeric sample rate is rejected",
			env:      map[string]string{EnvTraceSampleRate: "half"},
			mentions: []string{EnvTraceSampleRate, "half"},
		},
		{
			name:     "NaN sample rate is rejected",
			env:      map[string]string{EnvTraceSampleRate: "NaN"},
			mentions: []string{EnvTraceSampleRate},
		},
		{
			name:     "non-numeric batch size is rejected",
			env:      map[string]string{EnvTraceBatchSize: "many"},
			mentions: []string{EnvTraceBatchSize, "many"},
		},
		{
			name:     "zero batch size is rejected",
			env:      map[string]string{EnvTraceBatchSize: "0"},
			mentions: []string{EnvTraceBatchSize},
		},
		{
			name:     "negative queue size is rejected",
			env:      map[string]string{EnvTraceQueueSize: "-1"},
			mentions: []string{EnvTraceQueueSize},
		},
		{
			name:     "zero writer count is rejected",
			env:      map[string]string{EnvTraceWriterCount: "0"},
			mentions: []string{EnvTraceWriterCount},
		},
		{
			name:     "negative slow-request threshold is rejected",
			env:      map[string]string{EnvTraceAlwaysSampleSlowMs: "-1"},
			mentions: []string{EnvTraceAlwaysSampleSlowMs},
		},
		{
			name:     "a spaced integer is rejected rather than silently defaulted",
			env:      map[string]string{EnvTraceQueueSize: " 30000 "},
			mentions: []string{EnvTraceQueueSize},
		},
		{
			name:     "zero retention sweep interval is rejected",
			env:      map[string]string{EnvRetentionSweepIntervalMinutes: "0"},
			mentions: []string{EnvRetentionSweepIntervalMinutes},
		},
		{
			name:     "negative log retention is rejected",
			env:      map[string]string{EnvLogRetentionDays: "-3"},
			mentions: []string{EnvLogRetentionDays},
		},
		{
			name:   "zero-disabled knobs accept zero",
			env:    map[string]string{EnvLogRetentionDays: "0", EnvTraceAlwaysSampleSlowMs: "0", EnvDashboardCacheTTLSec: "0"},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Malformed input: non-boolean booleans. env.Bool reads anything but
		// "true" as false, so these would silently mean the opposite.
		// ------------------------------------------------------------------
		{
			name:     "numeric TRACE_ALWAYS_SAMPLE_ERRORS is rejected",
			env:      map[string]string{EnvTraceAlwaysSampleErrors: "1"},
			mentions: []string{EnvTraceAlwaysSampleErrors, "true", "false"},
		},
		{
			name:     "yes is not a boolean",
			env:      map[string]string{EnvTraceAlwaysSampleErrors: "yes"},
			mentions: []string{EnvTraceAlwaysSampleErrors},
		},
		{
			name:     "a spaced boolean is rejected because env.Bool reads it as false",
			env:      map[string]string{EnvTraceAlwaysSampleErrors: " true "},
			mentions: []string{EnvTraceAlwaysSampleErrors},
		},
		{
			name:   "upper-case booleans are accepted",
			env:    map[string]string{EnvTraceAlwaysSampleErrors: "TRUE", EnvOnlyOneLogFile: "False"},
			accept: true,
		},
		{
			name:     "numeric OTEL_ENABLED is rejected",
			env:      map[string]string{EnvOpenTelemetryEnabled: "1", EnvOpenTelemetryEndpoint: otlpEndpoint},
			mentions: []string{EnvOpenTelemetryEnabled},
		},
		{
			name:     "numeric ONLY_ONE_LOG_FILE is rejected",
			env:      map[string]string{EnvOnlyOneLogFile: "1"},
			mentions: []string{EnvOnlyOneLogFile},
		},

		// ------------------------------------------------------------------
		// W0.2: ONLY_ONE_LOG_FILE with an active-file size ceiling.
		// ------------------------------------------------------------------
		{
			name: "single log file with a size ceiling is rejected",
			env: map[string]string{
				EnvOnlyOneLogFile:         "true",
				EnvLogMaxActiveFileSizeMB: "2048",
			},
			mentions: []string{EnvLogMaxActiveFileSizeMB, EnvOnlyOneLogFile},
		},
		{
			name: "single log file with the ceiling disabled is accepted",
			env: map[string]string{
				EnvOnlyOneLogFile:         "true",
				EnvLogMaxActiveFileSizeMB: "0",
			},
			accept: true,
		},
		{
			name: "single log file tolerates a profile-provided ceiling",
			env: map[string]string{
				EnvObservabilityProfile: ObservabilityProfileScaled,
				EnvOnlyOneLogFile:       "true",
			},
			// Every profile supplies a LOG_MAX_ACTIVE_FILE_SIZE_MB default, so
			// rejecting on the effective value would refuse to start any
			// existing deployment that sets ONLY_ONE_LOG_FILE -- a compatibility
			// break on a ceiling the operator never asked for. Only an explicit
			// pairing is a contradiction; a default that cannot apply is inert,
			// and common/logger warns about it at startup.
			accept: true,
		},
		{
			name: "single log file conflicts with an explicitly set ceiling",
			env: map[string]string{
				EnvObservabilityProfile:   ObservabilityProfileScaled,
				EnvOnlyOneLogFile:         "true",
				EnvLogMaxActiveFileSizeMB: "2048",
			},
			// Writing both by hand asks for two incompatible things, which is
			// the case the proposal says to reject.
			mentions: []string{EnvLogMaxActiveFileSizeMB, EnvOnlyOneLogFile},
		},
		{
			name: "a size ceiling without the single-file switch is accepted",
			env: map[string]string{
				EnvOnlyOneLogFile:         "false",
				EnvLogMaxActiveFileSizeMB: "2048",
			},
			accept: true,
		},

		// ------------------------------------------------------------------
		// Row 5, endpoint half: the OTLP exporters take a host:port authority.
		// ------------------------------------------------------------------
		{
			name: "an https endpoint is accepted",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "https://" + otlpEndpoint,
				EnvOpenTelemetryInsecure: "false",
			},
			accept: true,
		},
		{
			name: "an endpoint without a port is accepted",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "collector",
			},
			accept: true,
		},
		{
			name: "a bracketed IPv6 endpoint is accepted",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "[::1]:4318",
			},
			accept: true,
		},
		{
			name: "a non-HTTP scheme is rejected",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "grpc://" + otlpEndpoint,
			},
			mentions: []string{EnvOpenTelemetryEndpoint, "unsupported scheme"},
		},
		{
			name: "an endpoint with a path is rejected",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: otlpEndpoint + "/v1/traces",
			},
			mentions: []string{EnvOpenTelemetryEndpoint, "path"},
		},
		{
			name: "an out-of-range port is rejected",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "collector:99999",
			},
			mentions: []string{EnvOpenTelemetryEndpoint, "65535"},
		},
		{
			name: "a non-numeric port is rejected",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "collector:http",
			},
			mentions: []string{EnvOpenTelemetryEndpoint},
		},
		{
			name: "an unbracketed IPv6 endpoint is rejected",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "::1:4318",
			},
			mentions: []string{EnvOpenTelemetryEndpoint},
		},
		{
			name: "embedded credentials are rejected without echoing them",
			env: map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: "otel:s3cr3t-token@" + otlpEndpoint,
			},
			mentions: []string{EnvOpenTelemetryEndpoint, "credentials"},
			forbids:  []string{"s3cr3t-token"},
		},
		{
			name: "a malformed endpoint is ignored while OpenTelemetry is off",
			env: map[string]string{
				EnvOpenTelemetryEndpoint: "grpc://" + otlpEndpoint,
			},
			accept: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errs := ValidateObservabilityEnv(ObservabilityEnvFromMap(tc.env))
			joined := joinObservabilityErrors(errs)

			if tc.accept {
				require.Empty(t, errs, "this configuration must start: %s", joined)
				return
			}

			require.NotEmpty(t, errs, "this configuration must be rejected")
			for _, want := range tc.mentions {
				require.Contains(t, joined, want,
					"the rejection must name it: %s", joined)
			}
			for _, forbidden := range tc.forbids {
				require.NotContains(t, joined, forbidden,
					"a secret-valued setting must never be echoed back")
			}
		})
	}
}

// TestObservabilityEnvFromOSDrivesTheSameChecks verifies the environment-backed
// input set behaves exactly like the explicit one, so the matrix above really
// describes the startup path and not a parallel implementation.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityEnvFromOSDrivesTheSameChecks(t *testing.T) {
	t.Setenv(EnvTraceSink, "db,none")
	t.Setenv(EnvTraceWriteMode, TraceWriteModeBatched)

	errs := ValidateObservabilityEnv(ObservabilityEnvFromOS())
	require.NotEmpty(t, errs, "db,none must be rejected when read from the process environment")
	require.Contains(t, joinObservabilityErrors(errs), EnvTraceSink)

	t.Setenv(EnvTraceSink, TraceSinkDB)
	require.Empty(t, ValidateObservabilityEnv(ObservabilityEnvFromOS()))
}

// TestResolveObservabilityEnvMatchesPackageVariables verifies the resolver
// reproduces the package variable initializers.
//
// The combination checks run against ResolveObservabilityEnv, so if the two
// drifted apart the matrix would be validating a configuration the process does
// not run on. Under the test environment both sides see the same variables.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestResolveObservabilityEnvMatchesPackageVariables(t *testing.T) {
	resolved := ResolveObservabilityEnv(ObservabilityEnvFromOS())

	require.Equal(t, ObservabilityProfile, resolved.Profile)
	require.Equal(t, TraceSinks, resolved.TraceSinks)
	require.Equal(t, TraceWriteMode, resolved.TraceWriteMode)
	require.InDelta(t, TraceSampleRate, resolved.TraceSampleRate, 1e-9)
	require.Equal(t, OpenTelemetryEnabled, resolved.OpenTelemetryEnabled)
	require.Equal(t, OpenTelemetryInsecure, resolved.OpenTelemetryInsecure)
	require.Equal(t, OpenTelemetryEndpoint, resolved.OpenTelemetryEndpoint)
	require.Equal(t, OnlyOneLogFile, resolved.OnlyOneLogFile)
	require.Equal(t, LogMaxActiveFileSizeMB, resolved.LogMaxActiveFileSizeMB)
}

// TestObservabilityProfileDefaultsResolveToTheDocumentedMatrix verifies the
// three profiles select the sink and write mode section 3.1 documents, since
// every combination rule is evaluated against those defaults.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityProfileDefaultsResolveToTheDocumentedMatrix(t *testing.T) {
	t.Parallel()

	standalone := ResolveObservabilityEnv(ObservabilityEnvFromMap(nil))
	require.Equal(t, []string{TraceSinkDB}, standalone.TraceSinks)
	require.Equal(t, TraceWriteModeSync, standalone.TraceWriteMode)
	require.InDelta(t, 1.0, standalone.TraceSampleRate, 1e-9)
	// The active-file ceiling is ON by default under every profile, generously
	// sized. It is not a behavior opt-in like sampling or batching: an unbounded
	// active log file is the W0.2 defect, and shipping it disabled would leave
	// the default deployment -- the one with nobody watching the log volume --
	// carrying the bug.
	require.Equal(t, 4096, standalone.LogMaxActiveFileSizeMB)

	scaled := ResolveObservabilityEnv(ObservabilityEnvFromMap(
		map[string]string{EnvObservabilityProfile: ObservabilityProfileScaled}))
	require.Equal(t, []string{TraceSinkDB}, scaled.TraceSinks)
	require.Equal(t, TraceWriteModeBatched, scaled.TraceWriteMode)
	require.InDelta(t, 0.05, scaled.TraceSampleRate, 1e-9)
	require.Equal(t, 2048, scaled.LogMaxActiveFileSizeMB)

	external := ResolveObservabilityEnv(ObservabilityEnvFromMap(
		map[string]string{EnvObservabilityProfile: ObservabilityProfileExternal}))
	require.Equal(t, []string{TraceSinkOTLP}, external.TraceSinks)
	require.Equal(t, TraceWriteModeBatched, external.TraceWriteMode)
	require.False(t, external.OpenTelemetryEnabled,
		"the external profile must not enable OpenTelemetry on its own")
}

// TestObservabilityRawInputCoversEveryDocumentedKnob verifies the specification
// tables name every variable the raw layer is supposed to guard, so adding a
// knob without a bound is visible here rather than in production.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityRawInputCoversEveryDocumentedKnob(t *testing.T) {
	t.Parallel()

	guarded := make(map[string]bool, len(observabilityEnvNames))
	for _, name := range observabilityEnvNames {
		guarded[name] = true
	}

	for _, name := range []string{
		EnvObservabilityProfile,
		EnvTraceSink,
		EnvTraceWriteMode,
		EnvTraceSampleRate,
		EnvTraceAlwaysSampleErrors,
		EnvTraceAlwaysSampleSlowMs,
		EnvTraceBatchSize,
		EnvTraceFlushIntervalMs,
		EnvTraceQueueSize,
		EnvTraceWriterCount,
		EnvTraceRetentionDays,
		EnvTraceMaxRecordBytes,
		EnvTraceMaxExternalCalls,
		EnvTraceMaxActiveRecorders,
		EnvTraceBatchMaxBytes,
		EnvAsyncTaskRetentionDays,
		EnvRetentionDeleteBatchSize,
		EnvRetentionDeletePauseMs,
		EnvRetentionSweepIntervalMinutes,
		EnvAppLogSink,
		EnvLogRecordLineFormat,
		EnvLogRetentionDays,
		EnvLogMaxTotalSizeMB,
		EnvLogMinFreeDiskMB,
		EnvLogMaxActiveFileSizeMB,
		EnvLogDiskCheckIntervalSec,
		EnvLogEmergencyMaxBytesPerSec,
		EnvLogDiskRecoveryMarginPct,
		EnvLogSampleInitial,
		EnvLogSampleThereafter,
		EnvLogSampleTickMs,
		EnvDashboardCacheTTLSec,
		EnvDashboardMaxSitewideRangeDays,
		EnvDashboardMaxConcurrentAggregates,
		EnvOnlyOneLogFile,
		EnvOpenTelemetryEnabled,
		EnvOpenTelemetryEndpoint,
		EnvOpenTelemetryInsecure,
	} {
		require.True(t, guarded[name], "%s must be validated as raw input", name)
	}
}

// joinObservabilityErrors renders a validation result for assertions.
//
// Parameters:
//   - errs: the validation errors.
//
// Return values:
//   - string: every message, newline separated; empty when there are none.
func joinObservabilityErrors(errs []error) string {
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "\n")
}
