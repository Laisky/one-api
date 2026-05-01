package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

func TestDallE3HasPerImagePricing(t *testing.T) {
	cfg, ok := ModelRatios["dall-e-3"]
	require.True(t, ok, "dall-e-3 not found in ModelRatios")
	require.Equal(t, 0.0, cfg.Ratio, "expected Ratio=0 for per-image model")
	require.NotNil(t, cfg.Image, "expected image config for dall-e-3")
	require.Greater(t, cfg.Image.PricePerImageUsd, 0.0, "expected price_per_image_usd > 0 for dall-e-3")
}

func TestOpenAIToolingDefaultsWebSearchPricing(t *testing.T) {
	defaults := OpenAIToolingDefaults
	pricing, ok := defaults.Pricing["web_search"]
	require.True(t, ok, "web search pricing missing for OpenAI defaults")
	require.InDelta(t, 0.01, pricing.UsdPerCall, 1e-9, "expected base web search pricing")
	require.Empty(t, defaults.Whitelist, "expected built-in allowlist to be inferred from pricing")

	keys := make([]string, 0, len(defaults.Pricing))
	for name := range defaults.Pricing {
		keys = append(keys, name)
	}
	require.ElementsMatch(t, []string{
		"code_interpreter",
		"file_search",
		"web_search",
		"web_search_preview_reasoning",
		"web_search_preview_non_reasoning",
	}, keys, "expected pricing map to enumerate all OpenAI built-in tools")
}

func TestRealtimeModelsIncludeCurrentStableIDs(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		ratio            float64
		cachedInputRatio float64
		completionRatio  float64
	}{
		"gpt-realtime-1.5": {
			ratio:            4.0 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.4 * ratio.MilliTokensUsd,
			completionRatio:  4.0,
		},
		"gpt-realtime-mini": {
			ratio:            0.6 * ratio.MilliTokensUsd,
			cachedInputRatio: 0.06 * ratio.MilliTokensUsd,
			completionRatio:  4.0,
		},
	}

	for modelName, expected := range testCases {
		cfg, ok := ModelRatios[modelName]
		require.True(t, ok, "%s missing from pricing map", modelName)
		require.InDelta(t, expected.ratio, cfg.Ratio, 1e-12)
		require.InDelta(t, expected.cachedInputRatio, cfg.CachedInputRatio, 1e-12)
		require.InDelta(t, expected.completionRatio, cfg.CompletionRatio, 1e-12)
	}
}

// TestOpenAIAudioMetadata verifies explicit token limits for transcribe and mini-tts models.
// Parameter t coordinates the test run. Returns no values.
func TestOpenAIAudioMetadata(t *testing.T) {
	t.Parallel()

	transcribeCfg, ok := ModelRatios["gpt-4o-transcribe"]
	require.True(t, ok)
	require.EqualValues(t, 16000, transcribeCfg.ContextLength)
	require.EqualValues(t, 2000, transcribeCfg.MaxOutputTokens)

	miniTranscribeCfg, ok := ModelRatios["gpt-4o-mini-transcribe"]
	require.True(t, ok)
	require.EqualValues(t, 16000, miniTranscribeCfg.ContextLength)
	require.EqualValues(t, 2000, miniTranscribeCfg.MaxOutputTokens)

	miniTTSCfg, ok := ModelRatios["gpt-4o-mini-tts"]
	require.True(t, ok)
	require.EqualValues(t, 2000, miniTTSCfg.ContextLength)
}
