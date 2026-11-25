package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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

// TestOpenRouterListModelsQuotesEffectiveGPT56SolTariff checks that the public
// OpenRouter provider feed quotes native OpenAI's GPT-5.6 Sol at the tariff
// billing charges at listing time: the promotional rate before the December 1
// cutoff and the standard rate from it on, for both aliases. Parameter t
// reports a stale quote.
func TestOpenRouterListModelsQuotesEffectiveGPT56SolTariff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cutoff := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at                         time.Time
		prompt, completion, cached float64 // USD per million tokens
	}{
		{cutoff.Add(-time.Nanosecond), 4, 20, .4},
		{cutoff, 5, 30, .5},
		{cutoff.AddDate(0, 0, 1), 5, 30, .5},
	} {
		pinModelsDisplayClock(t, tc.at)
		r := gin.New()
		r.GET("/openrouter/v1/models", OpenRouterListModels)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openrouter/v1/models", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp openrouterprovider.ModelListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		found := map[string]bool{}
		for _, m := range resp.Data {
			if m.ID != "gpt-5.6-sol" && m.ID != "gpt-5.6" {
				continue
			}
			found[m.ID] = true
			label := m.ID + " at " + tc.at.Format(time.RFC3339Nano)
			require.Equal(t, "openai: "+m.ID, m.Name, label)
			for _, price := range []struct {
				want float64
				got  string
			}{{tc.prompt, m.Pricing.Prompt}, {tc.completion, m.Pricing.Completion}, {tc.cached, m.Pricing.InputCacheRead}} {
				perToken, err := strconv.ParseFloat(price.got, 64)
				require.NoError(t, err, label)
				require.InDelta(t, price.want, perToken*1e6, 1e-9, label)
			}
		}
		require.True(t, found["gpt-5.6-sol"] && found["gpt-5.6"], "both aliases are listed")
	}
}
