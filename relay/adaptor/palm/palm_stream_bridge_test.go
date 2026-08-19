package palm

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

// palmUpstreamBody returns a canned PaLM generateMessage response body. The PaLM
// StreamHandler reads the whole upstream body and unmarshals it as a single JSON
// ChatResponse (PaLM is not a true SSE upstream), so a single JSON object with a
// candidate carrying the content token is the minimal reproduction input.
func palmUpstreamBody(content string) string {
	return `{"candidates":[{"author":"1","content":"` + content + `"}]}`
}

func newPalmStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func newPalmResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestStreamHandler_RoutesThroughRewriter verifies that when the Response API
// fallback installs a rewriter, the PaLM content token flows through HandleChunk
// and the terminal HandleDone/FinalizeUsage are invoked, rather than a raw
// chat-completion chunk or [DONE] sentinel leaking to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newPalmStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	errResp, text := StreamHandler(c, newPalmResponse(palmUpstreamBody("Hello world")))
	require.Nil(t, errResp)
	require.Equal(t, "Hello world", text)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	body := rec.Body.String()
	// The bridge must fully intercept output: no raw chat-completion chunk and no
	// chat-completions [DONE] sentinel should leak to the client.
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies that without a Response API
// rewriter the handler still emits a well-formed OpenAI chat.completion.chunk SSE
// frame plus a [DONE] sentinel (the unchanged plain Chat Completions path).
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newPalmStreamCtx(t)

	errResp, text := StreamHandler(c, newPalmResponse(palmUpstreamBody("Hello world")))
	require.Nil(t, errResp)
	require.Equal(t, "Hello world", text)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello world"`)
	require.Contains(t, body, "data: [DONE]")
}
