package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaude431NativeThinking checks final native rewriting with immutable signed history and opaque integers.
func TestClaude431NativeThinking(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"between_tools", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			raw := []byte(`{"model":"claude-sonnet-5-5","thinking":{"type":"` + mode + `"},"max_tokens":64,"output_config":{"effort":"high","opaque":9007199254740993},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"<signed>&opaque"}]},{"role":"user","content":"Continue"}]}`)
			var request ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			sanitizeClaudeMessagesRequest(&request)
			got, stats, err := rewriteAndSanitizeClaudeRequestBody(raw, &request)
			require.NoError(t, err)
			require.Zero(t, stats.RemovedThinkingBlocks)
			var before, after map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &before))
			require.NoError(t, json.Unmarshal(got, &after))
			require.JSONEq(t, `{"type":"between_tools"}`, string(after["thinking"]))
			require.Equal(t, before["messages"], after["messages"])
			require.Equal(t, before["output_config"], after["output_config"])
			again, _, err := rewriteAndSanitizeClaudeRequestBody(got, &request)
			require.NoError(t, err)
			require.Equal(t, got, again)
		})
	}
}

// TestClaude431NativeInvalidControls proves native rewrites do not quietly discard unsupported instructions.
func TestClaude431NativeInvalidControls(t *testing.T) {
	t.Parallel()
	for _, extra := range []string{
		`"thinking":{"type":"between_tools","display":""}`,
		`"thinking":{"type":"between_tools","future_field":true}`,
		`"thinking":{"type":"between_tools"},"output_config":{"effort":"max"}`,
		`"tool_choice":{"type":"any"}`,
	} {
		t.Run(extra, func(t *testing.T) {
			t.Parallel()
			raw := []byte(`{"model":"claude-sonnet-5-5","max_tokens":64,"messages":[{"role":"user","content":"Hello"}],` + extra + `}`)
			var request ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			sanitizeClaudeMessagesRequest(&request)
			_, _, err := rewriteAndSanitizeClaudeRequestBody(raw, &request)
			require.ErrorContains(t, err, "validation failed")
		})
	}
}
