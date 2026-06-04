package siliconflow

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDoResponse_Embeddings guards against the regression where SiliconFlow
// embedding responses (OpenAI-compatible /v1/embeddings schema) were routed
// through the chat-completion handler, which then failed with
// "no choices in response from upstream" because embedding payloads expose
// `data[*].embedding` instead of `choices`.
//
// The expected behavior: DoResponse must call openai_compatible.EmbeddingHandler
// for relaymode.Embeddings, forwarding the original payload to the client
// and returning the usage block.
func TestDoResponse_Embeddings(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	// Use a payload modeled after the failing request in the bug report:
	// a SiliconFlow Qwen/Qwen3-Embedding-8B response carrying a 4-dim
	// vector. The exact dimensionality is irrelevant; what matters is the
	// shape (data/usage) is correct.
	embeddingResponse := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.1, 0.2, 0.3, 0.4},
			},
		},
		"model": "Qwen/Qwen3-Embedding-8B",
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 0,
			"total_tokens":      10,
		},
	}
	body, err := json.Marshal(embeddingResponse)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	m := &meta.Meta{
		Mode:            relaymode.Embeddings,
		ActualModelName: "Qwen/Qwen3-Embedding-8B",
	}

	usage, errResp := (&Adaptor{}).DoResponse(c, resp, m)

	// Must not surface the "no choices" error that the chat-completion
	// handler produced for embedding payloads.
	require.Nil(t, errResp)
	require.NotNil(t, usage, "usage must be populated for billing")

	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, 10, usage.TotalTokens)

	// The original OpenAI-compatible body must be forwarded verbatim so
	// embedding SDKs can keep parsing it.
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.JSONEq(t, string(body), w.Body.String())
}
