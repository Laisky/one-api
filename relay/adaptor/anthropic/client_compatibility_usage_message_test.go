package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilitySharedMessageUsage checks the native MCP parser uses
// the same canonical iteration and invalid-receipt rules as HTTP handlers.
func TestClientCompatibilitySharedMessageUsage(t *testing.T) {
	t.Parallel()
	usage, err := ParseClaudeMessageUsage([]byte(`{"type":"message","usage":{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}}`))
	require.NoError(t, err)
	require.Equal(t, 144, usage.PromptTokens)
	require.Equal(t, 276, usage.CompletionTokens)
	require.Empty(t, usage.BillingEstimateReason)
	incomplete, err := ParseClaudeMessageUsage([]byte(`{"type":"message","usage":{"input_tokens":11}}`))
	require.NoError(t, err)
	require.Equal(t, 11, incomplete.PromptTokens)
	require.Equal(t, "incomplete_usage_after_accepted_claude_http", incomplete.BillingEstimateReason)
	for _, raw := range []string{
		`{"type":"message","usage":{"input_tokens":11,"output_tokens":-1}}`,
		`{"type":"message","usage":{"input_tokens":9223372036854775807,"output_tokens":1}}`,
		`{"type":"error","error":{"type":"overloaded_error"}}`,
	} {
		rejected, err := ParseClaudeMessageUsage([]byte(raw))
		require.Error(t, err)
		require.Nil(t, rejected)
	}
}
