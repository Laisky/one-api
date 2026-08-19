package config

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// externalUUIDBackfillEnvVars lists every environment variable owned by the
// external UUID backfill loader. Tests clear all of them so one case cannot leak
// configuration into another.
var externalUUIDBackfillEnvVars = []string{
	EnvExternalUUIDBackfillMaxRowsPerCycle,
	EnvExternalUUIDBackfillMaxCycleDuration,
	EnvExternalUUIDBackfillActiveInterval,
	EnvExternalUUIDBackfillIdleInterval,
	EnvExternalUUIDBackfillLockTimeout,
	EnvExternalUUIDBackfillDDLTimeout,
	EnvExternalUUIDBackfillAllowBlockingDDL,
	EnvExternalUUIDBackfillAutoFinalize,
	EnvExternalUUIDBackfillAutoFinalizeIdlePasses,
}

// clearExternalUUIDBackfillEnv unsets every backfill variable for the duration of
// the test and restores the package settings afterwards.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func clearExternalUUIDBackfillEnv(t *testing.T) {
	t.Helper()
	for _, name := range externalUUIDBackfillEnvVars {
		t.Setenv(name, "")
	}
	t.Cleanup(func() {
		// Restore documented defaults so a later test never observes the values
		// written by this one. t.Setenv has already restored the environment.
		require.NoError(t, LoadExternalUUIDBackfillSettings())
	})
}

// TestLoadExternalUUIDBackfillSettingsDefaults asserts that an unset environment
// yields the documented defaults and never fails.
func TestLoadExternalUUIDBackfillSettingsDefaults(t *testing.T) {
	clearExternalUUIDBackfillEnv(t)

	require.NoError(t, LoadExternalUUIDBackfillSettings())

	require.Equal(t, 10000, ExternalUUIDBackfillMaxRowsPerCycle)
	require.Equal(t, 30*time.Second, ExternalUUIDBackfillMaxCycleDuration)
	require.Equal(t, 5*time.Second, ExternalUUIDBackfillActiveInterval)
	require.Equal(t, 5*time.Minute, ExternalUUIDBackfillIdleInterval)
	require.Equal(t, 5*time.Second, ExternalUUIDBackfillLockTimeout)
	require.Equal(t, 30*time.Minute, ExternalUUIDBackfillDDLTimeout)
	require.False(t, ExternalUUIDBackfillAllowBlockingDDL)
	require.True(t, ExternalUUIDBackfillAutoFinalize,
		"a default deployment must complete the migration automatically")
	require.Equal(t, 3, ExternalUUIDBackfillAutoFinalizeIdlePasses)
}

// TestLoadExternalUUIDBackfillAutoFinalizeSettings asserts the automatic-completion
// settings parse strictly and validate inclusively.
func TestLoadExternalUUIDBackfillAutoFinalizeSettings(t *testing.T) {
	clearExternalUUIDBackfillEnv(t)

	t.Setenv(EnvExternalUUIDBackfillAutoFinalize, "false")
	require.NoError(t, LoadExternalUUIDBackfillSettings())
	require.False(t, ExternalUUIDBackfillAutoFinalize)

	t.Setenv(EnvExternalUUIDBackfillAutoFinalize, "yes")
	err := LoadExternalUUIDBackfillSettings()
	require.Error(t, err, "a typo must not silently change completion policy")
	require.Contains(t, err.Error(), EnvExternalUUIDBackfillAutoFinalize)
	t.Setenv(EnvExternalUUIDBackfillAutoFinalize, "")

	for _, tt := range []struct {
		value string
		want  int
		ok    bool
	}{
		{value: "1", want: 1, ok: true},
		{value: "1000", want: 1000, ok: true},
		{value: "0", ok: false},
		{value: "1001", ok: false},
		{value: "abc", ok: false},
	} {
		t.Setenv(EnvExternalUUIDBackfillAutoFinalizeIdlePasses, tt.value)
		err := LoadExternalUUIDBackfillSettings()
		if !tt.ok {
			require.Error(t, err, "value %q must be rejected", tt.value)
			require.Contains(t, err.Error(), EnvExternalUUIDBackfillAutoFinalizeIdlePasses)
			continue
		}
		require.NoError(t, err, "value %q must be accepted", tt.value)
		require.Equal(t, tt.want, ExternalUUIDBackfillAutoFinalizeIdlePasses)
	}
}

// TestLoadExternalUUIDBackfillSettingsRanges asserts inclusive boundary
// acceptance and out-of-range or unparseable rejection for every ranged setting.
func TestLoadExternalUUIDBackfillSettingsRanges(t *testing.T) {
	type valueCase struct {
		name    string
		value   string
		wantErr bool
	}

	tests := []struct {
		envVar string
		// observe returns the loaded value so accepted cases can be verified.
		observe func() string
		cases   []valueCase
	}{
		{
			envVar:  EnvExternalUUIDBackfillMaxRowsPerCycle,
			observe: func() string { return strconv.Itoa(ExternalUUIDBackfillMaxRowsPerCycle) },
			cases: []valueCase{
				{name: "min accepted", value: "1000", wantErr: false},
				{name: "max accepted", value: "1000000", wantErr: false},
				{name: "default accepted", value: "10000", wantErr: false},
				{name: "below min rejected", value: "999", wantErr: true},
				{name: "above max rejected", value: "1000001", wantErr: true},
				{name: "zero rejected", value: "0", wantErr: true},
				{name: "negative rejected", value: "-1", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
				{name: "duration string rejected", value: "30s", wantErr: true},
			},
		},
		{
			envVar:  EnvExternalUUIDBackfillMaxCycleDuration,
			observe: func() string { return ExternalUUIDBackfillMaxCycleDuration.String() },
			cases: []valueCase{
				{name: "min accepted", value: "1s", wantErr: false},
				{name: "max accepted", value: "30m", wantErr: false},
				{name: "default accepted", value: "30s", wantErr: false},
				{name: "below min rejected", value: "999ms", wantErr: true},
				{name: "above max rejected", value: "30m1s", wantErr: true},
				{name: "zero rejected", value: "0s", wantErr: true},
				{name: "negative rejected", value: "-1s", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
				{name: "unitless rejected", value: "30", wantErr: true},
			},
		},
		{
			envVar:  EnvExternalUUIDBackfillActiveInterval,
			observe: func() string { return ExternalUUIDBackfillActiveInterval.String() },
			cases: []valueCase{
				{name: "min accepted", value: "0s", wantErr: false},
				{name: "max accepted", value: "5m", wantErr: false},
				{name: "default accepted", value: "5s", wantErr: false},
				{name: "below min rejected", value: "-1ns", wantErr: true},
				{name: "above max rejected", value: "5m1s", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
			},
		},
		{
			envVar:  EnvExternalUUIDBackfillIdleInterval,
			observe: func() string { return ExternalUUIDBackfillIdleInterval.String() },
			cases: []valueCase{
				{name: "min accepted", value: "5s", wantErr: false},
				{name: "max accepted", value: "1h", wantErr: false},
				{name: "default accepted", value: "5m", wantErr: false},
				{name: "below min rejected", value: "4999ms", wantErr: true},
				{name: "above max rejected", value: "1h1s", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
			},
		},
		{
			envVar:  EnvExternalUUIDBackfillLockTimeout,
			observe: func() string { return ExternalUUIDBackfillLockTimeout.String() },
			cases: []valueCase{
				{name: "min accepted", value: "1s", wantErr: false},
				{name: "max accepted", value: "5m", wantErr: false},
				{name: "default accepted", value: "5s", wantErr: false},
				{name: "below min rejected", value: "999ms", wantErr: true},
				{name: "above max rejected", value: "5m1s", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
			},
		},
		{
			envVar:  EnvExternalUUIDBackfillDDLTimeout,
			observe: func() string { return ExternalUUIDBackfillDDLTimeout.String() },
			cases: []valueCase{
				{name: "min accepted", value: "1m", wantErr: false},
				{name: "max accepted", value: "24h", wantErr: false},
				{name: "default accepted", value: "30m", wantErr: false},
				{name: "below min rejected", value: "59s", wantErr: true},
				{name: "above max rejected", value: "24h1s", wantErr: true},
				{name: "garbage rejected", value: "abc", wantErr: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			for _, tc := range tt.cases {
				t.Run(tc.name, func(t *testing.T) {
					clearExternalUUIDBackfillEnv(t)
					t.Setenv(tt.envVar, tc.value)

					err := LoadExternalUUIDBackfillSettings()
					if tc.wantErr {
						require.Error(t, err)
						require.Contains(t, err.Error(), tt.envVar,
							"error must name the offending environment variable")
						return
					}

					require.NoError(t, err)
					require.Equal(t, normalizeDuration(tc.value), tt.observe(),
						"accepted value must be published to the package setting")
				})
			}
		})
	}
}

// TestLoadExternalUUIDBackfillAllowBlockingDDL asserts the blocking-DDL flag
// default, accepted spellings, and rejection of unparseable input.
func TestLoadExternalUUIDBackfillAllowBlockingDDL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr bool
	}{
		{name: "unset defaults to false", value: "", want: false},
		{name: "true accepted", value: "true", want: true},
		{name: "false accepted", value: "false", want: false},
		{name: "uppercase true accepted", value: "TRUE", want: true},
		{name: "numeric one accepted", value: "1", want: true},
		{name: "numeric zero accepted", value: "0", want: false},
		{name: "garbage rejected", value: "abc", wantErr: true},
		{name: "yes rejected", value: "yes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearExternalUUIDBackfillEnv(t)
			t.Setenv(EnvExternalUUIDBackfillAllowBlockingDDL, tt.value)

			err := LoadExternalUUIDBackfillSettings()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), EnvExternalUUIDBackfillAllowBlockingDDL)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, ExternalUUIDBackfillAllowBlockingDDL)
		})
	}
}

// TestLoadExternalUUIDBackfillSettingsIsAllOrNothing asserts that a rejected
// value leaves every package setting untouched.
func TestLoadExternalUUIDBackfillSettingsIsAllOrNothing(t *testing.T) {
	clearExternalUUIDBackfillEnv(t)
	require.NoError(t, LoadExternalUUIDBackfillSettings())

	// A valid row budget paired with an invalid DDL timeout must not publish the
	// row budget, because the load fails as a unit.
	t.Setenv(EnvExternalUUIDBackfillMaxRowsPerCycle, "2000")
	t.Setenv(EnvExternalUUIDBackfillDDLTimeout, "24h1s")

	err := LoadExternalUUIDBackfillSettings()
	require.Error(t, err)
	require.Contains(t, err.Error(), EnvExternalUUIDBackfillDDLTimeout)
	require.Equal(t, 10000, ExternalUUIDBackfillMaxRowsPerCycle,
		"a failed load must not partially apply settings")
}

// TestLoadExternalUUIDBackfillSettingsErrorMessage asserts the rejection message
// reports the variable, the offending value, and the permitted range.
func TestLoadExternalUUIDBackfillSettingsErrorMessage(t *testing.T) {
	clearExternalUUIDBackfillEnv(t)
	t.Setenv(EnvExternalUUIDBackfillMaxRowsPerCycle, "999")

	err := LoadExternalUUIDBackfillSettings()
	require.Error(t, err)

	message := err.Error()
	require.Contains(t, message, EnvExternalUUIDBackfillMaxRowsPerCycle)
	require.Contains(t, message, "999")
	require.Contains(t, message, "1000")
	require.Contains(t, message, "1000000")
	require.Contains(t, message, "inclusive")
}

// normalizeDuration renders a raw setting value the way the loaded setting prints
// it, so an accepted case can compare against the published value.
//
// Parameters:
//   - raw: the environment variable value used by the test case.
//
// Return values:
//   - string: canonical form of raw, or raw itself when it is not a duration.
func normalizeDuration(raw string) string {
	if parsed, err := time.ParseDuration(raw); err == nil && strings.ContainsAny(raw, "smhun") {
		return parsed.String()
	}
	return raw
}
