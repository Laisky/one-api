package aiproxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/internal/streambridgetest"
	"github.com/stretchr/testify/require"
)

// cannedUpstream builds an *http.Response whose body streams the aiproxy library
// upstream SSE wire format: one `data:{json}` line per LibraryStreamResponse,
// matching what streamResponseAIProxyLibrary2OpenAI parses.
func cannedUpstream(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

type failingReadCloser struct {
	data []byte
	read bool
}

// Read returns the canned bytes once, then fails with a synthetic read error.
func (r *failingReadCloser) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, r.data), nil
	}
	return 0, errors.New("synthetic stream read failure")
}

// Close closes the failing reader without additional errors.
func (r *failingReadCloser) Close() error {
	return nil
}

// TestStreamHandler_RoutesThroughRewriter verifies that when the Response API
// fallback installs a rewriter, every content token flows through HandleChunk
// and the terminal sequence (FinalizeUsage + HandleDone) is invoked, rather than
// raw chat-completion chunks / [DONE] being flushed to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	c, rec := newStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	upstream := "data:{\"content\":\"Hello\",\"model\":\"m\"}\n" +
		"data:{\"content\":\" world\",\"model\":\"m\"}\n"
	resp := cannedUpstream(upstream)

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// The two content tokens (plus a trailing empty documents chunk) must reach
	// the rewriter rather than being written verbatim to the client.
	got := rw.JoinedDeltas()
	require.Contains(t, got, "Hello")
	require.Contains(t, got, " world")
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	// Nothing chat-completion-shaped must leak into the recorder body when a
	// rewriter intercepts the stream.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
	require.NotContains(t, body, "Hello")
	require.NotContains(t, body, " world")
}

// TestStreamHandler_RoutesDocumentsThroughRewriter verifies that AIProxy's
// provider-specific trailing documents chunk is bridged as normal text instead
// of leaking raw chat-completion SSE to a Responses API client.
func TestStreamHandler_RoutesDocumentsThroughRewriter(t *testing.T) {
	c, rec := newStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	upstream := "data:{\"content\":\"Answer\",\"model\":\"m\",\"documents\":[{\"title\":\"Doc One\",\"url\":\"https://example.test/doc\"}]}\n"
	resp := cannedUpstream(upstream)

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	got := rw.JoinedDeltas()
	require.Contains(t, got, "Answer")
	require.Contains(t, got, "Reference Documents:")
	require.Contains(t, got, "[Doc One](https://example.test/doc)")
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)

	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "Reference Documents:")
}

// TestStreamHandler_ReadErrorDoesNotFinalizeAsCompleted verifies that a
// non-EOF upstream read failure returns an error and does not emit the synthetic
// documents/final completion sequence.
func TestStreamHandler_ReadErrorDoesNotFinalizeAsCompleted(t *testing.T) {
	c, rec := newStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := cannedUpstream("")
	resp.Body = &failingReadCloser{data: []byte("data:{\"content\":\"partial\",\"model\":\"m\"}\n")}

	errResp, usage := StreamHandler(c, resp)
	require.NotNil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, "read_stream_failed", errResp.Code)
	require.Equal(t, "partial", rw.JoinedDeltas())
	require.Equal(t, 0, rw.DoneCount)
	require.False(t, rw.UsageSet)
	require.NotContains(t, rec.Body.String(), "data: [DONE]")
}

// TestStreamHandler_ChunkDoneRenderedStillUsesBridge covers a rewriter that
// marks doneRendered from HandleChunk. The helper must keep routing chunks
// through the bridge and avoid raw chat-completion bytes.
func TestStreamHandler_ChunkDoneRenderedStillUsesBridge(t *testing.T) {
	c, rec := newStreamCtx(t)
	rw := &streambridgetest.Recorder{HandleChunkDone: true}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := cannedUpstream("data:{\"content\":\"Hello\",\"model\":\"m\"}\n")

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Contains(t, rw.JoinedDeltas(), "Hello")
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)
	require.NotContains(t, rec.Body.String(), "chat.completion.chunk")
}

// TestStreamHandler_EmitsChatCompletionChunks verifies that without a Response
// API rewriter the handler still emits well-formed OpenAI chat.completion.chunk
// SSE frames plus a [DONE] sentinel (the plain Chat Completions stream path).
func TestStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	c, rec := newStreamCtx(t)

	upstream := "data:{\"content\":\"Hello\",\"model\":\"m\"}\n" +
		"data:{\"content\":\" world\",\"model\":\"m\"}\n"
	resp := cannedUpstream(upstream)

	errResp, _ := StreamHandler(c, resp)
	require.Nil(t, errResp)

	body := rec.Body.String()
	require.Contains(t, body, "\"object\":\"chat.completion.chunk\"")
	require.Contains(t, body, "\"content\":\"Hello\"")
	require.Contains(t, body, "\"content\":\" world\"")
	require.Contains(t, body, "data: [DONE]")
}
