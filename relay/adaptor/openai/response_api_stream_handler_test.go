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

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestResponseAPIStreamHandler_NoDuplicate ensures delta events + done events do not create duplicate final chunks.
func TestResponseAPIStreamHandler_NoDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	sse := `event: response.created
data: {"type":"response.created","response":{"id":"resp_test_dup","object":"response","created_at":1741290958,"status":"in_progress"}}

event: response.output_item.added
data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_test_dup","type":"message","status":"in_progress","role":"assistant","content":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_test_dup","output_index":0,"content_index":0,"delta":"Hello"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_test_dup","output_index":0,"content_index":0,"delta":" world"}

event: response.output_text.done
data: {"type":"response.output_text.done","item_id":"msg_test_dup","output_index":0,"content_index":0,"text":"Hello world"}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_test_dup","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello world","annotations":[]}]}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_test_dup","object":"response","created_at":1741290958,"status":"completed","output":[{"id":"msg_test_dup","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello world","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}

data: [DONE]`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	err, aggregatedText, usage := ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
	require.Nil(t, err, "unexpected error")
	require.Equal(t, "Hello world", aggregatedText, "unexpected aggregated text")
	require.NotNil(t, usage, "usage should not be nil")
	require.Equal(t, 3, usage.TotalTokens, "unexpected total tokens")

	// Inspect emitted stream and ensure only one full-text chunk and one usage chunk
	body := w.Body.String()
	require.Contains(t, body, "data: [DONE]", "missing DONE in stream output")

	t.Logf("stream body:\n%s", body)

	usageChunks := 0
	finishCount := 0
	var combined strings.Builder
	fullTextChunks := 0

	for part := range strings.SplitSeq(body, "\n\n") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(part, "data: ")
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// ignore non-json payloads
			continue
		}
		if chunk.Usage != nil {
			usageChunks++
		}
		if len(chunk.Choices) > 0 {
			ch := chunk.Choices[0]
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				finishCount++
			}
			if content, ok := ch.Delta.Content.(string); ok && strings.TrimSpace(content) != "" {
				if strings.TrimSpace(content) == "Hello world" {
					fullTextChunks++
				}
				combined.WriteString(content)
			}
		}
	}

	// No duplicate full-text chunks; zero or one is acceptable. Deltas should
	// reconstruct the expected final text.
	require.LessOrEqual(t, fullTextChunks, 1, "expected at most 1 full-text chunk")
	require.Equal(t, "Hello world", strings.TrimSpace(combined.String()), "combined delta content mismatch")
	require.Equal(t, 1, usageChunks, "expected exactly 1 usage chunk")
	require.Equal(t, 1, finishCount, "expected exactly 1 finish_reason present")
}

func TestResponseAPIStreamHandlerWebSearchUsageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	sse := `event: response.completed
data: {"type":"response.completed","response":{"id":"resp_ws","object":"response","created_at":1,"status":"completed","output":[{"id":"msg_ws","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"web_search":{"requests":3}}}}}

data: [DONE]`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	err, _, usage := ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
	require.Nil(t, err, "unexpected error")
	require.NotNil(t, usage, "usage should not be nil")
	require.Equal(t, 7, usage.TotalTokens, "unexpected total tokens")

	countRaw, exists := c.Get(ctxkey.WebSearchCallCount)
	require.True(t, exists, "expected web search count in context")
	count, ok := countRaw.(int)
	require.True(t, ok, "web search count should be int")
	require.Equal(t, 3, count, "expected web search count 3")
}
