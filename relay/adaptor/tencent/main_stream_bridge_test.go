package tencent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/internal/streambridgetest"
)

// tencentStreamBody returns an io.ReadCloser that streams the given content
// tokens in Tencent's native SSE wire format: each frame is a `data: {json}`
// line carrying a ChatResponse whose Choices[0].Delta.Content holds the token.
// The final frame carries FinishReason "stop" to mirror the upstream end packet.
func tencentStreamBody(tokens []string) io.ReadCloser {
	var b strings.Builder
	for i, tok := range tokens {
		finish := ""
		if i == len(tokens)-1 {
			finish = `,"FinishReason":"stop"`
		}
		// Tencent stream frames carry the payload directly (not wrapped in
		// a Response envelope), matching streamResponseTencent2OpenAI's parse.
		b.WriteString(`data: {"Choices":[{"Delta":{"Content":"` + tok + `"}` + finish + `}]}`)
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

func newTencentStreamResp(tokens []string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       tencentStreamBody(tokens),
	}
}

func newTencentStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

// TestStreamHandler_RoutesThroughRewriter verifies that when the Response API
// fallback installs a rewriter, every Tencent stream token flows through
// HandleChunk and the terminal HandleDone/FinalizeUsage sequence is invoked,
// rather than raw chat-completion chunks (or a [DONE] sentinel) being flushed
// to the client. This reproduces the bug class and proves the fix.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newTencentStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := newTencentStreamResp([]string{"Hello", " world"})

	errResp, text := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.Equal(t, "Hello world", text)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	// With a rewriter present the bridge must fully intercept output: no raw
	// chat-completion chunk and no [DONE] sentinel may leak to the client.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies the no-bridge path still
// works: without a Response API rewriter the handler emits well-formed OpenAI
// chat.completion.chunk SSE frames plus a [DONE] sentinel.
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newTencentStreamCtx(t)

	resp := newTencentStreamResp([]string{"Hello", " world"})

	errResp, text := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.Equal(t, "Hello world", text)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
