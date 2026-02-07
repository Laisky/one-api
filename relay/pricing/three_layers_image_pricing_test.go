package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
)

// TestResolveImagePricing_AllLayers verifies channel/provider/global fallback behavior for image pricing.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestResolveImagePricing_AllLayers(t *testing.T) {
	const modelName = "image-three-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{
		modelName: {
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.011,
				DefaultSize:      "512x512",
				DefaultQuality:   "standard",
			},
		},
	})

	provider := &MockAdaptor{
		name: "provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Image: &adaptor.ImagePricingConfig{
					PricePerImageUsd: 0.022,
					DefaultSize:      "1024x1024",
					DefaultQuality:   "high",
				},
			},
		},
	}

	channelWithImage := map[string]model.ModelConfigLocal{
		modelName: {
			Image: &model.ImagePricingLocal{
				PricePerImageUsd: 0.033,
				DefaultSize:      "1536x1536",
				DefaultQuality:   "ultra",
			},
		},
	}
	cfg, ok := ResolveImagePricing(modelName, channelWithImage, provider)
	require.True(t, ok)
	require.InDelta(t, 0.033, cfg.PricePerImageUsd, 1e-12)
	require.Equal(t, "1536x1536", cfg.DefaultSize)

	channelWithoutImage := map[string]model.ModelConfigLocal{
		modelName: {Ratio: 0.15},
	}
	cfg, ok = ResolveImagePricing(modelName, channelWithoutImage, provider)
	require.True(t, ok)
	require.InDelta(t, 0.022, cfg.PricePerImageUsd, 1e-12)
	require.Equal(t, "1024x1024", cfg.DefaultSize)

	cfg, ok = ResolveImagePricing(modelName, channelWithoutImage, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.True(t, ok)
	require.InDelta(t, 0.011, cfg.PricePerImageUsd, 1e-12)
	require.Equal(t, "512x512", cfg.DefaultSize)

	cfg, ok = ResolveImagePricing("missing-image-model", nil, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.False(t, ok)
	require.Nil(t, cfg)
}
