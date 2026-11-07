package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// convertedClaudeReceipt returns the settlement receipt the converter recorded.
func convertedClaudeReceipt(t *testing.T, raw any, exists bool) *relaymodel.Usage {
	t.Helper()
	require.True(t, exists, "the converter must record the receipt key")
	receipt, ok := raw.(*relaymodel.Usage)
	require.True(t, ok, "the receipt must be a *relaymodel.Usage, got %T", raw)
	return receipt
}

// TestConvertOpenAIResponseToClaudeResponseUsageSemantics verifies the shared
// non-stream converter presents Chat and Responses receipts in Anthropic
// semantics (input_tokens excludes cache reads and writes) while recording the
// unchanged OpenAI-semantics counters as the server-only settlement receipt.
func TestConvertOpenAIResponseToClaudeResponseUsageSemantics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body string }{
		{name: "chat", body: `{"id":"chatcmpl-1","object":"chat.completion","model":"llama-3","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1000,"completion_tokens":40,"total_tokens":1040,"prompt_tokens_details":{"cached_tokens":600},"cache_write_5m_tokens":150,"cache_write_1h_tokens":50}}`},
		{name: "responses", body: `{"id":"resp_1","object":"response","model":"llama-3","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],` +
			`"usage":{"input_tokens":1000,"output_tokens":40,"total_tokens":1040,"input_tokens_details":{"cached_tokens":600},"cache_write_5m_tokens":150,"cache_write_1h_tokens":50}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newGinTestContext()
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "llama-3"})
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			converted, failure := ConvertOpenAIResponseToClaudeResponse(c, resp)
			require.Nil(t, failure)
			body, err := io.ReadAll(converted.Body)
			require.NoError(t, err)
			var claude relaymodel.ClaudeResponse
			require.NoError(t, json.Unmarshal(body, &claude))
			require.Equal(t, relaymodel.ClaudeUsage{InputTokens: 200, OutputTokens: 40, CacheReadInputTokens: 600, CacheCreationInputTokens: 200,
				CacheCreation: &relaymodel.ClaudeCacheCreation{Ephemeral5mInputTokens: 150, Ephemeral1hInputTokens: 50}}, claude.Usage)

			raw, exists := c.Get(ctxkey.ConvertedResponseUsage)
			require.Equal(t, &relaymodel.Usage{PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040, CacheWrite5mTokens: 150, CacheWrite1hTokens: 50,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 600}}, convertedClaudeReceipt(t, raw, exists))
		})
	}
}

// TestConvertOpenAIResponseToClaudeResponseClaudeModelKeepsInput verifies a
// Claude model served through an OpenAI-compatible upstream keeps input_tokens
// as reported: such receipts (one-api's own Claude convention) already exclude
// the cache buckets, and billing interprets them the same way.
func TestConvertOpenAIResponseToClaudeResponseClaudeModelKeepsInput(t *testing.T) {
	t.Parallel()
	c, _ := newGinTestContext()
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "claude-sonnet-4-5"})
	body := `{"id":"chatcmpl-1","model":"claude-sonnet-4-5","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":300,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":200}}}`
	converted, failure := ConvertOpenAIResponseToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, failure)
	out, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	var claude relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(out, &claude))
	require.Equal(t, 300, claude.Usage.InputTokens)
	require.Equal(t, 200, claude.Usage.CacheReadInputTokens)
}

// TestConvertOpenAIResponseToClaudeResponseReceiptLifecycle verifies the
// receipt always describes the latest conversion: an upstream without counters
// records nil (the controller keeps its estimate fallback), and an unknown
// payload clears a stale receipt.
func TestConvertOpenAIResponseToClaudeResponseReceiptLifecycle(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`,
		`{"id":"resp_1","object":"response","model":"m","status":"completed","output":[]}`,
		`{"error":{"message":"bad gateway"}}`,
	} {
		c, _ := newGinTestContext()
		c.Set(ctxkey.ConvertedResponseUsage, &relaymodel.Usage{PromptTokens: 999999})
		_, failure := ConvertOpenAIResponseToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
		require.Nil(t, failure)
		raw, exists := c.Get(ctxkey.ConvertedResponseUsage)
		require.Nil(t, convertedClaudeReceipt(t, raw, exists), "body %s must not leave a stale receipt", body)
	}
	require.NotPanics(t, func() { SetConvertedClaudeUsage(nil, &relaymodel.Usage{PromptTokens: 1}) })
}

// TestConvertOpenAIStreamToClaudeSSEResponsesUsageSemantics verifies the
// streamed final message_delta of a Responses upstream presents the cached
// prefix in cache_read_input_tokens without double-counting it in
// input_tokens, while the returned billing usage keeps OpenAI semantics.
func TestConvertOpenAIStreamToClaudeSSEResponsesUsageSemantics(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()
	wire := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"hi\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-4o\",\"output\":[]," +
		"\"usage\":{\"input_tokens\":1000,\"input_tokens_details\":{\"cached_tokens\":800},\"output_tokens\":40,\"total_tokens\":1040}}}\n\n"
	usage, failure := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(wire))}, 12, "gpt-4o")
	require.Nil(t, failure)
	require.Equal(t, 1000, usage.PromptTokens)
	require.Equal(t, 800, usage.PromptTokensDetails.CachedTokens)

	var final map[string]any
	for line := range strings.SplitSeq(w.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &event))
		if event["type"] == "message_delta" {
			final, _ = event["usage"].(map[string]any)
		}
	}
	require.NotNil(t, final, "stream must end with a message_delta carrying usage")
	require.EqualValues(t, 200, final["input_tokens"])
	require.EqualValues(t, 800, final["cache_read_input_tokens"])
	require.EqualValues(t, 40, final["output_tokens"])
}
