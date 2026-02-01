package controller

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

const testImageDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// buildTestChannelWithMediaPricing creates a channel with a single model pricing config for tests.
// Parameters: t is the test handle; modelName is the pricing key; cfg is the model pricing config.
// Returns: a channel populated with the provided model pricing configuration.
func buildTestChannelWithMediaPricing(t *testing.T, modelName string, cfg model.ModelConfigLocal) *model.Channel {
	t.Helper()
	channel := &model.Channel{}
	configs := map[string]model.ModelConfigLocal{
		modelName: cfg,
	}
	require.NoError(t, channel.SetModelPriceConfigs(configs))
	return channel
}

// newTestGinContext creates a Gin context suitable for controller billing tests.
// Parameters: t is the test handle.
// Returns: a Gin context with a response recorder attached.
func newTestGinContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	return c
}

// TestApplyOutputAudioCharges_UsdPerSecond verifies audio output billing via per-second pricing.
func TestApplyOutputAudioCharges_UsdPerSecond(t *testing.T) {
	modelName := "media-test-model"
	channel := buildTestChannelWithMediaPricing(t, modelName, model.ModelConfigLocal{
		Audio: &model.AudioPricingLocal{
			UsdPerSecond: 0.02,
		},
	})

	c := newTestGinContext(t)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.OutputAudioSeconds, 2.5)
	c.Set(ctxkey.ChannelModel, channel)

	meta := &metalib.Meta{
		ActualModelName: modelName,
		ChannelType:     channeltype.OpenAI,
		APIType:         apitype.OpenAI,
		PromptTokens:    8,
	}
	usage := &relaymodel.Usage{}

	applyOutputAudioCharges(c, &usage, meta)

	expected := int64(math.Ceil(0.02 * 2.5 * ratio.QuotaPerUsd))
	require.Equal(t, expected, usage.ToolsCost)
}

// TestApplyOutputAudioCharges_TokensFallback verifies audio output billing via token fallback.
func TestApplyOutputAudioCharges_TokensFallback(t *testing.T) {
	modelName := "media-test-model"
	channel := buildTestChannelWithMediaPricing(t, modelName, model.ModelConfigLocal{
		Ratio: 2.0,
		Audio: &model.AudioPricingLocal{
			PromptRatio:     2.0,
			CompletionRatio: 3.0,
		},
	})

	c := newTestGinContext(t)
	c.Set(ctxkey.ChannelRatio, 1.2)
	c.Set(ctxkey.OutputAudioTokens, 100)
	c.Set(ctxkey.ChannelModel, channel)

	meta := &metalib.Meta{
		ActualModelName: modelName,
		ChannelType:     channeltype.OpenAI,
		APIType:         apitype.OpenAI,
	}
	usage := &relaymodel.Usage{}

	applyOutputAudioCharges(c, &usage, meta)

	expected := int64(math.Ceil(float64(100) * 2.0 * 3.0 * 2.0 * 1.2))
	require.Equal(t, expected, usage.ToolsCost)
}

// TestApplyOutputVideoCharges_ResolutionMultiplier verifies video output billing with resolution multipliers.
func TestApplyOutputVideoCharges_ResolutionMultiplier(t *testing.T) {
	modelName := "media-test-model"
	channel := buildTestChannelWithMediaPricing(t, modelName, model.ModelConfigLocal{
		Video: &model.VideoPricingLocal{
			PerSecondUsd:   0.1,
			BaseResolution: "1280x720",
			ResolutionMultipliers: map[string]float64{
				"1920x1080": 2.0,
			},
		},
	})

	c := newTestGinContext(t)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.OutputVideoSeconds, 4.0)
	c.Set(ctxkey.OutputVideoResolution, "1920x1080")
	c.Set(ctxkey.ChannelModel, channel)

	meta := &metalib.Meta{
		ActualModelName: modelName,
		ChannelType:     channeltype.OpenAI,
		APIType:         apitype.OpenAI,
	}
	usage := &relaymodel.Usage{}

	applyOutputVideoCharges(c, &usage, meta)

	expected := int64(math.Ceil(0.1 * 2.0 * 4.0 * ratio.QuotaPerUsd))
	require.Equal(t, expected, usage.ToolsCost)
}

// TestResponseAPIMixedMediaPromptAndOutputBilling verifies prompt tokens and mixed output billing.
func TestResponseAPIMixedMediaPromptAndOutputBilling(t *testing.T) {
	modelName := "media-test-model"
	req := &openai.ResponseAPIRequest{
		Model: modelName,
		Input: openai.ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "describe the image"},
					map[string]any{"type": "input_image", "image_url": testImageDataURL, "detail": "low"},
				},
			},
		},
	}

	ctx := context.Background()
	promptTokens := getResponseAPIPromptTokens(ctx, req)
	textTokens := openai.CountTokenText("describe the image", modelName)
	imageTokens, err := openai.CountImageTokens(testImageDataURL, "low", modelName)
	require.NoError(t, err)
	require.Greater(t, promptTokens, textTokens)
	require.GreaterOrEqual(t, promptTokens, textTokens+imageTokens)

	channel := buildTestChannelWithMediaPricing(t, modelName, model.ModelConfigLocal{
		Image: &model.ImagePricingLocal{
			PricePerImageUsd: 0.02,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
		},
		Audio: &model.AudioPricingLocal{
			UsdPerSecond: 0.005,
		},
		Video: &model.VideoPricingLocal{
			PerSecondUsd:   0.1,
			BaseResolution: "1280x720",
			ResolutionMultipliers: map[string]float64{
				"1920x1080": 2.0,
			},
		},
	})

	c := newTestGinContext(t)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.OutputImageCount, 2)
	c.Set(ctxkey.OutputAudioSeconds, 3.5)
	c.Set(ctxkey.OutputVideoSeconds, 4.0)
	c.Set(ctxkey.OutputVideoResolution, "1920x1080")
	c.Set(ctxkey.ChannelModel, channel)

	meta := &metalib.Meta{
		ActualModelName: modelName,
		ChannelType:     channeltype.OpenAI,
		APIType:         apitype.OpenAI,
		PromptTokens:    promptTokens,
	}
	usage := &relaymodel.Usage{PromptTokens: promptTokens}

	applyOutputImageCharges(c, &usage, meta)
	applyOutputAudioCharges(c, &usage, meta)
	applyOutputVideoCharges(c, &usage, meta)

	imageQuota := calculateImageBaseQuota(0.02, 0, 1.0, 1.0, 2)
	audioQuota := int64(math.Ceil(0.005 * 3.5 * ratio.QuotaPerUsd))
	videoQuota := int64(math.Ceil(0.1 * 2.0 * 4.0 * ratio.QuotaPerUsd))
	expected := imageQuota + audioQuota + videoQuota

	require.Equal(t, expected, usage.ToolsCost)
}
