package coze

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

// cozeUpstreamStream builds a minimal canned Coze SSE stream carrying the given
// answer tokens. The wire format mirrors what StreamResponseCoze2OpenAI parses:
// each frame is a `data:{json}` line whose JSON is a StreamResponse with an
// "answer"-type message holding one content token.
func cozeUpstreamStream(tokens []string) string {
	var b strings.Builder
	for _, tok := range tokens {
		// {"event":"message","message":{"type":"answer","content":"<tok>"},"conversation_id":"conv-1"}
		b.WriteString(`data:{"event":"message","message":{"type":"answer","content":"`)
		b.WriteString(tok)
		b.WriteString(`"},"conversation_id":"conv-1"}`)
		b.WriteString("\n\n")
	}
	return b.String()
}

func newCozeStreamCtx(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *http.Response) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	return c, rec, resp
}

// TestStreamHandler_RoutesThroughRewriter verifies that when the Response API
// fallback installs a rewriter, every Coze answer token flows through
// HandleChunk and the terminal HandleDone is invoked, rather than raw
// chat-completion chunks / [DONE] being flushed to the client. This reproduces
// the bug class: before the fix the handler wrote chunks via render.ObjectData
// and terminated with render.Done, ignoring the bridge entirely.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec, resp := newCozeStreamCtx(t, cozeUpstreamStream([]string{"Hello", " world"}))
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	errResp, text := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, text)
	require.Equal(t, "Hello world", *text)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	// The bridge must fully intercept output: no raw chat-completion chunk and
	// no [DONE] sentinel may leak into the client response.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "[DONE]")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies that without a Response
// API rewriter the handler still emits well-formed OpenAI chat.completion.chunk
// SSE frames plus a [DONE] sentinel (the plain Chat Completions path).
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec, resp := newCozeStreamCtx(t, cozeUpstreamStream([]string{"Hello", " world"}))

	errResp, text := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, text)
	require.Equal(t, "Hello world", *text)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
