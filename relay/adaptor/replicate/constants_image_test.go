package replicate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Ensure key Replicate image models have non-zero per-image pricing
func TestReplicateImageModelPrices(t *testing.T) {
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
