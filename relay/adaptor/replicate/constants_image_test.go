package replicate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReplicateImageModelPrices ensures representative Replicate image models
// retain non-zero per-image pricing metadata. The t parameter manages the test
// lifecycle and assertions. This function returns no values.
func TestReplicateImageModelPrices(t *testing.T) {
	t.Parallel()
	cases := []string{
		"black-forest-labs/flux-schnell",
		"black-forest-labs/flux-pro",
		"stability-ai/stable-diffusion-3",
	}
	for _, model := range cases {
		cfg, ok := ModelRatios[model]
		require.True(t, ok, "model %s not found in ModelRatios", model)
		require.NotNil(t, cfg.Image, "expected Image config for %s", model)
		require.Greater(t, cfg.Image.PricePerImageUsd, float64(0), "expected price_per_image_usd > 0 for %s", model)
	}
}

// TestReplicateOfficialImageAdditions verifies the refreshed official model
// catalog, safe output prices, advertised modalities, and derived ModelList. The
// t parameter manages the test lifecycle and assertions. This function returns
// no values.
func TestReplicateOfficialImageAdditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model        string
		price        float64
		supportsEdit bool
	}{
		{model: "black-forest-labs/flux-kontext-max", price: 0.08, supportsEdit: true},
		{model: "bytedance/seedream-5-pro", price: 0.09, supportsEdit: true},
		{model: "google/nano-banana", price: 0.039, supportsEdit: true},
		{model: "google/nano-banana-2-lite", price: 0.034, supportsEdit: true},
		{model: "openai/gpt-image-1.5", price: 0.136, supportsEdit: true},
		{model: "recraft-ai/recraft-v4", price: 0.04},
		{model: "recraft-ai/recraft-v4-pro", price: 0.25},
		{model: "recraft-ai/recraft-v4.1", price: 0.04},
		{model: "recraft-ai/recraft-v4.1-pro", price: 0.25},
		{model: "recraft-ai/recraft-v4.1-utility", price: 0.04},
		{model: "recraft-ai/recraft-v4.1-utility-pro", price: 0.25},
		{model: "wan-video/wan-2.7-image", price: 0.03, supportsEdit: true},
		{model: "wan-video/wan-2.7-image-pro", price: 0.03, supportsEdit: true},
		{model: "xai/grok-imagine-image", price: 0.02, supportsEdit: true},
		{model: "xai/grok-imagine-image-quality", price: 0.07},
		{model: "xai/grok-imagine-image-2", price: 0.04},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()

			cfg, ok := ModelRatios[tt.model]
			require.True(t, ok, "model %s not found in ModelRatios", tt.model)
			require.Contains(t, ModelList, tt.model)
			require.NotNil(t, cfg.Image)
			require.InDelta(t, tt.price, cfg.Image.PricePerImageUsd, 1e-12)
			require.Equal(t, []string{"image"}, cfg.OutputModalities)
			if tt.supportsEdit {
				require.Contains(t, cfg.InputModalities, "image")
			} else {
				require.NotContains(t, cfg.InputModalities, "image")
			}
		})
	}

	seedream := ModelRatios["bytedance/seedream-5-pro"].Image
	require.Equal(t, "2K", seedream.DefaultSize)
	require.NotContains(t, seedream.SizeMultipliers, "1k")
	require.InDelta(t, 1.0, seedream.SizeMultipliers["2k"], 1e-12)

	nanoBananaLite := ModelRatios["google/nano-banana-2-lite"].Image
	require.Equal(t, "1K", nanoBananaLite.DefaultSize)
	require.InDelta(t, 1.0, nanoBananaLite.SizeMultipliers["1k"], 1e-12)

	gptImage := ModelRatios["openai/gpt-image-1.5"].Image
	require.Equal(t, "auto", gptImage.DefaultQuality)
	require.NotContains(t, gptImage.QualityMultipliers, "low")
	require.NotContains(t, gptImage.QualityMultipliers, "medium")
	require.InDelta(t, 1.0, gptImage.QualityMultipliers["auto"], 1e-12)
	require.InDelta(t, 1.0, gptImage.QualityMultipliers["high"], 1e-12)

	grokQuality := ModelRatios["xai/grok-imagine-image-quality"].Image
	require.Equal(t, "2k", grokQuality.DefaultSize)
	require.NotContains(t, grokQuality.SizeMultipliers, "1k")
	require.InDelta(t, 1.0, grokQuality.SizeMultipliers["2k"], 1e-12)

	grok2 := ModelRatios["xai/grok-imagine-image-2"].Image
	require.Equal(t, "2k", grok2.DefaultSize)
	require.Equal(t, "medium", grok2.DefaultQuality)
	require.InDelta(t, 1.0, grok2.SizeMultipliers["2k"], 1e-12)
	require.InDelta(t, 1.0, grok2.QualityMultipliers["medium"], 1e-12)
}
