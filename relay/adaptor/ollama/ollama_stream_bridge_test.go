package ollama

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

// cannedOllamaStream returns an *http.Response whose body streams the given
// upstream Ollama chat frames. Ollama emits newline-delimited JSON objects (one
// ChatResponse per line); streamResponseOllama2OpenAI parses each line, so the
// canned stream mirrors that exact wire format: content tokens followed by a
// terminal frame carrying done=true plus the eval counts.
func cannedOllamaStream(tokens []string) *http.Response {
	var sb strings.Builder
	for _, tok := range tokens {
		// {"model":"...","message":{"role":"assistant","content":"<tok>"},"done":false}
		sb.WriteString(`{"model":"llama3","message":{"role":"assistant","content":"` + tok + `"},"done":false}` + "\n")
	}
	// Terminal frame: done=true with usage counters.
	sb.WriteString(`{"model":"llama3","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":7,"eval_count":11}` + "\n")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sb.String())),
		Header:     make(http.Header),
	}
}

func newOllamaStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

// TestStreamHandler_RoutesThroughRewriter verifies that when the Response API
// fallback installs a rewriter, every token flows through HandleChunk and the
// terminal sequence (FinalizeUsage + HandleDone) is invoked, rather than raw
// chat-completion chunks / [DONE] being flushed to the client. This reproduces
// the bug class: before the fix the handler wrote chunks via render.ObjectData
// and terminated with render.Done, bypassing the bridge entirely.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newOllamaStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := cannedOllamaStream([]string{"Hello", " world"})

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)
	require.NotNil(t, rw.Usage)
	require.Equal(t, 11, rw.Usage.CompletionTokens)
	require.Equal(t, 7, rw.Usage.PromptTokens)

	// The bridge must fully intercept output: no raw chat-completion chunk and
	// no [DONE] sentinel may leak into the client body.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "[DONE]")
	require.NotContains(t, body, "Hello world")
	require.NotContains(t, body, `"Hello"`)

	// Usage returned to the caller must still be computed normally.
	require.NotNil(t, usage)
	require.Equal(t, 11, usage.CompletionTokens)
	require.Equal(t, 7, usage.PromptTokens)
}

// TestStreamHandler_EmitsChatCompletionChunks verifies that without a rewriter
// the handler still emits well-formed OpenAI chat.completion.chunk SSE frames
// plus a [DONE] sentinel (the plain Chat Completions stream path is unchanged).
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newOllamaStreamCtx(t)

	resp := cannedOllamaStream([]string{"Hello", " world"})

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")

	require.NotNil(t, usage)
	require.Equal(t, 11, usage.CompletionTokens)
	require.Equal(t, 7, usage.PromptTokens)
}
