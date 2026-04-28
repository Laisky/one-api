package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

func TestEmbeddingHandlerUsageFromResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)

	body := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"model":"text-embedding-3-small","usage":{"prompt_tokens":10,"total_tokens":10}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
	resp.Header.Set("Content-Type", "application/json")

	errResp, usage := EmbeddingHandler(c, resp, 4, "text-embedding-3-small")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 10, usage.TotalTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, body, w.Body.String())
}

func TestEmbeddingHandlerUsageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)

	body := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"model":"text-embedding-3-small"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	errResp, usage := EmbeddingHandler(c, resp, 7, "text-embedding-3-small")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Zero(t, usage.CompletionTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, body, w.Body.String())
}

func TestEmbeddingHandlerDecodesBase64Embedding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)

	base64Vector := encodeEmbeddingToBase64([]float32{0.25, -0.5, 1.5})
	body := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":"` + base64Vector + `"}],"model":"text-embedding-3-small","usage":{"prompt_tokens":3,"total_tokens":3}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
	resp.Header.Set("Content-Type", "application/json")

	errResp, usage := EmbeddingHandler(c, resp, 4, "text-embedding-3-small")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.PromptTokens)
	converted, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists)
	respPayload, ok := converted.(EmbeddingResponse)
	require.True(t, ok)
	require.Len(t, respPayload.Data, 1)
	require.True(t, respPayload.Data[0].Base64Encoded)
	require.Len(t, respPayload.Data[0].Embedding, 3)
	require.InDeltaSlice(t, []float64{0.25, -0.5, 1.5}, respPayload.Data[0].Embedding, 1e-6)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, body, w.Body.String())
}

func encodeEmbeddingToBase64(values []float32) string {
	if len(values) == 0 {
		return ""
	}
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		recorded := math.Float32bits(v)
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], recorded)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
