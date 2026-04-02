package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestStreamHandler_ClientContextCanceledReturnsUsage verifies Anthropic streaming keeps
// backward-compatible return values when the downstream client disconnects before data arrives.
// Parameters:
//   - t: the test context.
//
// Returns:
//   - nothing.
func TestStreamHandler_ClientContextCanceledReturnsUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-stream-test"))

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
	})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Contains(t, recorder.Body.String(), "[DONE]")
}

// TestClaudeNativeStreamHandler_OversizedDataLineForwarded verifies large native data lines stream correctly.
func TestClaudeNativeStreamHandler_OversizedDataLineForwarded(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-stream-test"))

	largeText := strings.Repeat("z", 128*1024)
	sse := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"usage\":{\"input_tokens\":3,\"cache_read_input_tokens\":1}}\n" +
		"\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"" + largeText + "\"}}\n" +
		"\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":3,\"output_tokens\":4}}\n" +
		"\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}

	errResp, usage := ClaudeNativeStreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 1, usage.PromptTokensDetails.CachedTokens)

	body := recorder.Body.String()
	require.Contains(t, body, largeText[:1024])
	require.Contains(t, body, largeText[len(largeText)-1024:])
	// Anthropic native streams should NOT emit [DONE] (that's an OpenAI convention)
	require.NotContains(t, body, "[DONE]")
	// Verify event type lines are emitted
	require.Contains(t, body, "event: message_start\n")
	require.Contains(t, body, "event: content_block_delta\n")
	require.Contains(t, body, "event: message_delta\n")
}

// TestStreamHandler_OversizedConvertedChunk verifies oversized Anthropic deltas convert to OpenAI chunks.
func TestStreamHandler_OversizedConvertedChunk(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-stream-test"))

	largeText := strings.Repeat("q", 128*1024)
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_big","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":7,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + largeText + `"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`,
	}, "\n\n") + "\n\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sse)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 9, usage.CompletionTokens)

	body := recorder.Body.String()
	require.Contains(t, body, largeText[:1024])
	require.Contains(t, body, largeText[len(largeText)-1024:])
	require.Contains(t, body, "data: [DONE]")
}
