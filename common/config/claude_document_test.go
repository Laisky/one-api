package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeNativeDocumentAllowanceConfiguration retains a strict positive
// operator-only contract, including unset defaults and overflow rejection.
func TestClaudeNativeDocumentAllowanceConfiguration(t *testing.T) {
	value, err := parseClaudeNativeDocumentTokenAllowance("", false)
	require.NoError(t, err)
	require.Equal(t, 32768, value)
	for _, raw := range []string{"", "0", "-1", "1048577", "9999999999999999999999999999", "1.5", "free"} {
		_, err := parseClaudeNativeDocumentTokenAllowance(raw, true)
		require.Error(t, err, raw)
	}
	for _, value := range []string{"1", "32768", "1048576", " 64000 "} {
		_, err := parseClaudeNativeDocumentTokenAllowance(value, true)
		require.NoError(t, err, value)
	}
}
