package cohere

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

// cohereUpstreamStream returns a canned Cohere streaming body. Cohere emits
// newline-delimited JSON objects (one per SSE line, no "event:"/"data:" prefix):
// a stream-start, one text-generation per token, then a stream-end carrying
// usage + finish reason. This mirrors the wire format parsed by
// StreamResponseCohere2OpenAI / StreamHandler.
func cohereUpstreamStream(tokens []string) string {
	var b strings.Builder
	b.WriteString(`{"is_finished":false,"event_type":"stream-start","generation_id":"gen-1"}` + "\n")
	for _, tok := range tokens {
		b.WriteString(`{"is_finished":false,"event_type":"text-generation","text":"` + tok + `"}` + "\n")
	}
	b.WriteString(`{"is_finished":true,"event_type":"stream-end","finish_reason":"COMPLETE",` +
		`"response":{"finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":7,"output_tokens":3}}}}` + "\n")
	return b.String()
}

func newCohereStreamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newCohereStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ctxkey.RequestModel, "command-r")
	return c, rec
}

// TestStreamHandler_RoutesThroughRewriter proves the bug-class fix: when the
// /v1/responses chat fallback installs a rewrite bridge, every token flows
// through HandleChunk and the terminal FinalizeUsage+HandleDone is invoked,
// instead of raw chat-completion chunks / "data: [DONE]" leaking to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newCohereStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := newCohereStreamResponse(cohereUpstreamStream([]string{"Hello", " world"}))

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
	require.NotContains(t, body, "Hello")
	require.NotContains(t, body, "world")
}

// TestStreamHandler_EmitsChatCompletionChunks asserts the no-bridge path is
// unchanged: without a rewriter the handler still emits OpenAI
// chat.completion.chunk SSE frames plus the [DONE] sentinel.
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newCohereStreamCtx(t)

	resp := newCohereStreamResponse(cohereUpstreamStream([]string{"Hello", " world"}))

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}
