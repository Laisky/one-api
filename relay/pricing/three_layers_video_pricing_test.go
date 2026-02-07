package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor"
)

// TestGetVideoPricingWithThreeLayers_AllLayers verifies channel/provider/global fallback behavior for video pricing.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestGetVideoPricingWithThreeLayers_AllLayers(t *testing.T) {
	const modelName = "video-three-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{
		modelName: {
			Video: &adaptor.VideoPricingConfig{
				PerSecondUsd:   0.01,
				BaseResolution: "640x360",
			},
		},
	})

	provider := &MockAdaptor{
		name: "provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Video: &adaptor.VideoPricingConfig{
					PerSecondUsd:   0.02,
					BaseResolution: "1280x720",
				},
			},
		},
	}

	channelOverride := &adaptor.VideoPricingConfig{
		PerSecondUsd:   0.03,
		BaseResolution: "1920x1080",
	}
	cfg := GetVideoPricingWithThreeLayers(modelName, channelOverride, provider)
	require.NotNil(t, cfg)
	require.InDelta(t, 0.03, cfg.PerSecondUsd, 1e-12)
	require.Equal(t, "1920x1080", cfg.BaseResolution)

	cfg = GetVideoPricingWithThreeLayers(modelName, &adaptor.VideoPricingConfig{}, provider)
	require.NotNil(t, cfg)
	require.InDelta(t, 0.02, cfg.PerSecondUsd, 1e-12)
	require.Equal(t, "1280x720", cfg.BaseResolution)

	cfg = GetVideoPricingWithThreeLayers(modelName, &adaptor.VideoPricingConfig{}, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.NotNil(t, cfg)
	require.InDelta(t, 0.01, cfg.PerSecondUsd, 1e-12)
	require.Equal(t, "640x360", cfg.BaseResolution)

	cfg = GetVideoPricingWithThreeLayers("missing-video-model", nil, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.Nil(t, cfg)
}
