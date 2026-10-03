package replicate

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestReplicateImageBillingBoundaryAcrossCatalog exercises every advertised
// image model through chat conversion, Claude conversion, and direct URL lookup.
// The latter prevents a caller bypassing conversion from selecting token billing
// for per-image work. Valid image routes and language models remain available.
func TestReplicateImageBillingBoundaryAcrossCatalog(t *testing.T) {
	imageModels := 0
	for _, name := range ModelList {
		pricing, exists := ModelRatios[name]
		require.True(t, exists, name)
		if pricing.Image == nil {
			t.Run(name+"/language-positive-control", func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				a := &Adaptor{}
				converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{Model: name, MaxTokens: 8})
				require.NoError(t, err)
				require.NotNil(t, converted)
				url, err := a.GetRequestURL(&meta.Meta{OriginModelName: name, Mode: relaymode.ChatCompletions})
				require.NoError(t, err)
				require.Contains(t, url, "/"+name+"/predictions")
			})
			continue
		}
		imageModels++
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			a := &Adaptor{}
			converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{Model: name, MaxTokens: 8})
			require.ErrorContains(t, err, "please use image API")
			require.Nil(t, converted)
			converted, err = a.ConvertClaudeRequest(c, &model.ClaudeRequest{Model: name, MaxTokens: 8})
			require.ErrorContains(t, err, "please use image API")
			require.Nil(t, converted)
			for _, mode := range []int{relaymode.ChatCompletions, relaymode.Completions, relaymode.Embeddings} {
				url, err := a.GetRequestURL(&meta.Meta{OriginModelName: name, Mode: mode})
				require.ErrorContains(t, err, "please use image API")
				require.Empty(t, url)
			}
			for _, mode := range []int{relaymode.ImagesGenerations, relaymode.ImagesEdits} {
				url, err := a.GetRequestURL(&meta.Meta{OriginModelName: name, Mode: mode})
				require.NoError(t, err)
				require.Contains(t, url, "/"+name+"/predictions")
			}
		})
	}
	require.Positive(t, imageModels, "the negative-control matrix must exercise real image models")
}
