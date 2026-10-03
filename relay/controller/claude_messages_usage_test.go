package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractConvertedClaudeSSEUsagePrefersClaudeMessageDeltaUsage verifies converted Claude SSE
// billing uses explicit output token metadata instead of dropping completion tokens.
func TestExtractConvertedClaudeSSEUsagePrefersClaudeMessageDeltaUsage(t *testing.T) {
	body := []byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"gemini-pro","content":[],"usage":{"input_tokens":4,"output_tokens":0}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":4,"output_tokens":9,"total_tokens":13}}

data: [DONE]

`)

	usage := extractConvertedClaudeSSEUsage(body, 1, "gemini-pro")

	require.NotNil(t, usage)
	require.Equal(t, 4, usage.PromptTokens)
	require.Equal(t, 9, usage.CompletionTokens)
	require.Equal(t, 13, usage.TotalTokens)
}

// TestExtractConvertedClaudeSSEUsageFallsBackToClaudeText verifies converted Claude SSE billing
// counts content_block_delta text when explicit usage metadata is absent.
func TestExtractConvertedClaudeSSEUsageFallsBackToClaudeText(t *testing.T) {
	body := []byte(`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

data: [DONE]

`)

	usage := extractConvertedClaudeSSEUsage(body, 7, "gemini-pro")

	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}

// TestExtractConvertedClaudeSSEUsagePreservesOpenAIStreamParsing verifies the shared helper still
// extracts OpenAI-compatible choices delta content for converted streaming responses.
func TestExtractConvertedClaudeSSEUsagePreservesOpenAIStreamParsing(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":[{\"type\":\"text\",\"text\":\" world\"}]}}]}\n" +
		"data: [DONE]\n")

	usage := extractConvertedClaudeSSEUsage(body, 3, "gpt-4o-mini")

	require.NotNil(t, usage)
	require.Equal(t, 3, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
