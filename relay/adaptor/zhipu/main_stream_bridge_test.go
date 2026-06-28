package zhipu

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

// zhipuUpstreamStream builds a canned Zhipu V3 SSE stream: one `data:<token>`
// frame per content token (the raw token text follows the `data:` prefix with no
// space, matching streamResponseZhipu2OpenAI which slices lineText[5:]), followed
// by a terminal `meta:<json>` usage frame, mirroring Zhipu's native wire format.
func zhipuUpstreamStream(tokens []string) io.ReadCloser {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString("data:")
		b.WriteString(tok)
		b.WriteString("\n\n")
	}
	// meta line carries usage; task_status terminates the stream.
	b.WriteString(`meta:{"request_id":"req-1","task_id":"task-1","task_status":"finish","usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`)
	b.WriteString("\n\n")
	return io.NopCloser(strings.NewReader(b.String()))
}

func newZhipuStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func newZhipuResponse(body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       body,
	}
}

// TestStreamHandler_RoutesThroughRewriter reproduces the bug class: a /v1/responses
// stream routed through the chat fallback installs a StreamRewriteHandler in the
// gin context, and the Zhipu V3 StreamHandler must route every chunk and the
// terminal event through that bridge rather than writing Chat-Completions SSE to
// the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newZhipuStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := newZhipuResponse(zhipuUpstreamStream([]string{"Hello", " world"}))

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.TotalTokens)

	// The bridge must have received the concatenated content deltas. The meta
	// frame produces an empty-content terminal chunk, so the concatenation is the
	// two content tokens.
	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	// Nothing raw must have leaked to the client: no chat.completion.chunk frame
	// and no Chat-Completions [DONE] sentinel.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
}

// TestStreamHandler_NoRewriterEmitsChatCompletionChunks verifies the no-bridge
// path is unchanged: without a rewriter the handler still emits OpenAI
// chat.completion.chunk SSE frames plus the [DONE] sentinel.
func TestStreamHandler_NoRewriterEmitsChatCompletionChunks(t *testing.T) {
	c, rec := newZhipuStreamCtx(t)

	resp := newZhipuResponse(zhipuUpstreamStream([]string{"Hello", " world"}))

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.TotalTokens)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
