package cloudflare

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestModelRatiosIncludeLatestWorkersAITextModels verifies the pricing and
// capability metadata published for current Cloudflare Workers AI text models.
func TestModelRatiosIncludeLatestWorkersAITextModels(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		ratio            float64
		cachedInputRatio float64
		completionRatio  float64
		contextLength    int32
		inputModalities  []string
		features         []string
	}{
		"@cf/zai-org/glm-4.7-flash": {
			ratio:           0.060 * ratio.MilliTokensUsd,
			completionRatio: 0.400 / 0.060,
			contextLength:   131_072,
		},
		"@cf/nvidia/nemotron-3-120b-a12b": {
			ratio:           0.500 * ratio.MilliTokensUsd,
			completionRatio: 1.500 / 0.500,
			contextLength:   256_000,
		},
		"@cf/moonshotai/kimi-k2.6": {
			ratio:            0.950 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.160 * ratio.MilliTokensUsd,
			completionRatio:  4.000 / 0.950,
			contextLength:    262_144,
		},
		"@cf/deepseek-ai/deepseek-v4-flash-0731": {
			ratio:            0.440 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.014 * ratio.MilliTokensUsd,
			completionRatio:  1.320 / 0.440,
			contextLength:    1_048_576,
			inputModalities:  []string{"text"},
			features:         []string{"tools", "reasoning"},
		},
		"@cf/deepseek-ai/deepseek-v4-pro-0813": {
			ratio:            1.320 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.044 * ratio.MilliTokensUsd,
			completionRatio:  3.960 / 1.320,
			contextLength:    1_048_576,
			inputModalities:  []string{"text", "image"},
			features:         []string{"tools", "reasoning"},
		},
		"@cf/qwen/qwen3.8-27b": {
			ratio:           0.450 * ratio.MilliTokensUsd,
			completionRatio: 3.200 / 0.450,
			contextLength:   262_144,
			inputModalities: []string{"text", "image"},
			features:        []string{"tools", "reasoning"},
		},
	}

	for modelName, expected := range testCases {
		modelName := modelName
		expected := expected
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[modelName]
			require.True(t, ok, "%s missing from Cloudflare pricing map", modelName)
			require.InDelta(t, expected.ratio, cfg.Ratio, 1e-12)
			require.InDelta(t, expected.cachedInputRatio, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, expected.completionRatio, cfg.CompletionRatio, 1e-12)
			require.Equal(t, expected.contextLength, cfg.ContextLength)
			if expected.inputModalities != nil {
				require.ElementsMatch(t, expected.inputModalities, cfg.InputModalities)
			}
			if expected.features != nil {
				require.ElementsMatch(t, expected.features, cfg.SupportedFeatures)
			}
		})
	}
}

// TestModelRatiosExcludeDeprecatedWorkersAIModels verifies that the default
// catalog no longer exposes model identifiers whose deprecation date passed.
func TestModelRatiosExcludeDeprecatedWorkersAIModels(t *testing.T) {
	t.Parallel()

	for _, modelName := range deprecatedCloudflareModels20260530 {
		_, exists := ModelRatios[modelName]
		require.False(t, exists, "%s must not remain in Cloudflare pricing metadata", modelName)
		require.NotContains(t, ModelList, modelName, "%s must not remain in the selectable model list", modelName)
	}
}

// TestModelRatiosIncludeLatestWorkersAISpeechModels verifies current
// Cloudflare speech pricing metadata.
func TestModelRatiosIncludeLatestWorkersAISpeechModels(t *testing.T) {
	t.Parallel()

	melottsCfg, ok := ModelRatios["@cf/myshell-ai/melotts"]
	require.True(t, ok, "@cf/myshell-ai/melotts missing from Cloudflare pricing map")
	require.NotNil(t, melottsCfg.Audio, "expected audio pricing metadata for @cf/myshell-ai/melotts")
	require.InDelta(t, 0.0002/60.0, melottsCfg.Audio.UsdPerSecond, 1e-12)

	aura1Cfg, ok := ModelRatios["@cf/deepgram/aura-1"]
	require.True(t, ok, "@cf/deepgram/aura-1 missing from Cloudflare pricing map")
	require.InDelta(t, 15.0*ratio.MilliTokensUsd, aura1Cfg.Ratio, 1e-12)

	aura2EnCfg, ok := ModelRatios["@cf/deepgram/aura-2-en"]
	require.True(t, ok, "@cf/deepgram/aura-2-en missing from Cloudflare pricing map")
	require.InDelta(t, 30.0*ratio.MilliTokensUsd, aura2EnCfg.Ratio, 1e-12)

	aura2EsCfg, ok := ModelRatios["@cf/deepgram/aura-2-es"]
	require.True(t, ok, "@cf/deepgram/aura-2-es missing from Cloudflare pricing map")
	require.InDelta(t, 30.0*ratio.MilliTokensUsd, aura2EsCfg.Ratio, 1e-12)
}
