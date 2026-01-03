package openai_compatible

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestEmbeddingHandler_Success(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a valid embedding response
	embeddingResponse := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			},
			{
				"object":    "embedding",
				"index":     1,
				"embedding": []float64{0.6, 0.7, 0.8, 0.9, 1.0},
			},
		},
		"model": "text-embedding-ada-002",
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 0,
			"total_tokens":      10,
		},
	}

	responseBody, err := json.Marshal(embeddingResponse)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify no error
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Check usage
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
	assert.Equal(t, 10, usage.TotalTokens)

	// Check that response was forwarded to client
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Verify response body was written correctly
	var responseData map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &responseData)
	require.NoError(t, err)
	assert.Equal(t, "list", responseData["object"])
	assert.Equal(t, "text-embedding-ada-002", responseData["model"])

	// Verify embedding data
	data, ok := responseData["data"].([]any)
	require.True(t, ok)
	assert.Len(t, data, 2)

	firstEmbedding := data[0].(map[string]any)
	assert.Equal(t, "embedding", firstEmbedding["object"])
	assert.Equal(t, float64(0), firstEmbedding["index"])
}

func TestEmbeddingHandler_ErrorResponse(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create an error response
	errorResponse := map[string]any{
		"error": map[string]any{
			"message": "Invalid API key",
			"type":    "authentication_error",
			"code":    "invalid_api_key",
		},
	}

	responseBody, err := json.Marshal(errorResponse)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify error response
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	assert.Equal(t, http.StatusUnauthorized, errResp.StatusCode)
	assert.Equal(t, model.ErrorTypeAuthentication, errResp.Error.Type)
	assert.Equal(t, "Invalid API key", errResp.Error.Message)
}

func TestEmbeddingHandler_EmptyResponseBody(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(""))),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify error response
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	assert.Equal(t, "empty_response_body", errResp.Error.Code)
}

func TestEmbeddingHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte("invalid json"))),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify error response
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	assert.Equal(t, "unmarshal_embedding_response_failed", errResp.Error.Code)
}

func TestEmbeddingHandler_NoEmbeddingData(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create response with no embedding data
	embeddingResponse := map[string]any{
		"object": "list",
		"data":   []any{}, // Empty data array
		"model":  "text-embedding-ada-002",
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	responseBody, err := json.Marshal(embeddingResponse)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify error response
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	assert.Equal(t, "no_embedding_data", errResp.Error.Code)
}

func TestEmbeddingHandler_UsageTotalTokensCalculation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a response with usage that needs total tokens calculation
	embeddingResponse := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"object":    "embedding",
				"index":     0,
				"embedding": []float64{0.1, 0.2, 0.3},
			},
		},
		"model": "text-embedding-ada-002",
		"usage": map[string]any{
			"prompt_tokens":     15,
			"completion_tokens": 5,
			"total_tokens":      0, // Missing total tokens
		},
	}

	responseBody, err := json.Marshal(embeddingResponse)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify no error
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Check that total tokens was calculated
	assert.Equal(t, 15, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 20, usage.TotalTokens) // Should be calculated as 15 + 5
}

func TestEmbeddingHandler_ReadBodyError(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create a response with a body that will cause read error
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       &errorReader{}, // Custom reader that always returns error
	}

	// Call the handler
	errResp, usage := EmbeddingHandler(c, resp)

	// Verify error response
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	assert.Equal(t, "read_response_body_failed", errResp.Error.Code)
}

// errorReader is a test helper that always returns an error on Read
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (e *errorReader) Close() error {
	return nil
}
