package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
)

// TestResolveAudioPricing_AllLayers verifies channel/provider/global fallback behavior for audio pricing.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestResolveAudioPricing_AllLayers(t *testing.T) {
	const modelName = "audio-three-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{
		modelName: {
			Audio: &adaptor.AudioPricingConfig{
				PromptRatio:           10.0,
				CompletionRatio:       1.5,
				PromptTokensPerSecond: 30.0,
			},
		},
	})

	provider := &MockAdaptor{
		name: "provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {
				Audio: &adaptor.AudioPricingConfig{
					PromptRatio:           20.0,
					CompletionRatio:       2.5,
					PromptTokensPerSecond: 40.0,
				},
			},
		},
	}

	channelWithAudio := map[string]model.ModelConfigLocal{
		modelName: {
			Audio: &model.AudioPricingLocal{
				PromptRatio:           30.0,
				CompletionRatio:       3.5,
				PromptTokensPerSecond: 50.0,
			},
		},
	}
	cfg, ok := ResolveAudioPricing(modelName, channelWithAudio, provider)
	require.True(t, ok)
	require.InDelta(t, 30.0, cfg.PromptRatio, 1e-12)
	require.InDelta(t, 3.5, cfg.CompletionRatio, 1e-12)

	channelWithoutAudio := map[string]model.ModelConfigLocal{
		modelName: {Ratio: 0.15},
	}
	cfg, ok = ResolveAudioPricing(modelName, channelWithoutAudio, provider)
	require.True(t, ok)
	require.InDelta(t, 20.0, cfg.PromptRatio, 1e-12)
	require.InDelta(t, 2.5, cfg.CompletionRatio, 1e-12)

	cfg, ok = ResolveAudioPricing(modelName, channelWithoutAudio, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.True(t, ok)
	require.InDelta(t, 10.0, cfg.PromptRatio, 1e-12)
	require.InDelta(t, 1.5, cfg.CompletionRatio, 1e-12)

	cfg, ok = ResolveAudioPricing("missing-audio-model", nil, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}})
	require.False(t, ok)
	require.Nil(t, cfg)
}
