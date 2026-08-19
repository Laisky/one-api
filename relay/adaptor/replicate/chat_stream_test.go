package replicate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/internal/streambridgetest"
	"github.com/Laisky/one-api/relay/meta"
)

// replicateStreamServer returns a mock Replicate stream endpoint that emits the
// given output tokens as `event: output` SSE frames followed by a terminal
// `event: done` frame, mirroring Replicate's native stream wire format.
func replicateStreamServer(t *testing.T, tokens []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range tokens {
			fmt.Fprintf(w, "event: output\ndata: %s\n\n", tok)
		}
		fmt.Fprint(w, "event: done\ndata: {}\n\n")
	}))
}

func newReplicateStreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	meta.Set2Context(c, &meta.Meta{
		ActualModelName: "openai/gpt-oss-120b",
		PromptTokens:    10,
		APIKey:          "test-key",
	})
	return c, rec
}

// TestChatStreamHandler_EmitsChatCompletionChunks verifies that without a
// Response API rewriter the handler emits well-formed OpenAI
// chat.completion.chunk SSE frames (not raw text) plus a [DONE] sentinel.
func TestChatStreamHandler_EmitsChatCompletionChunks(t *testing.T) {
	srv := replicateStreamServer(t, []string{"Hello", " world"})
	defer srv.Close()

	c, rec := newReplicateStreamCtx(t)

	text, err := chatStreamHandler(c, srv.URL)
	require.NoError(t, err)
	require.Equal(t, "Hello world", text)

	body := rec.Body.String()
	// The fix: payloads must be JSON chat-completion chunks, never raw tokens.
	require.NotContains(t, body, "data: Hello")
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]")
}

// TestChatStreamHandler_RoutesThroughRewriter verifies that when the Response
// API fallback installs a rewriter, every token flows through HandleChunk and
// the terminal sequence (FinalizeUsage + HandleDone) is invoked, rather than
// raw bytes being flushed to the client.
func TestChatStreamHandler_RoutesThroughRewriter(t *testing.T) {
	srv := replicateStreamServer(t, []string{"foo", "bar", "baz"})
	defer srv.Close()

	c, rec := newReplicateStreamCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	text, err := chatStreamHandler(c, srv.URL)
	require.NoError(t, err)
	require.Equal(t, "foobarbaz", text)

	require.Equal(t, "foobarbaz", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)
	// The handler must not write raw tokens directly when a rewriter is present.
	require.NotContains(t, rec.Body.String(), "data: foo")
}
