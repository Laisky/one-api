package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureLogContentTopupFallback verifies that top-up logs receive a descriptive default when content is missing.
func TestEnsureLogContentTopupFallback(t *testing.T) {
	t.Parallel()
	logEntry := &Log{Type: LogTypeTopup, Quota: 500000}
	ensureLogContent(logEntry)
	require.NotEmpty(t, logEntry.Content)
	require.Contains(t, logEntry.Content, "Top-up event")
}

// TestBuildManageLogContentRedaction confirms that sensitive fields are redacted in manage logs.
func TestBuildManageLogContentRedaction(t *testing.T) {
	t.Parallel()
	content := buildManageLogContent("password", "secret123", "newSecret456", "actor=42")
	require.Contains(t, content, manageLogRedactedPlaceholder)
	require.NotContains(t, content, "secret123")
	require.Contains(t, content, "actor=42")
}

// TestBuildManageLogContentValues ensures non-sensitive field changes are captured verbatim.
func TestBuildManageLogContentValues(t *testing.T) {
	t.Parallel()
	content := buildManageLogContent("quota", 100, 200, "")
	require.Contains(t, content, "quota")
	require.Contains(t, content, "100")
	require.Contains(t, content, "200")
}
