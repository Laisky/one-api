package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageReceipts verifies protocol counters independently of network
// chunk boundaries, repeated cumulative events, and malformed content recovery.
func TestResponseUsageReceipts(t *testing.T) {
	cases := []struct {
		name, body            string
		prompt, input, output int
	}{
		{"chat", `{"usage":{"prompt_tokens":73,"completion_tokens":19,"total_tokens":92}}`, 2, 73, 19},
		{"responses", `{"usage":{"input_tokens":73,"output_tokens":19,"total_tokens":92}}`, 2, 73, 19},
		{"claude", `{"type":"message","usage":{"input_tokens":73,"output_tokens":19}}`, 2, 73, 19},
		{"claude stream", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n", 2, 73, 19},
		{"responses stream", "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19,\"total_tokens\":92}}}\n\n", 2, 73, 19},
		{"cumulative", "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":7}}\n\ndata: {\"usage\":{\"completion_tokens\":19}}\n\ndata: {\"usage\":{\"completion_tokens\":19}}\n\n", 2, 73, 19},
		{"explicit zero", `{"usage":{"prompt_tokens":0,"completion_tokens":0},"choices":[{"message":{"content":"not a missing receipt"}}]}`, 99, 0, 0},
		{"multiline", "data: {\"usage\":{\ndata: \"prompt_tokens\":73,\"completion_tokens\":19}}\n\n", 2, 73, 19},
		{"CRLF", "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\r\n\r\n", 2, 73, 19},
		{"CR", "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\r\r", 2, 73, 19},
		{"no newline", "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}", 2, 73, 19},
		{"96 KiB event", "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("x", 96*1024) + "\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\n", 2, 73, 19},
		{"oversize recovery", "data: {\"x\":\"" + strings.Repeat("x", responseUsageCaptureLimit+10) + "\"}\n\ndata: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\n", 2, 73, 19},
		{"malformed recovery", "data: not-json\ndata: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\n", 2, 73, 19},
		{"total only", `{"usage":{"total_tokens":92}}`, 73, 73, 19},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1, 7, 64, 4096, len(tc.body)} {
				t.Run(fmt.Sprintf("chunk-%d", size), func(t *testing.T) {
					a := NewResponseUsageAccumulator(false)
					for off := 0; off < len(tc.body); off += size {
						a.Observe([]byte(tc.body[off:min(len(tc.body), off+size)]))
					}
					u := a.Finish(tc.prompt, nil)
					require.Equal(t, tc.input, u.PromptTokens)
					require.Equal(t, tc.output, u.CompletionTokens)
					require.Equal(t, tc.input+tc.output, u.TotalTokens)
					require.Equal(t, u, a.Finish(tc.prompt, nil))
					if tc.name != "total only" {
						require.Empty(t, u.BillingEstimateReason)
					}
				})
			}
		})
	}
}

// TestResponseUsageMissingOutput verifies that tools, reasoning and nested
// snapshots remain chargeable without a provider completion counter.
func TestResponseUsageMissingOutput(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"content":"hello world"}}],"usage":{"prompt_tokens":73}}`,
		`{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{\"query\":\"Ottawa weather\"}"}}]}}]}`,
		`{"response":{"output":[{"type":"message","content":[{"type":"output_text","text":"nested generated output"}]}]}}`,
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hidden reasoning work\"}}\n\n",
		"data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"large generated arguments\"}\n\n",
	} {
		u := ParseResponseUsage([]byte(body), 73, nil)
		require.Equal(t, 73, u.PromptTokens, body)
		require.Positive(t, u.CompletionTokens, body)
		require.Equal(t, u.PromptTokens+u.CompletionTokens, u.TotalTokens)
		require.NotEmpty(t, u.BillingEstimateReason)
	}
}

// TestResponseUsageCacheDimensions verifies canonical prompt cache accounting
// and preservation of partial reasoning/audio details across stream receipts.
func TestResponseUsageCacheDimensions(t *testing.T) {
	body := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":20,\"cache_creation_input_tokens\":30,\"cache_creation\":{\"ephemeral_5m_input_tokens\":12,\"ephemeral_1h_input_tokens\":18}}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":9}}\n\n"
	u := ParseResponseUsage([]byte(body), 1, nil)
	require.Equal(t, 60, u.PromptTokens)
	require.Equal(t, 9, u.CompletionTokens)
	require.Equal(t, 12, u.CacheWrite5mTokens)
	require.Equal(t, 18, u.CacheWrite1hTokens)
	require.NotNil(t, u.PromptTokensDetails)
	require.Equal(t, 20, u.PromptTokensDetails.CachedTokens)
	body = "data: {\"usage\":{\"prompt_tokens\":73,\"prompt_tokens_details\":{\"cached_tokens\":30},\"completion_tokens_details\":{\"reasoning_tokens\":19}}}\n\ndata: {\"usage\":{\"completion_tokens\":19,\"prompt_tokens_details\":{\"audio_tokens\":2}}}\n\n"
	u = ParseResponseUsage([]byte(body), 1, nil)
	require.Equal(t, 73, u.PromptTokens)
	require.Equal(t, 19, u.CompletionTokens)
	require.Equal(t, 30, u.PromptTokensDetails.CachedTokens)
	require.Equal(t, 2, u.PromptTokensDetails.AudioTokens)
	require.NotNil(t, u.CompletionTokensDetails)
	require.Equal(t, 19, u.CompletionTokensDetails.ReasoningTokens)
}

// TestResponseUsageInterrupted verifies that a provisional output counter cannot
// suppress observed work and that settlement receives an explicit estimate flag.
func TestResponseUsageInterrupted(t *testing.T) {
	a := NewResponseUsageAccumulator(true)
	a.Observe([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"" + strings.Repeat("generated ", 100) + "\"}}\n\n"))
	a.MarkIncomplete()
	u := a.Finish(1, nil)
	require.Equal(t, 73, u.PromptTokens)
	require.Greater(t, u.CompletionTokens, 1)
	require.Equal(t, "response_usage_incomplete_stream", u.BillingEstimateReason)
}

// TestResponseUsageCaptureBound verifies that retained payload memory is bounded
// even when an upstream supplies an arbitrarily long line or JSON response.
func TestResponseUsageCaptureBound(t *testing.T) {
	for _, stream := range []bool{false, true} {
		a := NewResponseUsageAccumulator(stream)
		chunk := []byte(strings.Repeat("x", 32*1024))
		if !stream {
			a.Observe([]byte("{"))
		}
		for i := 0; i < 128; i++ {
			a.Observe(chunk)
		}
		require.LessOrEqual(t, len(a.body), responseUsageCaptureLimit)
		require.LessOrEqual(t, len(a.line), responseUsageCaptureLimit)
		require.LessOrEqual(t, len(a.event), responseUsageCaptureLimit)
		u := a.Finish(73, nil)
		require.Positive(t, u.CompletionTokens)
		require.NotEmpty(t, u.BillingEstimateReason)
	}
}
