package replicate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReplicateImageModelPrices ensures representative Replicate image models
// retain non-zero per-image pricing metadata.
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
// catalog, fixed output prices, advertised edit modality, and derived ModelList.
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
		{model: "xai/grok-imagine-image-quality", price: 0.07, supportsEdit: true},
		{model: "xai/grok-imagine-image-2", price: 0.04, supportsEdit: true},
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
			}
		})
	}

	seedream := ModelRatios["bytedance/seedream-5-pro"].Image
	require.Equal(t, "2K", seedream.DefaultSize)
	require.InDelta(t, 0.5, seedream.SizeMultipliers["1K"], 1e-12)
	require.InDelta(t, 1.0, seedream.SizeMultipliers["2K"], 1e-12)

	gptImage := ModelRatios["openai/gpt-image-1.5"].Image
	require.Equal(t, "auto", gptImage.DefaultQuality)
	require.Equal(t, 10, gptImage.MaxImages)
	require.InDelta(t, 0.013/0.136, gptImage.QualityMultipliers["low"], 1e-12)
	require.InDelta(t, 0.05/0.136, gptImage.QualityMultipliers["medium"], 1e-12)

	grokQuality := ModelRatios["xai/grok-imagine-image-quality"].Image
	require.Equal(t, "2k", grokQuality.DefaultSize)
	require.InDelta(t, 0.05/0.07, grokQuality.SizeMultipliers["1k"], 1e-12)
	require.InDelta(t, 1.0, grokQuality.SizeMultipliers["2k"], 1e-12)

	grok2 := ModelRatios["xai/grok-imagine-image-2"].Image
	require.Equal(t, "2k", grok2.DefaultSize)
	require.Equal(t, "medium", grok2.DefaultQuality)
}
