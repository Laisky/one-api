package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestResponseStreamBridge_AdaptorEndToEnd drives a real channel adaptor's
// StreamHandler (ali / DashScope) through the real chatToResponseStreamBridge —
// the exact wiring used when a /v1/responses streaming request is routed through
// the chat-completions fallback. It proves the client receives genuine Responses
// API SSE events (response.created / response.output_text.delta /
// response.completed) instead of raw chat-completion chunks, closing the gap
// left by the per-adaptor stub tests (which exercise a fake rewriter).
//
// Every retrofitted adaptor funnels through the same
// openai_compatible.RenderStreamChunkWithBridge + FinalizeStreamWithBridge
// helper, so this end-to-end check over one representative adaptor validates the
// shared mechanism for all of them.
func TestResponseStreamBridge_AdaptorEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	meta := &metalib.Meta{ActualModelName: "qwen-test"}
	request := &openai.ResponseAPIRequest{Model: "qwen-test"}
	bridge := newChatToResponseStreamBridge(c, meta, request)
	c.Set(ctxkey.ResponseStreamRewriteHandler, bridge)

	// Canned DashScope-native stream (same wire format ali.StreamHandler parses).
	frames := []string{
		`{"output":{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"null"}]},"usage":{"input_tokens":7,"output_tokens":1}}`,
		`{"output":{"choices":[{"message":{"role":"assistant","content":" world"},"finish_reason":"null"}]},"usage":{"input_tokens":7,"output_tokens":2}}`,
		`{"output":{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]},"usage":{"input_tokens":7,"output_tokens":2}}`,
	}
	var b strings.Builder
	for _, f := range frames {
		b.WriteString("data:")
		b.WriteString(f)
		b.WriteString("\n\n")
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(b.String())),
	}

	errResp, _ := ali.StreamHandler(c, resp)
	require.Nil(t, errResp)

	body := rec.Body.String()
	for _, want := range []string{
		"response.created",
		"response.output_text.delta",
		"response.output_text.done",
		"response.completed",
		"Hello",
		"world",
	} {
		require.Contains(t, body, want)
	}
	// The chat-completions chunk wire shape must NOT leak into a Responses stream.
	require.NotContains(t, body, `"object":"chat.completion.chunk"`)
	// A trailing `data: [DONE]` is the bridge's own legitimate stream terminator
	// (the Responses API stream ends with it). It must come AFTER the terminal
	// response.completed event, never as a substitute for the Responses events.
	if done := strings.Index(body, "data: [DONE]"); done != -1 {
		if completed := strings.Index(body, "response.completed"); completed == -1 || completed > done {
			require.Failf(t, "[DONE] order", "[DONE] not preceded by response.completed; body:\n%s", body)
		}
	}
}
