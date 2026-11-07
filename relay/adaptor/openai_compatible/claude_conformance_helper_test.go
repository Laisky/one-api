package openai_compatible

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/common/claudeconformance"
)

// requireConformantClaudeStream validates raw Claude SSE output against the
// shared Anthropic Messages wire contract and returns the accumulated message.
func requireConformantClaudeStream(t *testing.T, raw string) claudeconformance.Message {
	t.Helper()
	report := claudeconformance.ValidateStream([]byte(raw))
	require.Empty(t, report.Violations, "stream violations:\n%s\nbody:\n%s", joinViolations(report.Violations), raw)
	return report.Message
}

// requireConformantClaudeMessage validates a non-streaming Claude Message body
// against the shared contract and returns the decoded message.
func requireConformantClaudeMessage(t *testing.T, raw []byte) claudeconformance.Message {
	t.Helper()
	report := claudeconformance.ValidateMessage(raw)
	require.Empty(t, report.Violations, "message violations:\n%s\nbody:\n%s", joinViolations(report.Violations), string(raw))
	return report.Message
}

// joinViolations renders violations one per line.
func joinViolations(violations []claudeconformance.Violation) string {
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		lines = append(lines, "  - "+v.String())
	}
	return strings.Join(lines, "\n")
}
