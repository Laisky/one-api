package config

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOTLPEndpointTransportConfigurationRejectsContradictions verifies an
// explicit endpoint scheme cannot silently select the opposite TLS mode.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestOTLPEndpointTransportConfigurationRejectsContradictions(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		insecure string
		accept   bool
	}{
		{name: "HTTPS requires TLS", endpoint: "HTTPS://collector:4318", insecure: "true"},
		{name: "HTTP requires insecure transport", endpoint: "http://collector:4318", insecure: "false"},
		{name: "HTTPS with TLS", endpoint: "HTTPS://collector:4318", insecure: "false", accept: true},
		{name: "HTTP with insecure transport", endpoint: "http://collector:4318", insecure: "true", accept: true},
		{name: "authority leaves transport explicit", endpoint: "collector:4318", insecure: "false", accept: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateObservabilityEnv(ObservabilityEnvFromMap(map[string]string{
				EnvOpenTelemetryEnabled:  "true",
				EnvOpenTelemetryEndpoint: tc.endpoint,
				EnvOpenTelemetryInsecure: tc.insecure,
			}))
			if tc.accept {
				require.Empty(t, errs, joinObservabilityErrors(errs))
				return
			}
			require.NotEmpty(t, errs)
			require.Contains(t, joinObservabilityErrors(errs), "OTEL_EXPORTER_OTLP_INSECURE")
		})
	}
}

// TestResolveObservabilityEnvNormalizesOTLPEndpointSchemeCase verifies the
// validation resolver configures the same authority that the exporter uses.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestResolveObservabilityEnvNormalizesOTLPEndpointSchemeCase(t *testing.T) {
	resolved := ResolveObservabilityEnv(ObservabilityEnvFromMap(map[string]string{
		EnvOpenTelemetryEndpoint: "HTTPS://collector:4318",
	}))
	require.Equal(t, "collector:4318", resolved.OpenTelemetryEndpoint)
}

// TestMalformedOTLPEndpointsNeverEchoSecrets verifies diagnostics redact every
// malformed endpoint because authorities may contain credentials or query tokens.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestMalformedOTLPEndpointsNeverEchoSecrets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		endpoint string
		secret   string
	}{
		{name: "query", endpoint: "https://collector:4318?api_key=query-secret", secret: "query-secret"},
		{name: "userinfo", endpoint: "https://user:userinfo-secret@collector:4318", secret: "userinfo-secret"},
		{name: "scheme", endpoint: "grpc://collector:4318?token=scheme-secret", secret: "scheme-secret"},
		{name: "port", endpoint: "https://collector:port-secret", secret: "port-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOTLPEndpointFormat(tc.endpoint)
			require.Error(t, err)
			require.NotContains(t, err.Error(), tc.secret)
		})
	}
}

// TestObservabilityIntegerBoundsRejectUnsafeConversions verifies raw values
// that would overflow duration or mebibyte conversions fail before startup.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityIntegerBoundsRejectUnsafeConversions(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: EnvLogMaxActiveFileSizeMB, value: strconv.FormatInt(math.MaxInt64>>19, 10)},
		{name: EnvLogDiskCheckIntervalSec, value: strconv.FormatInt(math.MaxInt64/int64(time.Second)+1, 10)},
		{name: EnvRetentionSweepIntervalMinutes, value: strconv.FormatInt(math.MaxInt64/int64(time.Minute)+1, 10)},
		{name: EnvLogSampleTickMs, value: strconv.FormatInt(math.MaxInt64/int64(time.Millisecond)+1, 10)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateObservabilityRawInput(ObservabilityEnvFromMap(map[string]string{tc.name: tc.value}))
			require.NotEmpty(t, errs)
			require.Contains(t, joinObservabilityErrors(errs), tc.name)
		})
	}
}

// TestObservabilityConversionHelpersSaturateInvalidDirectAssignments verifies
// callers that bypass raw environment validation still cannot wrap limits into
// negative byte counts or durations.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestObservabilityConversionHelpersSaturateInvalidDirectAssignments(t *testing.T) {
	prevSize, prevTotal, prevFree, prevInterval, prevMargin :=
		LogMaxActiveFileSizeMB, LogMaxTotalSizeMB, LogMinFreeDiskMB, LogDiskCheckIntervalSec, LogDiskRecoveryMarginPct
	t.Cleanup(func() {
		LogMaxActiveFileSizeMB, LogMaxTotalSizeMB, LogMinFreeDiskMB, LogDiskCheckIntervalSec, LogDiskRecoveryMarginPct =
			prevSize, prevTotal, prevFree, prevInterval, prevMargin
	})

	LogMaxActiveFileSizeMB = math.MaxInt
	require.Equal(t, int64(math.MaxInt64), LogMaxActiveFileSizeBytes())
	LogMaxTotalSizeMB, LogMinFreeDiskMB = math.MaxInt, math.MaxInt
	require.Equal(t, int64(math.MaxInt64), LogMaxTotalSizeBytes())
	require.Equal(t, int64(math.MaxInt64), LogMinFreeDiskBytes())

	LogDiskCheckIntervalSec = math.MaxInt
	require.Equal(t, time.Duration(math.MaxInt64), LogDiskCheckInterval())

	LogDiskRecoveryMarginPct = math.MaxInt
	require.Equal(t, int64(math.MaxInt64), LogDiskRecoveryFloorBytes(math.MaxInt64-1))
}

// TestTraceRecordByteLimitRejectsImpossibleBaseBudget verifies startup does not
// accept a byte ceiling smaller than the recorder's fixed bounded state.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestTraceRecordByteLimitRejectsImpossibleBaseBudget(t *testing.T) {
	errs := ValidateObservabilityRawInput(ObservabilityEnvFromMap(map[string]string{
		EnvTraceMaxRecordBytes: "1",
	}))
	require.NotEmpty(t, errs)
	require.Contains(t, joinObservabilityErrors(errs), EnvTraceMaxRecordBytes)

	require.Empty(t, ValidateObservabilityRawInput(ObservabilityEnvFromMap(map[string]string{
		EnvTraceMaxRecordBytes: "1024",
	})))
}
