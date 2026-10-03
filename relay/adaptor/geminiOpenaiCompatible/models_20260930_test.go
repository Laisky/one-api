package geminiOpenaiCompatible

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGeminiSeptember30TTSCatalog checks exact IDs, limits, and audio-only capabilities.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember30TTSCatalog(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			config, ok := ModelRatios[model]
			require.True(t, ok)
			require.Contains(t, ModelList, model)
			require.EqualValues(t, 8_192, config.ContextLength)
			require.EqualValues(t, 16_384, config.MaxOutputTokens)
			require.Equal(t, []string{"text"}, config.InputModalities)
			require.Equal(t, []string{"audio"}, config.OutputModalities)
			require.Empty(t, config.SupportedFeatures)
			require.Empty(t, config.SupportedSamplingParameters)
			require.Empty(t, config.SupportedReasoningEfforts)
			require.Empty(t, config.DefaultReasoningEffort)
			require.Zero(t, config.MaxReasoningTokens)
			require.NotContains(t, geminiWebSearchModels, model)
			require.Nil(t, config.Image)
			require.Nil(t, config.Video)
			require.Nil(t, config.Embedding)
			require.Empty(t, config.Tiers)
			require.NotNil(t, config.Audio)
			require.Zero(t, config.Audio.PromptTokensPerSecond)
			require.EqualValues(t, 25, config.Audio.CompletionTokensPerSecond)
			require.Contains(t, config.Description, "speech_metadata")
			require.Contains(t, config.Description, "WAV")
			require.Contains(t, config.Description, "/v1/audio/speech")
		})
	}
}

// TestGeminiSeptember30PricingBoundary checks the production time-window resolver
// at both sides of the 2027 transition, including an equivalent non-UTC instant.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember30PricingBoundary(t *testing.T) {
	t.Parallel()
	models := []struct {
		name      string
		inputUsd  float64
		outputUsd float64
		cachedUsd float64
		audio     bool
	}{
		{"gemini-3.8-flash-tts", 0.50, 9.00, 0.125, true},
		{"gemini-3.8-flash-lite-tts", 0.50, 6.00, 0.125, true},
		{"gemini-robotics-er-2-preview", 1.00, 5.00, 0.10, false},
		{"gemini-robotics-er-2-streaming-preview", 1.00, 5.00, 0, false},
	}
	instants := []struct {
		name       string
		at         time.Time
		multiplier float64
	}{
		{"audit-date", time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC), 1},
		{"last-promotional-instant", time.Date(2026, time.December, 31, 23, 59, 59, 999_999_999, time.UTC), 1},
		{"first-standard-instant", time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC), 2},
		{"same-instant-non-UTC", time.Date(2026, time.December, 31, 19, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60)), 2},
	}
	for _, model := range models {
		t.Run(model.name, func(t *testing.T) {
			t.Parallel()
			config, ok := ModelRatios[model.name]
			require.True(t, ok)
			require.Len(t, config.TimeWindows, 1)
			require.Equal(t, "UTC", config.TimeWindows[0].TimeZone)
			require.Equal(t, "2027-01-01", config.TimeWindows[0].DateFrom)
			for _, instant := range instants {
				t.Run(instant.name, func(t *testing.T) {
					resolved := pricing.ApplyTimeWindow(config, instant.at)
					require.InDelta(t, model.inputUsd*instant.multiplier, resolved.Ratio/ratio.MilliTokensUsd, 1e-12)
					require.InDelta(t, model.outputUsd*instant.multiplier, resolved.Ratio*resolved.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
					require.InDelta(t, model.cachedUsd*instant.multiplier, resolved.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
					if !model.audio {
						require.Nil(t, resolved.Audio)
						return
					}
					require.NotNil(t, resolved.Audio)
					// Match quota.Compute's audio formula, not just the text fallback.
					audioUsd := resolved.Ratio * resolved.Audio.PromptRatio * resolved.Audio.CompletionRatio / ratio.MilliTokensUsd
					require.InDelta(t, model.outputUsd*instant.multiplier, audioUsd, 1e-12)
					require.InDelta(t, model.outputUsd*instant.multiplier*250/1_000_000,
						audioUsd*resolved.Audio.CompletionTokensPerSecond*10/1_000_000, 1e-12)
				})
			}
		})
	}
}

// TestGemini38TTSConfigIsolation ensures mutations cannot leak across model entries.
// Parameters: t is the current test handle. Returns: none.
func TestGemini38TTSConfigIsolation(t *testing.T) {
	t.Parallel()
	first := gemini38TTSConfig(9, "First")
	second := gemini38TTSConfig(9, "Second")
	first.Audio.CompletionRatio = 99
	first.InputModalities[0] = "changed"
	first.OutputModalities[0] = "changed"
	first.TimeWindows[0].Overlay.Ratio = 99
	first.TimeWindows[0].Ranges[0].Start = "12:00"
	require.InDelta(t, 18, second.Audio.CompletionRatio, 1e-12)
	require.Equal(t, []string{"text"}, second.InputModalities)
	require.Equal(t, []string{"audio"}, second.OutputModalities)
	require.InDelta(t, ratio.MilliTokensUsd, second.TimeWindows[0].Overlay.Ratio, 1e-12)
	require.Equal(t, "00:00", second.TimeWindows[0].Ranges[0].Start)
}

// TestGeminiSeptember30LifecyclePreservesLegacyPricing checks migration guidance
// without assuming a shutdown, deleting IDs, or copying new prices onto old models.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember30LifecyclePreservesLegacyPricing(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		model       string
		inputUsd    float64
		outputUsd   float64
		replacement string
	}{
		{"gemini-3.1-flash-tts-preview", 1, 20, "gemini-3.8-flash-lite-tts"},
		{"gemini-2.5-flash-preview-tts", 0.5, 10, "gemini-3.8-flash-lite-tts"},
		{"gemini-2.5-pro-preview-tts", 1, 20, "gemini-3.8-flash-tts"},
	} {
		t.Run(tt.model, func(t *testing.T) {
			config, ok := ModelRatios[tt.model]
			require.True(t, ok)
			require.Contains(t, ModelList, tt.model)
			require.Contains(t, config.Description, "no shutdown date is announced")
			require.Contains(t, config.Description, tt.replacement)
			require.InDelta(t, tt.inputUsd, config.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tt.outputUsd, config.Ratio*config.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.Empty(t, config.TimeWindows)
		})
	}
	for _, model := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"} {
		require.Contains(t, ModelList, model)
		require.Contains(t, ModelRatios[model].Description, "prior active usage")
		require.Contains(t, ModelRatios[model].Description, "not deprecated")
	}
	require.Contains(t, ModelRatios["gemini-omni-flash-preview"].Description, "earliest shutdown September 30, 2026")
}
