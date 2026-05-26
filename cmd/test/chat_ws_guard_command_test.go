package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseWSGuardArgs_DefaultsAttackOnly(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com", Token: "tok"}
	opts, err := parseWSGuardArgs([]string{}, cfg)
	require.NoError(t, err)
	require.Equal(t, "http://example.com/v1/responses", opts.apiEndpoint)
	require.Equal(t, "tok", opts.apiToken)
	require.Equal(t, defaultWSGuardBoundModel, opts.boundModel)
	require.Equal(t, defaultWSGuardAttackModel, opts.attackModel)
	require.Equal(t, defaultWSGuardTimeout, opts.timeout)
	require.True(t, opts.runAttack)
	require.False(t, opts.runHappyPath)
}

func TestParseWSGuardArgs_RejectsSameBoundAndAttackModel(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com", Token: "tok"}
	_, err := parseWSGuardArgs(
		[]string{"--bound-model", "gpt-5", "--attack-model", "gpt-5"},
		cfg,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must differ")
}

func TestParseWSGuardArgs_RejectsSkipAttackWithoutHappyPath(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com", Token: "tok"}
	_, err := parseWSGuardArgs([]string{"--skip-attack"}, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nothing to run")
}

func TestParseWSGuardArgs_HappyPathEnablesBothModes(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com", Token: "tok"}
	opts, err := parseWSGuardArgs([]string{"--happy-path"}, cfg)
	require.NoError(t, err)
	require.True(t, opts.runAttack)
	require.True(t, opts.runHappyPath)
}

func TestParseWSGuardArgs_RequiresToken(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com"}
	_, err := parseWSGuardArgs([]string{}, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "token")
}

func TestParseWSGuardArgs_TimeoutMustBePositive(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "http://example.com", Token: "tok"}
	_, err := parseWSGuardArgs([]string{"--timeout", "0s"}, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout")
}

func TestAppendQueryModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     string
		model    string
		expected string
	}{
		{"no query", "ws://h/v1/responses", "gpt-5-mini", "ws://h/v1/responses?model=gpt-5-mini"},
		{"existing query", "ws://h/v1/responses?foo=bar", "gpt-5-mini", "ws://h/v1/responses?foo=bar&model=gpt-5-mini"},
		{"empty model leaves url untouched", "ws://h/v1/responses", "", "ws://h/v1/responses"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, appendQueryModel(tt.base, tt.model))
		})
	}
}

func TestDecodeWSEvent(t *testing.T) {
	t.Parallel()

	ev, err := decodeWSEvent([]byte(`{"type":"error","error":{"code":"x"}}`))
	require.NoError(t, err)
	require.Equal(t, "error", ev["type"])

	_, err = decodeWSEvent([]byte(`{not json`))
	require.Error(t, err)
}

// guard against accidental regression of the time.Duration default.
func TestWSGuardConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, 30*time.Second, defaultWSGuardTimeout)
	require.Equal(t, "model_switch_denied", wsGuardErrorCode)
}
