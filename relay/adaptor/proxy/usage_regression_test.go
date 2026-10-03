package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestProxyUsageRetainsClaudeInput verifies that message_start input counters
// survive subsequent cumulative output-only message_delta updates.
func TestProxyUsageRetainsClaudeInput(t *testing.T) {
	body := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n"
	usage := proxyUsageFromResponse([]byte(body), &meta.Meta{PromptTokens: 2})
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestProxyUsageReadsNestedResponsesCounters verifies that response.completed
// events use the counters nested in response instead of silently returning zero.
func TestProxyUsageReadsNestedResponsesCounters(t *testing.T) {
	body := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19,\"total_tokens\":92}}}\n\n"
	usage := proxyUsageFromResponse([]byte(body), nil)
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestProxyUsageSurvivesLongSSELines verifies that a large but valid content
// event cannot hide the trailing authoritative usage event from the parser.
func TestProxyUsageSurvivesLongSSELines(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("x", 96*1024) + "\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19,\"total_tokens\":92}}\n\n"
	usage := proxyUsageFromResponse([]byte(body), nil)
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, 92, usage.TotalTokens)
}

// TestProxyUsageEstimatesOnlyMissingCounters verifies that an input-only usage
// block does not suppress fallback estimation of observed output content.
func TestProxyUsageEstimatesOnlyMissingCounters(t *testing.T) {
	usage := proxyUsageFromResponse([]byte(`{"choices":[{"message":{"content":"hello world"}}],"usage":{"prompt_tokens":73}}`), nil)
	require.Equal(t, 73, usage.PromptTokens)
	require.Positive(t, usage.CompletionTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}

// TestProxyUsageCountsToolOnlyOutput verifies that tool arguments are billable
// completion output even when the assistant does not emit visible message text.
func TestProxyUsageCountsToolOnlyOutput(t *testing.T) {
	usage := proxyUsageFromResponse([]byte(`{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{\"query\":\"Ottawa weather\"}"}}]}}]}`), &meta.Meta{PromptTokens: 73})
	require.Equal(t, 73, usage.PromptTokens)
	require.Positive(t, usage.CompletionTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
