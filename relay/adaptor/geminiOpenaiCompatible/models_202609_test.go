package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiSeptember2026FlashCatalog verifies Gemini 3.6, 3.7, and 3.8
// pricing, limits, reasoning levels, search pricing eligibility, and the 2027 price transition.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember2026FlashCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model            string
		reasoningEfforts []string
	}{
		{model: "gemini-3.6-flash", reasoningEfforts: gemini3FlashReasoningEfforts},
		{model: "gemini-3.7-flash", reasoningEfforts: gemini37PlusReasoningEfforts},
		{model: "gemini-3.8-flash", reasoningEfforts: gemini37PlusReasoningEfforts},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()

			config, ok := ModelRatios[tt.model]
			require.True(t, ok)
			require.InDelta(t, 0.75*ratio.MilliTokensUsd, config.Ratio, 1e-12)
			require.InDelta(t, 3.75/0.75, config.CompletionRatio, 1e-12)
			require.InDelta(t, 0.075*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
			require.EqualValues(t, 1_048_576, config.ContextLength)
			require.EqualValues(t, 65_536, config.MaxOutputTokens)
			require.Equal(t, tt.reasoningEfforts, config.SupportedReasoningEfforts)
			require.Contains(t, geminiWebSearchModels, tt.model)
			require.Contains(t, ModelList, tt.model)

			require.Len(t, config.TimeWindows, 1)
			window := config.TimeWindows[0]
			require.Equal(t, "UTC", window.TimeZone)
			require.Equal(t, "2027-01-01", window.DateFrom)
			require.Len(t, window.Ranges, 1)
			require.Equal(t, "00:00", window.Ranges[0].Start)
			require.Equal(t, "00:00", window.Ranges[0].End)
			require.InDelta(t, 1.50*ratio.MilliTokensUsd, window.Overlay.Ratio, 1e-12)
			require.InDelta(t, 7.50/1.50, window.Overlay.CompletionRatio, 1e-12)
			require.InDelta(t, 0.15*ratio.MilliTokensUsd, window.Overlay.CachedInputRatio, 1e-12)
		})
	}
}

// TestGeminiSeptember2026SpecializedCatalog verifies the newly published
// transcription, Omni, and Robotics ER 2 endpoints and their paid-tier pricing.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember2026SpecializedCatalog(t *testing.T) {
	t.Parallel()

	transcribe := ModelRatios["gemini-3.5-transcribe"]
	require.InDelta(t, 2.00*ratio.MilliTokensUsd, transcribe.Ratio, 1e-12)
	require.InDelta(t, 12.00/2.00, transcribe.CompletionRatio, 1e-12)
	require.Equal(t, []string{"audio"}, transcribe.InputModalities)
	require.Equal(t, []string{"text"}, transcribe.OutputModalities)
	require.NotNil(t, transcribe.Audio)
	require.InDelta(t, 25.0, transcribe.Audio.PromptTokensPerSecond, 1e-12)

	transcribeLive := ModelRatios["gemini-3.5-transcribe-live"]
	require.InDelta(t, 3.50*ratio.MilliTokensUsd, transcribeLive.Ratio, 1e-12)
	require.InDelta(t, 21.00/3.50, transcribeLive.CompletionRatio, 1e-12)

	omni := ModelRatios["gemini-omni-1.1-flash"]
	require.InDelta(t, 1.50*ratio.MilliTokensUsd, omni.Ratio, 1e-12)
	require.InDelta(t, 9.00/1.50, omni.CompletionRatio, 1e-12)
	require.Equal(t, []string{"video"}, omni.OutputModalities)
	require.NotNil(t, omni.Video)
	require.InDelta(t, 0.10136, omni.Video.PerSecondUsd, 1e-12)
	require.Equal(t, "1280x720", omni.Video.BaseResolution)

	roboticsTests := []struct {
		model             string
		cachedInputUsd    float64
		expectedFeatures []string
	}{
		{
			model:             "gemini-robotics-er-2-preview",
			cachedInputUsd:    0.20,
			expectedFeatures: []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"},
		},
		{
			model:             "gemini-robotics-er-2-streaming-preview",
			expectedFeatures: []string{"tools", "web_search", "reasoning"},
		},
	}

	for _, tt := range roboticsTests {
		t.Run(tt.model, func(t *testing.T) {
			config, ok := ModelRatios[tt.model]
			require.True(t, ok, "%s missing from pricing map", tt.model)
			require.InDelta(t, 2.00*ratio.MilliTokensUsd, config.Ratio, 1e-12)
			require.InDelta(t, 10.00/2.00, config.CompletionRatio, 1e-12)
			require.InDelta(t, tt.cachedInputUsd*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
			require.EqualValues(t, 131_072, config.ContextLength)
			require.EqualValues(t, 65_536, config.MaxOutputTokens)
			require.ElementsMatch(t, tt.expectedFeatures, config.SupportedFeatures)
			require.Contains(t, config.SupportedFeatures, "web_search")
			require.Contains(t, geminiWebSearchModels, tt.model)
		})
	}
}

// TestGeminiSeptember2026LifecycleMetadata verifies corrected replacement and
// shutdown guidance for existing catalog entries. Parameters: t is the current test handle.
// Returns: none.
func TestGeminiSeptember2026LifecycleMetadata(t *testing.T) {
	t.Parallel()

	require.Contains(t, ModelRatios["gemini-embedding-001"].Description, "May 14, 2028")
	require.Contains(t, ModelRatios["gemini-3.1-flash-lite"].Description, "May 7, 2027")
	require.Contains(t, ModelRatios["gemini-3-flash-preview"].Description, "gemini-3.6-flash")
	require.NotContains(t, ModelRatios["gemini-2.5-pro"].Description, "2026-10-16")
	require.NotContains(t, ModelRatios["gemini-2.5-flash"].Description, "2026-10-16")
	require.NotContains(t, ModelRatios["gemini-2.5-flash-lite"].Description, "2026-10-16")
	require.Contains(t, ModelRatios["gemini-robotics-er-1.6-preview"].Description, "August 31, 2026")
	require.Contains(t, ModelRatios["gemini-omni-flash-preview"].Description, "September 30, 2026")
}
