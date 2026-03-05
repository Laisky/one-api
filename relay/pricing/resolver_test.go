package pricing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
)

// TestResolveImagePricing_ChannelConfigMissingImageFallback verifies image pricing falls back
// to provider defaults when the channel config omits image metadata.
func TestResolveImagePricing_ChannelConfigMissingImageFallback(t *testing.T) {
	modelName := "image-test-model"
	channelConfigs := map[string]model.ModelConfigLocal{
		modelName: {Ratio: 0.15},
	}

	provider := &MockAdaptor{
		name: "mock-provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Image: &adaptor.ImagePricingConfig{
					PricePerImageUsd: 0.039,
					DefaultSize:      "1024x1024",
					DefaultQuality:   "standard",
				},
			},
		},
	}

	cfg, ok := ResolveImagePricing(modelName, channelConfigs, provider)

	require.True(t, ok, "expected resolver to return image pricing")
	require.NotNil(t, cfg, "expected non-nil image pricing config")
	require.InDelta(t, 0.039, cfg.PricePerImageUsd, 0.0000001, "expected provider image price fallback")
	require.Equal(t, "1024x1024", cfg.DefaultSize, "expected provider default size")
	require.Equal(t, "standard", cfg.DefaultQuality, "expected provider default quality")
}

// TestResolveAudioPricing_ChannelConfigMissingAudioFallback verifies audio pricing falls back
// to provider defaults when the channel config omits audio metadata.
func TestResolveAudioPricing_ChannelConfigMissingAudioFallback(t *testing.T) {
	modelName := "audio-test-model"
	channelConfigs := map[string]model.ModelConfigLocal{
		modelName: {Ratio: 0.15},
	}

	provider := &MockAdaptor{
		name: "mock-provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Audio: &adaptor.AudioPricingConfig{
					PromptRatio:           16.0,
					CompletionRatio:       2.0,
					PromptTokensPerSecond: 10.0,
				},
			},
		},
	}

	cfg, ok := ResolveAudioPricing(modelName, channelConfigs, provider)

	require.True(t, ok, "expected resolver to return audio pricing")
	require.NotNil(t, cfg, "expected non-nil audio pricing config")
	require.InDelta(t, 16.0, cfg.PromptRatio, 0.0000001, "expected provider prompt ratio fallback")
	require.InDelta(t, 2.0, cfg.CompletionRatio, 0.0000001, "expected provider completion ratio fallback")
	require.InDelta(t, 10.0, cfg.PromptTokensPerSecond, 0.0000001, "expected provider prompt tokens per second fallback")
}

// TestGetVideoPricingWithThreeLayers_ChannelConfigMissingVideoFallback verifies video pricing
// falls back to provider defaults when the channel override lacks video metadata.
func TestGetVideoPricingWithThreeLayers_ChannelConfigMissingVideoFallback(t *testing.T) {
	modelName := "video-test-model"
	channelOverride := &adaptor.VideoPricingConfig{}

	provider := &MockAdaptor{
		name: "mock-provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Video: &adaptor.VideoPricingConfig{
					PerSecondUsd:   0.10,
					BaseResolution: "1280x720",
				},
			},
		},
	}

	cfg := GetVideoPricingWithThreeLayers(modelName, channelOverride, provider)

	require.NotNil(t, cfg, "expected provider video pricing fallback")
	require.InDelta(t, 0.10, cfg.PerSecondUsd, 0.0000001, "expected provider per-second pricing fallback")
	require.Equal(t, "1280x720", cfg.BaseResolution, "expected provider base resolution fallback")
}

func TestResolveModelConfig_ChannelOverridePreservesCacheAndTiers(t *testing.T) {
	const modelName = "claude-4.6-sonnet"
	var channelConfigs map[string]model.ModelConfigLocal
	err := json.Unmarshal([]byte(`{
		"claude-4.6-sonnet": {
			"ratio": 1.543,
			"completion_ratio": 7.715,
			"cached_input_ratio": 1.543,
			"cache_write_5m_ratio": 1.543,
			"cache_write_1h_ratio": 1.543,
			"tiers": [
				{
					"input_token_threshold": 200000,
					"ratio": 3.086,
					"completion_ratio": 11.571,
					"cached_input_ratio": 3.086,
					"cache_write_5m_ratio": 3.086,
					"cache_write_1h_ratio": 3.086
				}
			]
		}
	}`), &channelConfigs)
	require.NoError(t, err)

	cfg, ok := ResolveModelConfig(modelName, channelConfigs, nil)
	require.True(t, ok)
	require.InDelta(t, 1.543, cfg.CachedInputRatio, 0.0000001)
	require.InDelta(t, 1.543, cfg.CacheWrite5mRatio, 0.0000001)
	require.InDelta(t, 1.543, cfg.CacheWrite1hRatio, 0.0000001)
	require.Len(t, cfg.Tiers, 1)
	require.Equal(t, 200000, cfg.Tiers[0].InputTokenThreshold)
	require.InDelta(t, 3.086, cfg.Tiers[0].Ratio, 0.0000001)
	require.InDelta(t, 3.086, cfg.Tiers[0].CachedInputRatio, 0.0000001)
	require.InDelta(t, 3.086, cfg.Tiers[0].CacheWrite5mRatio, 0.0000001)
	require.InDelta(t, 3.086, cfg.Tiers[0].CacheWrite1hRatio, 0.0000001)
}
