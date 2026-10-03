package replicate

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// reviewExpectedReplicateImageModels is an independent contract, not generated
// from ModelList, ModelRatios, or Image flags at test runtime. Intentional model
// retirement must update this roster explicitly; additive models are still
// exercised by the full-catalog matrix below.
var reviewExpectedReplicateImageModels = []string{
	"black-forest-labs/flux-kontext-pro",
	"black-forest-labs/flux-1.1-pro",
	"black-forest-labs/flux-2-dev",
	"black-forest-labs/flux-2-max",
	"black-forest-labs/flux-2-pro",
	"black-forest-labs/flux-2-flex",
	"black-forest-labs/flux-2-klein-4b",
	"black-forest-labs/flux-1.1-pro-ultra",
	"black-forest-labs/flux-canny-dev",
	"black-forest-labs/flux-canny-pro",
	"black-forest-labs/flux-depth-dev",
	"black-forest-labs/flux-depth-pro",
	"black-forest-labs/flux-dev",
	"black-forest-labs/flux-dev-lora",
	"black-forest-labs/flux-fill-dev",
	"black-forest-labs/flux-fill-pro",
	"black-forest-labs/flux-pro",
	"black-forest-labs/flux-redux-dev",
	"black-forest-labs/flux-redux-schnell",
	"black-forest-labs/flux-schnell",
	"black-forest-labs/flux-schnell-lora",
	"bytedance/dreamina-3.1",
	"bytedance/seedream-3",
	"bytedance/seedream-4",
	"bytedance/seedream-4.5",
	"bytedance/seedream-5-lite",
	"openai/gpt-image-2",
	"google/imagen-4",
	"google/imagen-4-ultra",
	"google/imagen-4-fast",
	"google/imagen-3",
	"google/imagen-3-fast",
	"ideogram-ai/ideogram-v2",
	"ideogram-ai/ideogram-v2-turbo",
	"ideogram-ai/ideogram-v3-turbo",
	"ideogram-ai/ideogram-v3-balanced",
	"ideogram-ai/ideogram-v3-quality",
	"recraft-ai/recraft-v3",
	"recraft-ai/recraft-v3-svg",
	"stability-ai/stable-diffusion-3",
	"stability-ai/stable-diffusion-3.5-large",
	"stability-ai/stable-diffusion-3.5-large-turbo",
	"stability-ai/stable-diffusion-3.5-medium",
	"black-forest-labs/flux-kontext-max",
	"bytedance/seedream-5-pro",
	"google/nano-banana",
	"google/nano-banana-2-lite",
	"openai/gpt-image-1.5",
	"recraft-ai/recraft-v4",
	"recraft-ai/recraft-v4-pro",
	"recraft-ai/recraft-v4.1",
	"recraft-ai/recraft-v4.1-pro",
	"recraft-ai/recraft-v4.1-utility",
	"recraft-ai/recraft-v4.1-utility-pro",
	"wan-video/wan-2.7-image",
	"wan-video/wan-2.7-image-pro",
	"xai/grok-imagine-image",
	"xai/grok-imagine-image-quality",
	"xai/grok-imagine-image-2",
}

// reviewValidateReplicateImageRoster catches removals and lost classification
// before the routing test can silently skip or misclassify an expected image.
func reviewValidateReplicateImageRoster(models []string, prices map[string]adaptor.ModelConfig) error {
	advertised := make(map[string]bool, len(models))
	for _, name := range models {
		advertised[name] = true
	}
	for _, name := range reviewExpectedReplicateImageModels {
		if !advertised[name] {
			return fmt.Errorf("expected image model %q is not advertised", name)
		}
		cfg, exists := prices[name]
		if !exists {
			return fmt.Errorf("expected image model %q has no pricing", name)
		}
		if cfg.Image == nil {
			return fmt.Errorf("expected image model %q has no image pricing", name)
		}
	}
	return nil
}

// TestReplicateImageRosterRejectsCatalogMutants verifies the independent oracle
// rejects the faults the original data-derived matrix could overlook. Copies
// isolate every mutation; production globals and nested image configs are never
// changed. The healthy catalog is a positive control, not a vacuous rejection.
func TestReplicateImageRosterRejectsCatalogMutants(t *testing.T) {
	require.NoError(t, reviewValidateReplicateImageRoster(ModelList, ModelRatios))
	for _, name := range reviewExpectedReplicateImageModels {
		for _, fault := range []string{"removed-from-catalog", "removed-price", "removed-image-flag"} {
			t.Run(name+"/"+fault, func(t *testing.T) {
				models := slices.Clone(ModelList)
				prices := maps.Clone(ModelRatios)
				var reason string
				switch fault {
				case "removed-from-catalog":
					models = slices.DeleteFunc(models, func(value string) bool { return value == name })
					reason = "is not advertised"
				case "removed-price":
					delete(prices, name)
					reason = "has no pricing"
				case "removed-image-flag":
					cfg := prices[name]
					cfg.Image = nil
					prices[name] = cfg
					reason = "has no image pricing"
				}
				err := reviewValidateReplicateImageRoster(models, prices)
				require.ErrorContains(t, err, name)
				require.ErrorContains(t, err, reason)
			})
		}
	}
	require.NoError(t, reviewValidateReplicateImageRoster(ModelList, ModelRatios), "mutation controls must not alter the real catalog")
}

// TestReplicateImageBillingBoundaryAcrossCatalog exercises every advertised
// image model through chat conversion, Claude conversion, and direct URL lookup.
// The latter prevents a caller bypassing conversion from selecting token billing
// for per-image work. Valid image routes and language models remain available.
func TestReplicateImageBillingBoundaryAcrossCatalog(t *testing.T) {
	require.NoError(t, reviewValidateReplicateImageRoster(ModelList, ModelRatios))
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
