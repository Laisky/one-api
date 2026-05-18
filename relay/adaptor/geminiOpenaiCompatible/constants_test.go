package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
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
	testCases := []struct {
		model         string
		expectedRatio float64
	}{
		{model: "gemini-embedding-001", expectedRatio: geminiEmbedding001TextPrice * ratio.MilliTokensUsd},
		{model: "gemini-embedding-2-preview", expectedRatio: geminiEmbedding2PreviewTextPrice * ratio.MilliTokensUsd},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[tc.model]
			require.True(t, ok, "%s missing from pricing map", tc.model)
			require.InDelta(t, tc.expectedRatio, cfg.Ratio, 1e-12)
			require.InDelta(t, 1.0, cfg.CompletionRatio, 1e-12)
			if tc.model == "gemini-embedding-2-preview" {
				require.NotNil(t, cfg.Embedding, "expected multimodal embedding pricing metadata")
				require.InDelta(t, geminiEmbedding2PreviewImagePrice*ratio.MilliTokensUsd, cfg.Embedding.ImageTokenRatio, 1e-12)
				require.InDelta(t, geminiEmbedding2PreviewAudioPrice*ratio.MilliTokensUsd, cfg.Embedding.AudioTokenRatio, 1e-12)
				require.InDelta(t, geminiEmbedding2PreviewVideoPrice*ratio.MilliTokensUsd, cfg.Embedding.VideoTokenRatio, 1e-12)
				require.InDelta(t, geminiEmbedding2PreviewUsdPerImage, cfg.Embedding.UsdPerImage, 1e-12)
				require.InDelta(t, geminiEmbedding2PreviewUsdPerAudioSecond, cfg.Embedding.UsdPerAudioSecond, 1e-12)
				require.InDelta(t, geminiEmbedding2PreviewUsdPerVideoFrame, cfg.Embedding.UsdPerVideoFrame, 1e-12)
			}
		})
	}
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

func TestGemini31FlashImagePreviewPricing(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios["gemini-3.1-flash-image-preview"]
	require.True(t, ok, "gemini-3.1-flash-image-preview missing from pricing map")
	require.InDelta(t, 0.50*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 3.00/0.50, cfg.CompletionRatio, 1e-9)
	require.NotNil(t, cfg.Image, "expected image pricing metadata for gemini-3.1-flash-image-preview")
	require.InDelta(t, gemini31FlashImage1KPrice, cfg.Image.PricePerImageUsd, 1e-12)
	require.Contains(t, cfg.Image.SizeMultipliers, "512x512")
	require.Contains(t, cfg.Image.SizeMultipliers, "1024x1024")
	require.Contains(t, cfg.Image.SizeMultipliers, "2048x2048")
	require.Contains(t, cfg.Image.SizeMultipliers, "4096x4096")
	require.InDelta(t, gemini31FlashImage512Price/gemini31FlashImage1KPrice, cfg.Image.SizeMultipliers["512x512"], 1e-12)
	require.InDelta(t, gemini31FlashImage2KPrice/gemini31FlashImage1KPrice, cfg.Image.SizeMultipliers["2048x2048"], 1e-12)
	require.InDelta(t, gemini31FlashImage4KPrice/gemini31FlashImage1KPrice, cfg.Image.SizeMultipliers["4096x4096"], 1e-12)
}

func TestGemini31FlashLivePreviewPricing(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["gemini-3.1-flash-live-preview"]
	require.True(t, ok, "gemini-3.1-flash-live-preview missing from pricing map")
	require.InDelta(t, 0.75*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 4.50/0.75, cfg.CompletionRatio, 1e-12)
	require.NotNil(t, cfg.Audio, "expected live preview audio pricing metadata")
	require.InDelta(t, 3.00/0.75, cfg.Audio.PromptRatio, 1e-12)
	require.InDelta(t, 12.00/4.50, cfg.Audio.CompletionRatio, 1e-12)
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

// TestGeminiMetadataOverrides verifies researched model limits and feature metadata for Gemini image and flash families.
// Parameter t drives test execution. Returns no values.
func TestGeminiMetadataOverrides(t *testing.T) {
	t.Parallel()

	flashCfg, ok := ModelRatios["gemini-3-flash-preview"]
	require.True(t, ok)
	require.EqualValues(t, 65536, flashCfg.MaxOutputTokens)
	// Gemini 3 Flash defaults to "minimal" thinking per the official docs.
	require.Equal(t, "minimal", flashCfg.DefaultReasoningEffort)

	flashLiteCfg, ok := ModelRatios["gemini-3.1-flash-lite-preview"]
	require.True(t, ok)
	require.EqualValues(t, 65536, flashLiteCfg.MaxOutputTokens)

	flashImageCfg, ok := ModelRatios["gemini-3.1-flash-image-preview"]
	require.True(t, ok)
	require.EqualValues(t, 131072, flashImageCfg.ContextLength)
	require.EqualValues(t, 32768, flashImageCfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"json_mode", "structured_outputs"}, flashImageCfg.SupportedFeatures)

	proImageCfg, ok := ModelRatios["gemini-3-pro-image-preview"]
	require.True(t, ok)
	require.EqualValues(t, 65536, proImageCfg.ContextLength)
	require.EqualValues(t, 32768, proImageCfg.MaxOutputTokens)

	flash25ImageCfg, ok := ModelRatios["gemini-2.5-flash-image"]
	require.True(t, ok)
	require.EqualValues(t, 65536, flash25ImageCfg.ContextLength)
	require.EqualValues(t, 32768, flash25ImageCfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"json_mode", "structured_outputs"}, flash25ImageCfg.SupportedFeatures)
}

// TestGemini31FlashLiteGAPricing verifies the newly registered GA tier mirrors the preview pricing
// and exposes parity metadata (context, modalities, reasoning levels) for OpenRouter discovery.
func TestGemini31FlashLiteGAPricing(t *testing.T) {
	t.Parallel()

	ga, ok := ModelRatios["gemini-3.1-flash-lite"]
	require.True(t, ok, "gemini-3.1-flash-lite missing from pricing map")
	require.InDelta(t, 0.25*ratio.MilliTokensUsd, ga.Ratio, 1e-12)
	require.InDelta(t, 1.50/0.25, ga.CompletionRatio, 1e-9)
	require.InDelta(t, 0.025*ratio.MilliTokensUsd, ga.CachedInputRatio, 1e-12)
	require.NotNil(t, ga.Audio, "expected audio pricing for Flash Lite GA")
	require.InDelta(t, 0.50/0.25, ga.Audio.PromptRatio, 1e-9)
	require.EqualValues(t, 1_048_576, ga.ContextLength)
	require.EqualValues(t, 65536, ga.MaxOutputTokens)
	require.Equal(t, "minimal", ga.DefaultReasoningEffort)

	preview, ok := ModelRatios["gemini-3.1-flash-lite-preview"]
	require.True(t, ok)
	require.InDelta(t, ga.Ratio, preview.Ratio, 1e-12)
	require.InDelta(t, ga.CompletionRatio, preview.CompletionRatio, 1e-9)
}

// TestGemini31FlashTtsPreviewPricing verifies the new Gemini 3.1 Flash TTS preview entry charges
// $1.00 input / $20.00 output per Google's published pricing.
func TestGemini31FlashTtsPreviewPricing(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["gemini-3.1-flash-tts-preview"]
	require.True(t, ok, "gemini-3.1-flash-tts-preview missing from pricing map")
	require.InDelta(t, 1.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 20.0/1.0, cfg.CompletionRatio, 1e-9)
	require.NotNil(t, cfg.Audio, "expected audio pricing for TTS preview")
}

// TestGeminiRoboticsER16Pricing verifies the new Gemini Robotics-ER 1.6 preview entry charges
// $1.00 input (text/image/video), $2.00 audio input, and $5.00 output per Google's pricing.
func TestGeminiRoboticsER16Pricing(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["gemini-robotics-er-1.6-preview"]
	require.True(t, ok, "gemini-robotics-er-1.6-preview missing from pricing map")
	require.InDelta(t, 1.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 5.0/1.0, cfg.CompletionRatio, 1e-9)
	require.NotNil(t, cfg.Audio, "expected audio pricing for robotics 1.6")
	require.InDelta(t, 2.0/1.0, cfg.Audio.PromptRatio, 1e-9)
}

// TestGeminiEmbedding2GAAlias verifies that the GA gemini-embedding-2 alias mirrors the preview entry.
func TestGeminiEmbedding2GAAlias(t *testing.T) {
	t.Parallel()

	ga, ok := ModelRatios["gemini-embedding-2"]
	require.True(t, ok, "gemini-embedding-2 missing from pricing map")
	require.InDelta(t, geminiEmbedding2PreviewTextPrice*ratio.MilliTokensUsd, ga.Ratio, 1e-12)
	require.NotNil(t, ga.Embedding, "expected multimodal embedding metadata for GA tier")
	require.InDelta(t, geminiEmbedding2PreviewImagePrice*ratio.MilliTokensUsd, ga.Embedding.ImageTokenRatio, 1e-12)
}
