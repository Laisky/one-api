package gemini

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

// geminiCachedTurn answers "Hello world" with a 1000-token prompt whose first
// 800 tokens were served from Gemini's context cache.
const geminiCachedTurn = `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello world"}]},"finishReason":"STOP","index":0}],` +
	`"usageMetadata":{"promptTokenCount":1000,"candidatesTokenCount":40,"totalTokenCount":1040,"cachedContentTokenCount":800}}`

// geminiRecordedReceipt returns the converted response settlement receipt.
func geminiRecordedReceipt(t *testing.T, c *gin.Context) *model.Usage {
	t.Helper()
	raw, exists := c.Get(ctxkey.ConvertedResponseUsage)
	require.True(t, exists, "the converter must record the receipt key")
	receipt, ok := raw.(*model.Usage)
	require.True(t, ok, "unexpected receipt type %T", raw)
	return receipt
}

// TestGeminiClaudeNonStreamUsageSemantics verifies the non-stream Gemini
// Claude rendering reports the cached prefix as cache_read_input_tokens with
// input_tokens excluding it (promptTokenCount includes cached content), while
// the recorded receipt keeps the counters billing settled on before.
func TestGeminiClaudeNonStreamUsageSemantics(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 12}
	converted, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, []byte(geminiCachedTurn), info)
	require.Nil(t, failure)
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	var claude model.ClaudeResponse
	require.NoError(t, json.Unmarshal(body, &claude))
	require.Equal(t, model.ClaudeUsage{InputTokens: 200, OutputTokens: 40, CacheReadInputTokens: 800}, claude.Usage)
	require.Equal(t, &model.Usage{PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040,
		PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 800}}, geminiRecordedReceipt(t, c))
}

// TestGeminiClaudeNonStreamUnmeasuredPromptHasNoCache verifies a receipt that
// did not measure the prompt cannot attribute a cached prefix to the gateway
// estimate that replaces it.
func TestGeminiClaudeNonStreamUnmeasuredPromptHasNoCache(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 12}
	turn := strings.Replace(geminiCachedTurn, `"promptTokenCount":1000,`, ``, 1)
	_, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, []byte(turn), info)
	require.Nil(t, failure)
	receipt := geminiRecordedReceipt(t, c)
	require.Equal(t, 12, receipt.PromptTokens)
	require.Nil(t, receipt.PromptTokensDetails)
}

// TestGeminiClaudeStreamUsageSemantics verifies the buffered Gemini stream
// rendering presents the final receipt in Anthropic semantics in both
// message_start and the cumulative message_delta, and records the receipt.
func TestGeminiClaudeStreamUsageSemantics(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 12}
	converted, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, []byte("data: "+geminiCachedTurn+"\r\n\r\n"), info)
	require.Nil(t, failure)
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)

	usageByEvent := map[string]map[string]any{}
	for line := range strings.SplitSeq(string(body), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &event))
		switch event["type"] {
		case "message_start":
			message, _ := event["message"].(map[string]any)
			usageByEvent["message_start"], _ = message["usage"].(map[string]any)
		case "message_delta":
			usageByEvent["message_delta"], _ = event["usage"].(map[string]any)
		}
	}
	for _, name := range []string{"message_start", "message_delta"} {
		usage := usageByEvent[name]
		require.NotNil(t, usage, name)
		require.EqualValues(t, 200, usage["input_tokens"], name)
		require.EqualValues(t, 800, usage["cache_read_input_tokens"], name)
	}
	require.EqualValues(t, 0, usageByEvent["message_start"]["output_tokens"])
	require.EqualValues(t, 40, usageByEvent["message_delta"]["output_tokens"])
	require.Equal(t, &model.Usage{PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040,
		PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 800}}, geminiRecordedReceipt(t, c))
}
