package openai_compatible

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

type bridgeRecorder struct {
	deltas          []string
	doneCount       int
	usageSet        bool
	handleChunkDone bool
}

// HandleChunk records each bridge chunk and returns the configured doneRendered
// value for stream bridge helper tests.
func (r *bridgeRecorder) HandleChunk(_ *gin.Context, chunk *ChatCompletionsStreamResponse) (bool, bool) {
	for _, ch := range chunk.Choices {
		r.deltas = append(r.deltas, ch.Delta.StringContent())
	}
	return true, r.handleChunkDone
}

// HandleUpstreamDone records upstream completion for interface completeness.
func (r *bridgeRecorder) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	return true, false
}

// HandleDone records finalization for stream bridge helper tests.
func (r *bridgeRecorder) HandleDone(_ *gin.Context) (bool, bool) {
	r.doneCount++
	return true, true
}

// FinalizeUsage records that usage finalization was invoked.
func (r *bridgeRecorder) FinalizeUsage(_ *model.Usage) {
	r.usageSet = true
}

// JoinedDeltas returns all recorded text deltas concatenated in order.
func (r *bridgeRecorder) JoinedDeltas() string {
	return strings.Join(r.deltas, "")
}

func newBridgeTestCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func chunkWithContent(content string) *ChatCompletionsStreamResponse {
	return &ChatCompletionsStreamResponse{
		Id:     "chatcmpl-test",
		Object: "chat.completion.chunk",
		Model:  "test-model",
		Choices: []ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: model.Message{Role: "assistant", Content: content},
		}},
	}
}

// TestRenderStreamChunkWithBridge_NoRewriter verifies that without a bridge the
// helper behaves exactly like render.ObjectData + render.Done.
func TestRenderStreamChunkWithBridge_NoRewriter(t *testing.T) {
	c, rec := newBridgeTestCtx(t)

	require.NoError(t, RenderStreamChunkWithBridge(c, chunkWithContent("Hello")))
	FinalizeStreamWithBridge(c, &model.Usage{})

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, `"content":"Hello"`)
	require.Contains(t, body, "data: [DONE]")
}

// TestRenderStreamChunkWithBridge_WithRewriter verifies that when a bridge is
// present every chunk is routed through it and nothing raw is written, and that
// finalize drives FinalizeUsage + HandleDone instead of [DONE].
func TestRenderStreamChunkWithBridge_WithRewriter(t *testing.T) {
	c, rec := newBridgeTestCtx(t)
	rw := &bridgeRecorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	require.NoError(t, RenderStreamChunkWithBridge(c, chunkWithContent("foo")))
	require.NoError(t, RenderStreamChunkWithBridge(c, chunkWithContent("bar")))
	FinalizeStreamWithBridge(c, &model.Usage{CompletionTokens: 3})

	require.Equal(t, "foobar", rw.JoinedDeltas())
	require.True(t, rw.usageSet)
	require.Equal(t, 1, rw.doneCount)
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk")
	require.NotContains(t, body, "data: [DONE]")
}

// TestRenderStreamChunkWithBridge_ChunkDoneRendered verifies that a rewriter can
// report doneRendered from HandleChunk while the helper still suppresses raw
// chunk bytes.
func TestRenderStreamChunkWithBridge_ChunkDoneRendered(t *testing.T) {
	c, rec := newBridgeTestCtx(t)
	rw := &bridgeRecorder{handleChunkDone: true}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	require.NoError(t, RenderStreamChunkWithBridge(c, chunkWithContent("done")))

	require.Equal(t, "done", rw.JoinedDeltas())
	require.Empty(t, rec.Body.String())
}

// TestNormalizeStreamChunk_ForeignType verifies the marshal/unmarshal path used
// by adaptors whose chunk type is openai.ChatCompletionsStreamResponse (a
// distinct type) rather than the openai_compatible one.
func TestNormalizeStreamChunk_ForeignType(t *testing.T) {
	foreign := map[string]any{
		"object": "chat.completion.chunk",
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{"role": "assistant", "content": "baz"},
		}},
	}
	got, ok := normalizeStreamChunk(foreign)
	require.True(t, ok)
	require.Len(t, got.Choices, 1)
	require.Equal(t, "baz", got.Choices[0].Delta.StringContent())
}
