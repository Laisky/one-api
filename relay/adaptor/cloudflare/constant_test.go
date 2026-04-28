package cloudflare

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

func TestModelRatiosIncludeLatestWorkersAITextModels(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		ratio            float64
		cachedInputRatio float64
		completionRatio  float64
	}{
		"@cf/zai-org/glm-4.7-flash": {
			ratio:           0.060 * ratio.MilliTokensUsd,
			completionRatio: 0.400 / 0.060,
		},
		"@cf/nvidia/nemotron-3-120b-a12b": {
			ratio:           0.500 * ratio.MilliTokensUsd,
			completionRatio: 1.500 / 0.500,
		},
		"@cf/moonshotai/kimi-k2.5": {
			ratio:            0.600 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.100 * ratio.MilliTokensUsd,
			completionRatio:  3.000 / 0.600,
		},
	}

	for modelName, expected := range testCases {
		cfg, ok := ModelRatios[modelName]
		require.True(t, ok, "%s missing from Cloudflare pricing map", modelName)
		require.InDelta(t, expected.ratio, cfg.Ratio, 1e-12)
		require.InDelta(t, expected.cachedInputRatio, cfg.CachedInputRatio, 1e-12)
		require.InDelta(t, expected.completionRatio, cfg.CompletionRatio, 1e-12)
	}
}

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
