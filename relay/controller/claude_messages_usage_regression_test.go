package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConvertedUsageRetainsMessageStartInput verifies that cumulative Claude output
// updates do not discard input usage reported only by message_start.
func TestConvertedUsageRetainsMessageStartInput(t *testing.T) {
	body := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n")
	usage := extractConvertedClaudeSSEUsage(body, 2, "gpt-4o-mini")
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestConvertedUsagePreservesOpenAICounters verifies that explicit OpenAI usage
// takes precedence over estimates even when no visible output text is present.
func TestConvertedUsagePreservesOpenAICounters(t *testing.T) {
	body := []byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19,\"total_tokens\":92}}\n\n")
	usage := extractConvertedClaudeSSEUsage(body, 2, "gpt-4o-mini")
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestConvertedUsageReadsNestedResponsesCounters verifies that a converted
// Responses completion event retains usage nested inside its response object.
func TestConvertedUsageReadsNestedResponsesCounters(t *testing.T) {
	body := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19,\"total_tokens\":92}}}\n\n")
	usage := extractConvertedClaudeSSEUsage(body, 2, "gpt-4o-mini")
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestConvertedUsageDoesNotSumCumulativeOutput verifies that repeated and later
// cumulative output counters are applied once, not added as token deltas.
func TestConvertedUsageDoesNotSumCumulativeOutput(t *testing.T) {
	body := []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73}}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n")
	usage := extractConvertedClaudeSSEUsage(body, 2, "gpt-4o-mini")
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}
