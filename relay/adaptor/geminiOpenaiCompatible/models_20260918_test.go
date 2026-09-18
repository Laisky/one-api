package geminiOpenaiCompatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiSeptember18LiveCatalog checks prices, capabilities, and derived lists.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember18LiveCatalog(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		model    string
		extended bool
	}{
		{model: "gemini-3.8-live"},
		{model: "gemini-3.8-live-extended-thinking", extended: true},
	} {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			config, ok := ModelRatios[tt.model]
			require.True(t, ok)
			require.Contains(t, ModelList, tt.model)
			require.Contains(t, geminiWebSearchModels, tt.model)
			require.NotNil(t, config.Audio)

			// Reconstruct absolute USD/1M prices to catch inverted audio ratios.
			require.InDelta(t, 0.75, config.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, 4.50, config.Ratio*config.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, 3.00, config.Ratio*config.Audio.PromptRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, 12.00, config.Ratio*config.CompletionRatio*config.Audio.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, 0.014, GeminiToolingDefaultsForModel(tt.model).Pricing["web_search"].UsdPerCall, 1e-12)
			require.EqualValues(t, 131_072, config.ContextLength)
			require.EqualValues(t, 65_536, config.MaxOutputTokens)
			require.ElementsMatch(t, []string{"text", "image", "audio", "video"}, config.InputModalities)
			require.ElementsMatch(t, []string{"text", "audio"}, config.OutputModalities)
			require.Zero(t, config.CachedInputRatio)
			require.Empty(t, config.TimeWindows)
			require.Empty(t, config.Tiers)
			require.Nil(t, config.Image)
			require.Nil(t, config.Video)
			require.Empty(t, config.DefaultReasoningEffort)
			require.Zero(t, config.MaxReasoningTokens)
			require.Empty(t, config.SupportedSamplingParameters)
			require.Contains(t, config.Description, "not implemented by this REST adaptor")

			if tt.extended {
				require.Equal(t, []string{"low", "medium", "high"}, config.SupportedReasoningEfforts)
				require.ElementsMatch(t, []string{"tools", "web_search", "reasoning"}, config.SupportedFeatures)
			} else {
				require.Empty(t, config.SupportedReasoningEfforts)
				require.ElementsMatch(t, []string{"tools", "web_search", "reasoning"}, config.SupportedFeatures)
			}
		})
	}
}

// TestGemini38LiveConfigIsolation checks that metadata builders do not share mutable state.
// Parameters: t is the current test handle. Returns: none.
func TestGemini38LiveConfigIsolation(t *testing.T) {
	t.Parallel()
	first := gemini38LiveConfig(true)
	second := gemini38LiveConfig(true)
	first.Audio.PromptRatio = 99
	first.InputModalities[0] = "changed"
	first.OutputModalities[0] = "changed"
	first.SupportedFeatures[0] = "changed"
	first.SupportedReasoningEfforts[0] = "changed"
	require.InDelta(t, 4.0, second.Audio.PromptRatio, 1e-12)
	require.Equal(t, "text", second.InputModalities[0])
	require.Equal(t, "text", second.OutputModalities[0])
	require.Equal(t, "tools", second.SupportedFeatures[0])
	require.Equal(t, "low", second.SupportedReasoningEfforts[0])
}

// TestGeminiSeptember18FlashSamplingMetadata rejects deprecated sampling advertisements.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember18FlashSamplingMetadata(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, []string{"stop", "max_tokens"}, ModelRatios[model].SupportedSamplingParameters)
		})
	}
	// The older Gemini 2.5 family keeps its independent sampling metadata.
	require.Contains(t, ModelRatios["gemini-2.5-flash"].SupportedSamplingParameters, "temperature")
}

// TestGeminiSeptember18LifecycleMetadata checks dates without deleting legacy configurations.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember18LifecycleMetadata(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		model       string
		date        string
		replacement string
	}{
		{"gemini-3-pro-preview", "March 9, 2026", "gemini-3.1-pro-preview"},
		{"gemini-3.1-flash-lite-preview", "May 25, 2026", "gemini-3.1-flash-lite"},
		{"gemini-3.1-flash-image-preview", "June 25, 2026", "gemini-3.1-flash-image"},
		{"gemini-3-pro-image-preview", "June 25, 2026", "gemini-3-pro-image"},
		{"gemini-2.5-flash-image", "earliest shutdown October 2, 2026", "gemini-3.1-flash-image"},
		{"gemini-2.5-flash-image-preview", "January 15, 2026", "gemini-3.1-flash-image"},
		{"gemini-3.1-flash-live-preview", "no shutdown date is announced", "gemini-3.8-live"},
	} {
		t.Run(tt.model, func(t *testing.T) {
			config, ok := ModelRatios[tt.model]
			require.True(t, ok)
			require.Contains(t, ModelList, tt.model)
			require.Contains(t, config.Description, tt.date)
			require.Contains(t, config.Description, tt.replacement)
			require.Positive(t, config.Ratio)
		})
	}
}
