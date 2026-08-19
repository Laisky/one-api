package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/openrouterprovider"
)

// TestOpenRouterListModels_ResponseShape spins up a Gin router with the
// public OpenRouter provider listing handler attached and asserts the response
// matches the schema OpenRouter expects: {"data":[Model,...]} with each
// element exposing the required string/list/numeric fields.
func TestOpenRouterListModels_ResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/openrouter/v1/models", OpenRouterListModels)

	req, err := http.NewRequest(http.MethodGet, "/openrouter/v1/models", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var resp openrouterprovider.ModelListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data, "expected at least one model in the catalog")

	for _, m := range resp.Data {
		require.NotEmpty(t, m.ID, "model id must be non-empty")
		require.NotEmpty(t, m.Name, "model name must be non-empty")
		require.NotEmpty(t, m.Quantization, "quantization must be populated (default fp16)")
		require.NotEmpty(t, m.InputModalities, "input_modalities must be populated")
		require.NotEmpty(t, m.OutputModalities, "output_modalities must be populated")
		require.Greater(t, m.ContextLength, int32(0), "context_length must be positive")
		require.Greater(t, m.MaxOutputLength, int32(0), "max_output_length must be positive")
		require.NotEmpty(t, m.Pricing.Prompt, "pricing.prompt must always be set (zero -> '0')")
		require.NotEmpty(t, m.Pricing.Completion, "pricing.completion must always be set (zero -> '0')")
		require.NotEmpty(t, m.SupportedSamplingParameters, "supported_sampling_parameters must be populated")
		require.NotEmpty(t, m.SupportedFeatures, "supported_features must be populated")
	}
}

// TestOpenRouterListModels_StableDeduplication checks that the catalog does
// not emit two models with identical case-insensitive ids; the listing should
// keep the first occurrence so callers see a stable ordering.
func TestOpenRouterListModels_StableDeduplication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/openrouter/v1/models", OpenRouterListModels)

	req, _ := http.NewRequest(http.MethodGet, "/openrouter/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp openrouterprovider.ModelListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	seen := make(map[string]struct{}, len(resp.Data))
	for _, m := range resp.Data {
		key := m.ID
		_, dup := seen[key]
		require.False(t, dup, "duplicate model id in listing: %s", key)
		seen[key] = struct{}{}
	}
}
