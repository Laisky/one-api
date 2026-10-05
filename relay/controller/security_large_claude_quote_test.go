package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurityLargeClaudeCanonicalQuote rejects the old discontinuous byte/4 admission shortcut.
func TestSecurityLargeClaudeCanonicalQuote(t *testing.T) {
	for _, n := range []int{280000, 260000, 262144} {
		raw, err := json.Marshal(map[string]any{"model": "claude-sonnet-4", "max_tokens": 1, "messages": []any{map[string]any{"role": "user", "content": strings.Repeat("😀", n)}}})
		require.NoError(t, err)
		var request ClaudeMessagesRequest
		require.NoError(t, json.Unmarshal(raw, &request))
		expected := requireClaudePromptTokens(t, context.Background(), &request)
		require.Positive(t, expected)
		for _, size := range []int{len(raw), len(raw) + (1 << 20)} {
			require.Equal(t, expected, requireClaudePromptEstimate(t, context.Background(), &request, size), "JSON whitespace/size must not lower a semantic quote")
		}
	}
}

// BenchmarkSecurityLargeClaudeCanonicalQuote measures real tokenizer work on bounded high-density input.
func BenchmarkSecurityLargeClaudeCanonicalQuote(b *testing.B) {
	raw, err := json.Marshal(map[string]any{"model": "claude-sonnet-4", "max_tokens": 1, "messages": []any{map[string]any{"role": "user", "content": strings.Repeat("😀", 280000)}}})
	require.NoError(b, err)
	var request ClaudeMessagesRequest
	require.NoError(b, json.Unmarshal(raw, &request))
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requireClaudePromptEstimate(b, context.Background(), &request, len(raw))
	}
}
