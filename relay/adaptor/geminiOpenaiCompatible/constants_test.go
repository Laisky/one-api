package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

func TestGeminiWebSearchPricingApplied(t *testing.T) {
	t.Parallel()
	pricing, ok := geminiToolingDefaults.Pricing["web_search"]
	require.True(t, ok, "web search pricing missing for gemini defaults")
	require.InDelta(t, 0.035, pricing.UsdPerCall, 1e-9, "expected $0.035 per grounded search call")
	require.Empty(t, geminiToolingDefaults.Whitelist, "gemini defaults should not restrict whitelist")
}

func TestGeminiTieredPricingConfigured(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-3-pro-preview"]
	require.True(t, ok, "gemini-3-pro-preview missing from pricing map")
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.Len(t, cfg.Tiers, 1, "expected a single tier for gemini-3-pro-preview")
	tier := cfg.Tiers[0]
	require.Equal(t, 200001, tier.InputTokenThreshold)
	require.InDelta(t, 4.0*ratio.MilliTokensUsd, tier.Ratio, 1e-12)
	require.InDelta(t, 18.0/4.0, tier.CompletionRatio, 1e-9)
}

func TestGeminiFlashAudioPricing(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-2.5-flash"]
	require.True(t, ok, "gemini-2.5-flash missing from pricing map")
	require.NotNil(t, cfg.Audio, "gemini-2.5-flash audio pricing missing")
	require.InDelta(t, 1.0/0.30, cfg.Audio.PromptRatio, 1e-9)
	require.InDelta(t, 0.30, cfg.Audio.CompletionRatio, 1e-9)
}

func TestGemini3FlashPricing(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-3-flash-preview"]
	require.True(t, ok, "gemini-3-flash-preview missing from pricing map")
	require.InDelta(t, 0.50*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 3.00/0.50, cfg.CompletionRatio, 1e-9)
	require.NotNil(t, cfg.Audio, "gemini-3-flash-preview audio pricing missing")
	require.InDelta(t, 1.00/0.50, cfg.Audio.PromptRatio, 1e-9)
	require.InDelta(t, 3.00/1.00, cfg.Audio.CompletionRatio, 1e-9)
}

func TestGeminiEmbeddingConfig(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-embedding-001"]
	require.True(t, ok, "gemini-embedding-001 missing from pricing map")
	require.InDelta(t, 0.15*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
}

func TestGemini3ProImagePreviewPricing(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-3-pro-image-preview"]
	require.True(t, ok, "gemini-3-pro-image-preview missing from pricing map")
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.NotNil(t, cfg.Image, "expected image pricing metadata for gemini-3-pro-image-preview")
	require.InDelta(t, gemini3ProImageBasePrice, cfg.Image.PricePerImageUsd, 1e-12)
	require.Len(t, cfg.Tiers, 1, "expected large-context tier to be defined")
	require.Contains(t, cfg.Image.SizeMultipliers, "1024x1024")
	require.Contains(t, cfg.Image.SizeMultipliers, "2048x2048")
	require.Contains(t, cfg.Image.SizeMultipliers, "4096x4096")
	require.InDelta(t, gemini3ProImage4KPrice/gemini3ProImageBasePrice, cfg.Image.SizeMultipliers["4096x4096"], 1e-12)
}

func TestGetModelModalitiesGeminiVersionCutoff(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		model    string
		expected []string
	}{
		{name: "LegacyGeminiText", model: "gemini-2.4-flash", expected: []string{ModalityText}},
		{name: "CutoffGemini", model: "gemini-2.5-flash", expected: nil},
		{name: "FutureGemini", model: "gemini-3-pro-preview", expected: nil},
		{name: "FutureGemini", model: "gemini-3.0-pro-preview", expected: nil},
		{name: "MixedCaseGemini", model: "Gemini-2.5-Flash", expected: nil},
		{name: "RoboticsNoVersion", model: "gemini-robotics-er-1.5-preview", expected: []string{ModalityText}},
		{name: "ImageBeforeCutoff", model: "gemini-2.0-flash-image", expected: []string{ModalityText, ModalityImage}},
		{name: "ImageAfterCutoff", model: "gemini-2.5-flash-image", expected: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			modalities := GetModelModalities(tc.model)
			require.Equal(t, tc.expected, modalities)
		})
	}
}

func TestGeminiVersionAtLeast(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		model    string
		min      float64
		expected bool
	}{
		{model: "gemini-2.5-flash", min: 2.5, expected: true},
		{model: "Gemini-3-Pro-Preview", min: 2.5, expected: true},
		{model: "Gemini-3.0-Pro-Preview", min: 2.5, expected: true},
		{model: "gemini-2.4-flash", min: 2.5, expected: false},
		{model: "not-gemini", min: 2.5, expected: false},
		{model: "", min: 2.5, expected: false},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.expected, GeminiVersionAtLeast(tc.model, tc.min), tc.model)
	}
}
