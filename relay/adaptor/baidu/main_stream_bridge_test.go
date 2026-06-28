package baidu

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

// baiduStreamBody builds a canned Baidu upstream SSE stream. Each frame mirrors
// the wire format the StreamHandler parses: a `data: {json}` line (the handler
// strips the leading 6 bytes `data: ` before json.Unmarshal into
// ChatStreamResponse). The final frame carries is_end=true plus usage.
func baiduStreamBody(tokens []string) io.ReadCloser {
	var b strings.Builder
	for i, tok := range tokens {
		isEnd := "false"
		usage := ""
		if i == len(tokens)-1 {
			isEnd = "true"
			usage = `,"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}`
		}
		b.WriteString(`data: {"id":"as-123","object":"chat.completion","created":1700000000,"result":"`)
		b.WriteString(tok)
		b.WriteString(`","is_end":` + isEnd + usage + "}")
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

func newBaiduStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func newBaiduStreamResp(tokens []string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       baiduStreamBody(tokens),
	}
}

// TestStreamHandler_RoutesThroughRewriter proves the bug-class fix: when the
// /v1/responses chat-fallback installs a Response API rewriter, every Baidu
// token must flow through HandleChunk and the terminal sequence (FinalizeUsage
// + HandleDone) must fire, with no raw chat-completion chunk or [DONE] sentinel
// leaking to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newBaiduStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := newBaiduStreamResp([]string{"Hello", " world"})
	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	// The bridge must fully intercept output: no raw chat-completion chunk nor
	// [DONE] sentinel may leak into the client response.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies the no-bridge path still
// behaves like a plain Chat Completions stream: the client receives
// chat.completion.chunk SSE frames with the expected content deltas and a final
// [DONE] sentinel.
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newBaiduStreamCtx(t)

	resp := newBaiduStreamResp([]string{"Hello", " world"})
	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.TotalTokens)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
