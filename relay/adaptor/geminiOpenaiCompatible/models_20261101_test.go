package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiNovember2026ImageCatalog checks the stable image-model limits,
// modalities, capabilities and standard pricing against Google's model cards.
// Sources (verified 2026-11-01):
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-lite-image
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-image
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3-pro-image
//   - https://ai.google.dev/gemini-api/docs/pricing
func TestGeminiNovember2026ImageCatalog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		model           string
		contextLength   int32
		maxOutputTokens int32
		inputModalities []string
		reasoningLevels []string
		defaultEffort   string
		inputUsd        float64
		outputUsd       float64
		imageUsd        float64
	}{
		{
			model:           "gemini-3.1-flash-lite-image",
			contextLength:   65_536,
			maxOutputTokens: 4_096,
			inputModalities: []string{"text", "image", "video", "file"},
			reasoningLevels: []string{"minimal", "high"},
			defaultEffort:   "minimal",
			inputUsd:        0.25,
			outputUsd:       1.50,
			imageUsd:        0.0336,
		},
		{
			model:           "gemini-3.1-flash-image",
			contextLength:   131_072,
			maxOutputTokens: 32_768,
			inputModalities: []string{"text", "image", "video", "file"},
			inputUsd:        0.50,
			outputUsd:       3.00,
			imageUsd:        0.067,
		},
		{
			model:           "gemini-3-pro-image",
			contextLength:   65_536,
			maxOutputTokens: 32_768,
			inputModalities: []string{"text", "image"},
			inputUsd:        2.00,
			outputUsd:       12.00,
			imageUsd:        0.134,
		},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			cfg, exists := ModelRatios[tc.model]
			require.True(t, exists)
			require.Contains(t, ModelList, tc.model)
			require.Equal(t, tc.contextLength, cfg.ContextLength)
			require.Equal(t, tc.maxOutputTokens, cfg.MaxOutputTokens)
			require.Equal(t, tc.inputModalities, cfg.InputModalities)
			require.Equal(t, []string{"text", "image"}, cfg.OutputModalities)
			require.Equal(t, []string{"reasoning"}, cfg.SupportedFeatures)
			require.Equal(t, tc.reasoningLevels, cfg.SupportedReasoningEfforts)
			require.Equal(t, tc.defaultEffort, cfg.DefaultReasoningEffort)
			require.NotContains(t, cfg.SupportedFeatures, "structured_outputs")
			require.NotContains(t, cfg.SupportedFeatures, "json_mode")
			require.NotContains(t, cfg.SupportedFeatures, "tools")
			require.Zero(t, cfg.CachedInputRatio, "image models do not support context caching")
			require.Empty(t, cfg.Tiers, "do not inherit an unreachable 200K input surcharge")
			require.InDelta(t, tc.inputUsd*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, tc.outputUsd*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
			require.NotNil(t, cfg.Image)
			require.InDelta(t, tc.imageUsd, cfg.Image.PricePerImageUsd, 1e-12)
		})
	}

	// Historical IDs must remain configurable; do not delete or silently remap.
	for _, oldModel := range []string{"gemini-3.1-flash-image-preview", "gemini-3-pro-image-preview"} {
		require.Contains(t, ModelList, oldModel)
		require.NotNil(t, ModelRatios[oldModel].Image)
	}
}

// TestGeminiNovember2026ThinkingCatalog verifies model-specific defaults and
// levels without applying a modern model's constraints to historical variants.
// Source: https://ai.google.dev/gemini-api/docs/thinking (2026-11-01).
func TestGeminiNovember2026ThinkingCatalog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		model  string
		levels []string
		def    string
	}{
		{"gemini-3-flash-preview", []string{"minimal", "low", "medium", "high"}, "high"},
		{"gemini-3-pro-preview", []string{"low", "high"}, "high"},
		{"gemini-3.1-pro-preview", []string{"low", "medium", "high"}, "high"},
		{"gemini-3.8-flash", []string{"low", "medium", "high"}, "medium"},
		{"gemini-robotics-er-2-preview", []string{"minimal", "low", "medium", "high"}, "high"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			cfg, exists := ModelRatios[tc.model]
			require.True(t, exists)
			require.Equal(t, tc.levels, cfg.SupportedReasoningEfforts)
			require.Equal(t, tc.def, cfg.DefaultReasoningEffort)
		})
	}

	// The separate streaming Robotics preview has no published level table;
	// keep its existing Live-specific discovery defaults unchanged.
	streaming := ModelRatios["gemini-robotics-er-2-streaming-preview"]
	require.Equal(t, []string{"low", "medium", "high"}, streaming.SupportedReasoningEfforts)
	require.Equal(t, "medium", streaming.DefaultReasoningEffort)
}
