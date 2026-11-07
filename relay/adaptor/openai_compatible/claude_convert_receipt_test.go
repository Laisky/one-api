package openai_compatible

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// convertClaudeStreamUsageForTest runs ConvertOpenAIStreamToClaudeSSE over
// upstream SSE lines and returns the usage it settles from.
func convertClaudeStreamUsageForTest(t *testing.T, lines ...string) *relaymodel.Usage {
	t.Helper()
	c, _ := newGinTestContext()
	body := strings.Join(lines, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	return usage
}

// TestClaudeStreamResponsesReceiptSettlesUsage proves a Responses upstream,
// which never sends [DONE], is billed from the usage receipt of its terminal
// event like the native Responses relay, instead of being treated as an
// interrupted stream and over-billed from an estimate of the observed text
// (each output snapshot repeats the text). A stream that drops before any
// terminal receipt still settles conservatively as incomplete.
func TestClaudeStreamResponsesReceiptSettlesUsage(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("long answer ", 200)
	deltas := []string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"` + text + `"}`,
		`data: {"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"` + text + `"}`,
	}
	for _, terminal := range []string{"completed", "incomplete"} {
		t.Run(terminal, func(t *testing.T) {
			usage := convertClaudeStreamUsageForTest(t, append(deltas,
				`data: {"type":"response.`+terminal+`","response":{"id":"resp_1","object":"response","status":"`+terminal+`","incomplete_details":{"reason":"max_output_tokens"},"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"`+text+`"}]}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`)...)
			require.Equal(t, 3, usage.PromptTokens)
			require.Equal(t, 4, usage.CompletionTokens, "the terminal receipt is authoritative")
			require.Empty(t, usage.BillingEstimateReason)
		})
	}
	t.Run("dropped_before_terminal", func(t *testing.T) {
		usage := convertClaudeStreamUsageForTest(t, deltas...)
		require.Equal(t, "response_usage_incomplete_stream", usage.BillingEstimateReason)
		require.Greater(t, usage.CompletionTokens, 4, "an interrupted stream is never billed below the observed output")
	})
}
