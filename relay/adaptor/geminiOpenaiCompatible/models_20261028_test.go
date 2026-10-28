package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiOctober28NanoBanana21Catalog checks discovery, Standard tariffs,
// output resolution tiers, and deliberately unsupported capabilities.
func TestGeminiOctober28NanoBanana21Catalog(t *testing.T) {
	t.Parallel()
	cfg, ok := ModelRatios[GeminiNanoBanana21Model]
	require.True(t, ok)
	require.Contains(t, ModelList, GeminiNanoBanana21Model)

	require.InDelta(t, 1.50, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 7.50, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.Zero(t, cfg.CachedInputRatio, "Developer API does not support model caching")
	require.EqualValues(t, 131_072, cfg.ContextLength)
	require.EqualValues(t, 32_768, cfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"text", "image", "video", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text", "image"}, cfg.OutputModalities)
	require.ElementsMatch(t, []string{"reasoning"}, cfg.SupportedFeatures)
	require.NotContains(t, cfg.SupportedFeatures, "tools")
	require.NotContains(t, cfg.SupportedFeatures, "structured_outputs")
	require.Equal(t, []string{"max_tokens"}, cfg.SupportedSamplingParameters)
	require.ElementsMatch(t, []string{"minimal", "medium", "high"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.Zero(t, cfg.MaxReasoningTokens)
	require.Nil(t, cfg.Audio)
	require.Nil(t, cfg.Video)
	require.Nil(t, cfg.Embedding)
	require.Empty(t, cfg.Tiers)
	require.Empty(t, cfg.TimeWindows)

	require.NotNil(t, cfg.Image)
	require.Equal(t, "1024x1024", cfg.Image.DefaultSize)
	require.Equal(t, "standard", cfg.Image.DefaultQuality)
	require.NotContains(t, cfg.Image.SizeMultipliers, "512x512")
	require.Len(t, cfg.Image.SizeMultipliers, 3)
	for _, tc := range []struct {
		size     string
		imageUsd float64
	}{
		{"1024x1024", 0.0336},
		{"2048x2048", 0.0504},
		{"4096x4096", 0.1134},
	} {
		multiplier, exists := cfg.Image.SizeMultipliers[tc.size]
		require.True(t, exists, "missing billing tier %s", tc.size)
		require.InDelta(t, tc.imageUsd, cfg.Image.PricePerImageUsd*multiplier, 1e-12)
	}
	require.Contains(t, cfg.Description, "Higher image resolution requires explicit native imageConfig")
}

// TestGeminiOctober28NanoBanana21Routing checks the named model's image
// response-mode policy and search tariff without relaxing other Gemini IDs.
func TestGeminiOctober28NanoBanana21Routing(t *testing.T) {
	t.Parallel()
	require.True(t, IsGeminiImageGenerationModel(GeminiNanoBanana21Model))
	require.True(t, IsGeminiImageGenerationModel("Gemini-Nano-Banana-2.1"))
	require.True(t, IsGeminiImageGenerationModel("gemini-3.1-flash-image"))
	require.False(t, IsGeminiImageGenerationModel("gemini-3.8-flash"))

	require.Equal(t, []string{ModalityText, ModalityImage}, GetModelModalities(GeminiNanoBanana21Model))
	require.Equal(t, []string{ModalityText, ModalityImage}, GetModelModalities("Gemini-Nano-Banana-2.1"))
	require.Nil(t, GetModelModalities("gemini-3.1-flash-image"), "numeric Gemini 2.5+ omits this field for backward compatibility")
	require.Nil(t, GetModelModalities("gemini-4-argon"))

	require.NotContains(t, geminiWebSearchModels, GeminiNanoBanana21Model)
	require.Empty(t, GeminiToolingDefaultsForModel(GeminiNanoBanana21Model).Pricing)
	require.InDelta(t, 0.035, GeminiToolingDefaultsForModel("gemini-2.5-flash").Pricing["web_search"].UsdPerCall, 1e-12)
}

// TestGeminiOctober28NanoBananaConfigIsolation verifies the image-tier map
// cannot leak mutations across independently constructed pricing entries.
func TestGeminiOctober28NanoBananaConfigIsolation(t *testing.T) {
	t.Parallel()
	first := geminiNanoBanana21Config()
	second := geminiNanoBanana21Config()
	first.Image.SizeMultipliers["4096x4096"] = 99
	first.InputModalities[0] = "mutated"
	first.OutputModalities[0] = "mutated"
	first.SupportedFeatures[0] = "mutated"
	first.SupportedReasoningEfforts[0] = "mutated"
	require.InDelta(t, 0.1134, second.Image.PricePerImageUsd*second.Image.SizeMultipliers["4096x4096"], 1e-12)
	require.Equal(t, "text", second.InputModalities[0])
	require.Equal(t, "text", second.OutputModalities[0])
	require.Equal(t, "web_search", second.SupportedFeatures[0])
	require.Equal(t, "minimal", second.SupportedReasoningEfforts[0])
}

// TestGeminiOctober28LifecycleMetadata verifies current dated migration advice
// while retaining selectable IDs and the existing pricing configurations.
func TestGeminiOctober28LifecycleMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, date, replacement string
	}{
		{"gemini-3.1-flash-tts-preview", "November 17, 2026", "gemini-3.8-flash-lite-tts"},
		{"gemini-2.5-flash-preview-tts", "November 17, 2026", "gemini-3.8-flash-lite-tts"},
		{"gemini-2.5-pro-preview-tts", "November 17, 2026", "gemini-3.8-flash-tts"},
		{"gemini-3.1-flash-live-preview", "November 17, 2026", "gemini-3.8-live"},
		{"gemini-2.5-flash-native-audio-preview-12-2025", "November 17, 2026", "gemini-3.8-live"},
		{"gemini-2.5-flash-image", "March 15, 2027", "gemini-3.1-flash-lite-image"},
		{"gemini-omni-flash-preview", "October 22, 2026", "gemini-omni-1.1-flash"},
		{"gemini-2.5-computer-use-preview-10-2025", "July 28, 2026", "gemini-3.8-flash"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[tc.id]
			require.True(t, ok, "never delete a configured legacy model")
			require.Contains(t, ModelList, tc.id)
			require.Contains(t, cfg.Description, tc.date)
			require.Contains(t, cfg.Description, tc.replacement)
			require.Positive(t, cfg.Ratio)
		})
	}
}
