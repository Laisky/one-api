package deepinfra

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertRerankRequest verifies native request shaping and response context retention.
func TestConvertRerankRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	topN := 2

	converted, err := (&Adaptor{}).ConvertRerankRequest(context, &model.RerankRequest{
		Model:     "Qwen/Qwen3-Reranker-8B",
		Query:     "best database for vectors",
		Documents: []string{"postgres", "redis", "sqlite"},
		TopN:      &topN,
	})
	require.NoError(t, err)
	require.Equal(t, &upstreamRerankRequest{
		Query:     "best database for vectors",
		Documents: []string{"postgres", "redis", "sqlite"},
	}, converted)
	require.Equal(t, 2, context.GetInt(rerankTopNContextKey))
}

// TestConvertRerankRequestRejectsUnsupportedFields verifies explicit validation.
func TestConvertRerankRequestRejectsUnsupportedFields(t *testing.T) {
	t.Parallel()

	maxTokens := 256
	_, err := (&Adaptor{}).ConvertRerankRequest(nil, &model.RerankRequest{
		Query:           "query",
		Documents:       []string{"document"},
		MaxTokensPerDoc: &maxTokens,
	})
	require.ErrorContains(t, err, "max_tokens_per_doc")
}

// TestHandleRerankResponse verifies sorting, top_n truncation, index preservation, and usage.
func TestHandleRerankResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(rerankDocumentsContextKey, []string{"first", "second", "third"})
	context.Set(rerankTopNContextKey, 2)

	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"scores":[0.1,0.9,0.5]}`)),
		Header:     make(http.Header),
	}
	relayError, usage := handleRerankResponse(context, response, "Qwen/Qwen3-Reranker-8B", 42)
	require.Nil(t, relayError)
	require.NotNil(t, usage)
	require.Equal(t, 42, usage.PromptTokens)

	var body canonicalRerankResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "list", body.Object)
	require.Len(t, body.Data, 2)
	require.Equal(t, 1, body.Data[0].Index)
	require.Equal(t, "second", body.Data[0].Document)
	require.Equal(t, 2, body.Data[1].Index)
}

// TestHandleRerankResponseRejectsScoreCardinalityMismatch verifies upstream validation.
func TestHandleRerankResponseRejectsScoreCardinalityMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(rerankDocumentsContextKey, []string{"first", "second"})
	context.Set(rerankTopNContextKey, 2)

	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"scores":[0.7]}`)),
		Header:     make(http.Header),
	}
	relayError, usage := handleRerankResponse(context, response, "Qwen/Qwen3-Reranker-8B", 10)
	require.Nil(t, usage)
	require.NotNil(t, relayError)
	require.Equal(t, http.StatusBadGateway, relayError.StatusCode)
}

// TestBuildDeepInfraError verifies FastAPI validation detail normalization.
func TestBuildDeepInfraError(t *testing.T) {
	t.Parallel()

	err := buildDeepInfraError([]byte(`{"detail":[{"msg":"field required","type":"missing"}]}`), http.StatusUnprocessableEntity)
	require.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	require.Equal(t, "field required", err.Message)
}
