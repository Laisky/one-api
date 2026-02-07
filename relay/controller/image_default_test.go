package controller

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/gemini"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// Test that DALL-E 3 defaults quality to "standard" (not "auto").
func TestGetImageRequest_DefaultQuality_DALLE3(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{
        "model": "dall-e-3",
        "prompt": "test prompt",
        "size": "1024x1024"
    }`)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ir, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	cfg, ok := pricing.ResolveModelConfig("dall-e-3", nil, &openai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected pricing config for dall-e-3")
	applyImageDefaults(ir, cfg.Image)
	require.Equal(t, "standard", ir.Quality, "expected default quality 'standard' for dall-e-3")
}

func TestGetImageRequest_DefaultQuality_GPTImage1(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{
        "model": "gpt-image-1",
        "prompt": "test prompt",
        "size": "1024x1024"
    }`)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ir, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	cfg, ok := pricing.ResolveModelConfig("gpt-image-1", nil, &openai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected pricing config for gpt-image-1")
	applyImageDefaults(ir, cfg.Image)
	require.Equal(t, "high", ir.Quality, "expected default quality 'high' for gpt-image-1")
}

func TestGetImageRequest_DefaultQuality_GPTImage1Mini(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{
        "model": "gpt-image-1-mini",
        "prompt": "test prompt",
        "size": "1024x1024"
    }`)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ir, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	cfg, ok := pricing.ResolveModelConfig("gpt-image-1-mini", nil, &openai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected pricing config for gpt-image-1-mini")
	applyImageDefaults(ir, cfg.Image)
	require.Equal(t, "high", ir.Quality, "expected default quality 'high' for gpt-image-1-mini")
}

func TestGetImageRequest_DefaultQuality_DALLE2(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{
        "model": "dall-e-2",
        "prompt": "test prompt",
        "size": "1024x1024"
    }`)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ir, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	cfg, ok := pricing.ResolveModelConfig("dall-e-2", nil, &openai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected pricing config for dall-e-2")
	applyImageDefaults(ir, cfg.Image)
	require.Equal(t, "standard", ir.Quality, "expected default quality 'standard' for dall-e-2")
}

// TestResolveImagePricing_ChannelModelWithoutImage verifies image pricing resolution
// falls back to provider defaults when channel model config exists but omits image metadata.
func TestResolveImagePricing_ChannelModelWithoutImage(t *testing.T) {
	t.Parallel()

	const imageModel = "gemini-2.5-flash-image"
	channelConfigs := map[string]model.ModelConfigLocal{
		imageModel: {
			Ratio: 0.15,
		},
	}

	imagePricingCfg, ok := pricing.ResolveImagePricing(imageModel, channelConfigs, &gemini.Adaptor{})
	require.True(t, ok, "expected image pricing resolution to succeed")

	require.NotNil(t, imagePricingCfg, "expected resolved image pricing config")
	require.InDelta(t, 0.039, imagePricingCfg.PricePerImageUsd, 1e-9, "expected provider default per-image pricing")
	require.Equal(t, "1024x1024", imagePricingCfg.DefaultSize, "expected provider default size")
	require.Equal(t, "standard", imagePricingCfg.DefaultQuality, "expected provider default quality")
}
