package ali

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

// aliStreamBody builds a canned DashScope-native streaming body in the exact
// wire format the StreamHandler parses: each frame is a `data:{json}` line where
// the JSON marshals to an ali ChatResponse (output.choices[].message.content +
// usage). The handler turns each frame into an openai.ChatCompletionsStreamResponse
// chunk via streamResponseAli2OpenAI.
func aliStreamBody(frames ...string) io.ReadCloser {
	var b strings.Builder
	for _, f := range frames {
		b.WriteString("data:")
		b.WriteString(f)
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

func newAliStreamResp(frames ...string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       aliStreamBody(frames...),
	}
}

func newAliStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

// Two content frames followed by a final frame carrying usage and a finish
// reason, mirroring DashScope's incremental_output stream.
var aliCannedFrames = []string{
	`{"output":{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"null"}]},"usage":{"input_tokens":7,"output_tokens":1}}`,
	`{"output":{"choices":[{"message":{"role":"assistant","content":" world"},"finish_reason":"null"}]},"usage":{"input_tokens":7,"output_tokens":2}}`,
	`{"output":{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]},"usage":{"input_tokens":7,"output_tokens":2}}`,
}

// TestStreamHandler_RoutesThroughRewriter proves the fix: when the /v1/responses
// chat fallback installs a rewrite bridge, every ali chunk flows through
// HandleChunk and the terminal FinalizeUsage+HandleDone sequence runs, with no
// raw chat-completion chunk or [DONE] sentinel leaking to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newAliStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	errResp, usage := StreamHandler(c, newAliStreamResp(aliCannedFrames...))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	body := rec.Body.String()
	require.NotContains(t, body, `"object":"chat.completion.chunk"`)
	require.NotContains(t, body, "data: [DONE]")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies the no-bridge path is
// unchanged: without a rewriter the handler still emits OpenAI
// chat.completion.chunk SSE frames plus the [DONE] sentinel.
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newAliStreamCtx(t)

	errResp, usage := StreamHandler(c, newAliStreamResp(aliCannedFrames...))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
