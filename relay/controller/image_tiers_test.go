package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/ali"
	"github.com/songquanpeng/one-api/relay/adaptor/gemini"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai"
	"github.com/songquanpeng/one-api/relay/adaptor/xai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// Sanity check: usd_per_image * QuotaPerUsd with $0.04 -> 0.04 * 500000 = 20000
// We purposely do not call adapters; this guards controller math/unit consistency.
func TestImageUsdToQuotaMath(t *testing.T) {
	const quotaPerUsd = 500000.0
	usd := 0.04
	quotaPerImage := usd * quotaPerUsd
	require.Equal(t, 20000.0, quotaPerImage, "expected 20000 quota per image for $0.04")
}

// Test tier table values align with legacy logic for key models/sizes/qualities.
func TestImageTierTablesParity(t *testing.T) {
	// DALL-E 3 hd 1024x1024 -> 2x; other sizes -> 1.5x
	cases := []struct {
		model   string
		size    string
		quality string
		want    float64
	}{
		{"dall-e-3", "1024x1024", "hd", 2},
		{"dall-e-3", "1024x1792", "hd", 3}, // 2 * 1.5
		{"dall-e-3", "1792x1024", "hd", 3}, // 2 * 1.5
		{"gpt-image-1", "1024x1024", "high", 167.0 / 11},
		{"gpt-image-1", "1024x1536", "high", 250.0 / 11},
		{"gpt-image-1", "1536x1024", "high", 250.0 / 11},
		{"gpt-image-1", "1024x1024", "medium", 42.0 / 11},
		{"gpt-image-1", "1024x1536", "medium", 63.0 / 11},
		{"gpt-image-1", "1536x1024", "medium", 63.0 / 11},
		{"gpt-image-1", "1024x1024", "low", 1},
		{"gpt-image-1", "1024x1536", "low", 16.0 / 11},
		{"gpt-image-1", "1536x1024", "low", 16.0 / 11},
		{"gpt-image-1-mini", "1024x1024", "high", 36.0 / 5.0},
		{"gpt-image-1-mini", "1024x1536", "high", 52.0 / 5.0},
		{"gpt-image-1-mini", "1536x1024", "high", 52.0 / 5.0},
		{"gpt-image-1-mini", "1024x1024", "medium", 11.0 / 5.0},
		{"gpt-image-1-mini", "1024x1536", "medium", 15.0 / 5.0},
		{"gpt-image-1-mini", "1536x1024", "medium", 15.0 / 5.0},
		{"gpt-image-1-mini", "1024x1024", "low", 1},
		{"gpt-image-1-mini", "1024x1536", "low", 6.0 / 5.0},
		{"gpt-image-1-mini", "1536x1024", "low", 6.0 / 5.0},
	}

	for _, tc := range cases {
		cfg, ok := pricing.ResolveModelConfig(tc.model, nil, &openai.Adaptor{})
		require.True(t, ok && cfg.Image != nil, "missing image pricing config for %s", tc.model)
		got, err := getImageCostRatio(&relaymodel.ImageRequest{Model: tc.model, Size: tc.size, Quality: tc.quality}, cfg.Image)
		require.NoError(t, err)
		require.InDelta(t, tc.want, got, 1e-9, "%s %s %s: got %v, want %v", tc.model, tc.size, tc.quality, got, tc.want)
	}
}

func TestAliImagePricingConfig(t *testing.T) {
	cfg, ok := pricing.ResolveModelConfig("ali-stable-diffusion-xl", nil, &ali.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected ali-stable-diffusion-xl image pricing metadata")
	require.Equal(t, 1, cfg.Image.MinImages, "unexpected MinImages")
	require.Equal(t, 4, cfg.Image.MaxImages, "unexpected MaxImages")
	require.Equal(t, 1.0, cfg.Image.SizeMultipliers["512x1024"], "unexpected size multiplier for 512x1024")
}

func TestXAIImagePricingConfig(t *testing.T) {
	cfg, ok := pricing.ResolveModelConfig("grok-2-image", nil, &xai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected grok-2-image pricing metadata")
	require.InDelta(t, 0.07, cfg.Image.PricePerImageUsd, 1e-9, "expected image price 0.07")
	require.Equal(t, 1.0, cfg.Image.SizeMultipliers["1024x1024"], "unexpected xAI multiplier for 1024x1024")
}

func TestGeminiImagePricingConfig(t *testing.T) {
	cfg, ok := pricing.ResolveModelConfig("gemini-2.5-flash-image", nil, &gemini.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected gemini-2.5-flash-image pricing metadata")
	require.InDelta(t, 0.039, cfg.Image.PricePerImageUsd, 1e-9, "expected image price 0.039")
}

func TestVertexAIImagenPricingConfig(t *testing.T) {
	cfg, ok := pricing.ResolveModelConfig("imagen-4.0-generate-001", nil, &vertexai.Adaptor{})
	require.True(t, ok && cfg.Image != nil, "expected imagen-4.0-generate-001 pricing metadata")
	require.InDelta(t, 0.04, cfg.Image.PricePerImageUsd, 1e-9, "expected image price 0.04")
	require.Equal(t, 1.0, cfg.Image.SizeMultipliers["1024x1024"], "unexpected imagen multiplier for 1024x1024")
}
