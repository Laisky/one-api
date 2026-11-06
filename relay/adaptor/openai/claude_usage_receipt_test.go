package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// claudeUsageReceiptContext returns a request context for the mapped model.
func claudeUsageReceiptContext(actualModel string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: actualModel})
	return c
}

// recordedClaudeReceipt returns the converted response settlement receipt.
func recordedClaudeReceipt(t *testing.T, c *gin.Context) *model.Usage {
	t.Helper()
	raw, exists := c.Get(ctxkey.ConvertedResponseUsage)
	require.True(t, exists, "the converter must record the receipt key")
	receipt, ok := raw.(*model.Usage)
	require.True(t, ok, "unexpected receipt type %T", raw)
	return receipt
}

// decodeClaudeUsage reads the usage object of a converted Claude body.
func decodeClaudeUsage(t *testing.T, resp *http.Response) model.ClaudeUsage {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var claude model.ClaudeResponse
	require.NoError(t, json.Unmarshal(body, &claude), "body: %s", string(body))
	return claude.Usage
}

// TestOpenAIClaudeNonStreamChatRecordsCacheReceipt verifies the OpenAI
// adaptor's non-stream Chat conversion settles on the cache-aware receipt the
// native Chat handler bills (top-level cached_tokens and aggregate cache writes
// normalized), and presents it in Anthropic semantics. Previously the cache
// fields were dropped, billing cached input at the full input price.
func TestOpenAIClaudeNonStreamChatRecordsCacheReceipt(t *testing.T) {
	t.Parallel()
	c := claudeUsageReceiptContext("gpt-4o-mini")
	body := `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":1000,"completion_tokens":40,"total_tokens":1040,"cached_tokens":800,"cache_write_tokens":100}}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}}
	converted, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, resp, []byte(body))
	require.Nil(t, failure)
	require.Equal(t, model.ClaudeUsage{InputTokens: 100, OutputTokens: 40, CacheReadInputTokens: 800, CacheCreationInputTokens: 100,
		CacheCreation: &model.ClaudeCacheCreation{Ephemeral5mInputTokens: 100}}, decodeClaudeUsage(t, converted))
	require.Equal(t, &model.Usage{PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040, CacheWrite5mTokens: 100,
		PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 800}}, recordedClaudeReceipt(t, c))
}

// TestOpenAIClaudeNonStreamResponsesRecordsCacheReceipt verifies the Responses
// conversion settles on the same counters the native Responses handler bills
// (ToModelUsage, which folds input_tokens_details.cache_write_tokens into the
// 5m bucket) and presents them in Anthropic semantics.
func TestOpenAIClaudeNonStreamResponsesRecordsCacheReceipt(t *testing.T) {
	t.Parallel()
	c := claudeUsageReceiptContext("gpt-4o")
	response := &ResponseAPIResponse{
		Id: "resp_1", Object: "response", Model: "gpt-4o", Status: "completed",
		Output: []OutputItem{{Id: "msg_1", Type: "message", Role: "assistant", Content: []OutputContent{{Type: "output_text", Text: "hi"}}}},
		Usage: &ResponseAPIUsage{InputTokens: 1000, OutputTokens: 40, TotalTokens: 1040,
			InputTokensDetails: &ResponseAPIInputTokensDetails{CachedTokens: 600, CacheWriteTokens: 200}},
	}
	converted, failure := (&Adaptor{}).ConvertResponseAPIToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, response)
	require.Nil(t, failure)
	require.Equal(t, model.ClaudeUsage{InputTokens: 200, OutputTokens: 40, CacheReadInputTokens: 600, CacheCreationInputTokens: 200,
		CacheCreation: &model.ClaudeCacheCreation{Ephemeral5mInputTokens: 200}}, decodeClaudeUsage(t, converted))
	require.Equal(t, &model.Usage{PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040, CacheWrite5mTokens: 200,
		PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 600}}, recordedClaudeReceipt(t, c))
}

// TestOpenAIClaudeConversionReceiptAbsentCases verifies conversions without
// measured counters record a nil receipt (the controller then keeps its
// historical body-parsing fallback), including the buffered SSE rendering and
// an unparseable body, and that a stale receipt never survives them.
func TestOpenAIClaudeConversionReceiptAbsentCases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, contentType, body string }{
		{name: "chat_without_usage", contentType: "application/json", body: `{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"message":{"content":"hi"},"finish_reason":"stop"}]}`},
		{name: "responses_without_usage", contentType: "application/json", body: `{"id":"resp_1","object":"response","model":"m","status":"completed","output":[]}`},
		{name: "unparseable", contentType: "application/json", body: `not json`},
		{name: "buffered_sse", contentType: "text/event-stream", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := claudeUsageReceiptContext("gpt-4o")
			c.Set(ctxkey.ConvertedResponseUsage, &model.Usage{PromptTokens: 999999, CompletionTokens: 1})
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{tc.contentType}}, Body: io.NopCloser(strings.NewReader(tc.body))}
			converted, failure := (&Adaptor{}).convertToClaudeResponse(c, resp)
			require.Nil(t, failure)
			_, err := io.Copy(io.Discard, converted.Body)
			require.NoError(t, err)
			require.Nil(t, recordedClaudeReceipt(t, c))
		})
	}
}
