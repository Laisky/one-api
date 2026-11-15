package geminiOpenaiCompatible

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGeminiNovember12Catalog verifies the published Gemini 4 model contracts
// exposed by the shared catalogue. Parameter t is the test handle; it returns nothing.
func TestGeminiNovember12Catalog(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model, defaultEffort string
		maxOutput            int32
		efforts              []string
	}{
		{"gemini-4-carbon", "medium", 32_768, []string{"low", "medium", "high"}},
		{"gemini-4-hydrogen", "high", 65_536, []string{"low", "medium", "high", "xhigh"}},
		{"gemini-4-argon", "medium", 32_768, []string{"low", "medium", "high"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[tc.model]
			require.True(t, ok, "published model must not use fallback pricing")
			require.Contains(t, ModelList, tc.model)
			require.EqualValues(t, 1_048_576, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Equal(t, []string{"text", "image", "audio", "video", "file"}, cfg.InputModalities)
			require.Equal(t, []string{"text"}, cfg.OutputModalities)
			require.ElementsMatch(t, []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}, cfg.SupportedFeatures)
			require.Equal(t, []string{"max_tokens"}, cfg.SupportedSamplingParameters)
			require.Equal(t, tc.efforts, cfg.SupportedReasoningEfforts)
			require.Equal(t, tc.defaultEffort, cfg.DefaultReasoningEffort)
			require.Zero(t, cfg.MaxReasoningTokens, "level-based reasoning has no integer budget")
			require.Nil(t, cfg.Audio, "unified input tariff is not audio-generation pricing")
			require.Nil(t, cfg.Image)
			require.Nil(t, cfg.Video)
			require.Nil(t, cfg.Embedding)
			require.Empty(t, cfg.Tiers, "no Gemini 3 Pro long-context surcharge")
			require.Contains(t, cfg.Description, "implicit only")
			require.Contains(t, cfg.Description, "continuation")
			require.InDelta(t, 0.014, GeminiToolingDefaultsForModel(tc.model).Pricing["web_search"].UsdPerCall, 1e-12)
		})
	}
	for _, unsupported := range []string{"gemini-decision-1", "gemini-omni-1.2-flash", "gemini-4-carbon-cyber-permissive"} {
		require.NotContains(t, ModelList, unsupported, "separate protocols and Vertex-only IDs need their own integration")
	}
}

// TestGeminiNovember12PricingBoundary checks real pricing resolution on either
// side of the UTC promotional boundary. Parameter t is the test handle; it returns nothing.
func TestGeminiNovember12PricingBoundary(t *testing.T) {
	t.Parallel()
	boundary := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, modelName := range []string{"gemini-4-carbon", "gemini-4-hydrogen", "gemini-4-argon"} {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[modelName]
			require.True(t, ok)
			for _, at := range []time.Time{
				boundary.Add(-time.Nanosecond), boundary,
				boundary.In(time.FixedZone("UTC-5", -5*60*60)),
				time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
			} {
				input, output, cached := 2.0, 10.0, 0.10
				if !at.Before(boundary) {
					input, output, cached = 4.0, 20.0, 0.20
				}
				if modelName == "gemini-4-hydrogen" {
					input, output, cached = 1.50, 7.50, 0.075
				}
				resolved := pricing.ApplyTimeWindow(cfg, at)
				require.InDelta(t, input, resolved.Ratio/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, output, resolved.Ratio*resolved.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, cached, resolved.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
				require.Equal(t, cfg.SupportedReasoningEfforts, resolved.SupportedReasoningEfforts)
				require.Equal(t, cfg.MaxOutputTokens, resolved.MaxOutputTokens)
			}
			require.Equal(t, cfg, ModelRatios[modelName], "resolution must not mutate shared defaults")
		})
	}
}

// TestGeminiNovember12Lifecycle checks current migration advice while keeping
// configured historical IDs and tariffs. Parameter t is the test handle; it returns nothing.
func TestGeminiNovember12Lifecycle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ model, date, replacement string }{
		{"gemini-2.5-pro", "December 1, 2026", "gemini-3.8-flash"},
		{"gemini-2.5-flash", "December 1, 2026", "gemini-3.5-flash-lite"},
		{"gemini-embedding-2-preview", "November 6, 2026", "gemini-embedding-2"},
		{"gemini-embedding-001", "May 14, 2028", "gemini-embedding-2"},
		{"gemini-omni-1.1-flash", "December 3, 2026", "gemini-omni-1.2-flash"},
	} {
		cfg, ok := ModelRatios[tc.model]
		require.True(t, ok)
		require.Contains(t, ModelList, tc.model)
		require.Contains(t, cfg.Description, tc.date)
		require.Contains(t, cfg.Description, tc.replacement)
	}
	for modelName, inputUsd := range map[string]float64{"gemini-2.5-pro": 1.25, "gemini-2.5-flash": 0.30} {
		cfg := ModelRatios[modelName]
		require.InDelta(t, inputUsd, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.Empty(t, cfg.TimeWindows, "lifecycle notes must not change legacy tariffs or infer Vertex redirects")
		require.NotContains(t, cfg.Description, "shutdown 2026-10-16", "remove obsolete shutdown claim")
		require.Contains(t, cfg.Description, "historical pricing")
	}
	stable, preview := ModelRatios["gemini-embedding-2"], ModelRatios["gemini-embedding-2-preview"]
	require.EqualValues(t, 8_192, stable.ContextLength)
	require.Equal(t, stable.Ratio, preview.Ratio)
	require.Equal(t, stable.Embedding, preview.Embedding)
	require.InDelta(t, 0.20, stable.Ratio/ratio.MilliTokensUsd, 1e-12)
	for _, existing := range []string{"gemini-3.8-flash", "gemini-3.5-flash-lite", "gemini-3.8-live", "gemini-3.8-flash-tts", "gemini-nano-banana-2.1"} {
		require.Contains(t, ModelList, existing)
	}
}

// TestGemini4ConfigIsolation verifies construction never shares mutable tariff
// or capability fields. Parameter t is the test handle; it returns nothing.
func TestGemini4ConfigIsolation(t *testing.T) {
	t.Parallel()
	carbon, secondCarbon, argon := gemini4CarbonConfig(), gemini4CarbonConfig(), gemini4ArgonConfig()
	hydrogen, secondHydrogen := gemini4HydrogenConfig(), gemini4HydrogenConfig()
	carbon.TimeWindows[0].Overlay.Ratio = 99
	carbon.TimeWindows[0].Ranges[0].Start = "12:00"
	carbon.InputModalities[0] = "mutated"
	carbon.SupportedReasoningEfforts[0] = "mutated"
	hydrogen.SupportedReasoningEfforts[0] = "mutated"
	for _, untouched := range []adaptor.ModelConfig{secondCarbon, argon} {
		require.InDelta(t, 4.0, untouched.TimeWindows[0].Overlay.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.Equal(t, "00:00", untouched.TimeWindows[0].Ranges[0].Start)
		require.Equal(t, "text", untouched.InputModalities[0])
		require.Equal(t, "low", untouched.SupportedReasoningEfforts[0])
	}
	require.Empty(t, hydrogen.TimeWindows, "Hydrogen has no published promotional expiry")
	require.Equal(t, "low", secondHydrogen.SupportedReasoningEfforts[0])
	for _, name := range []string{"gemini-4-argon", "Gemini-4-Carbon", "GEMINI-4-HYDROGEN"} {
		require.True(t, IsGemini4Model(name))
	}
	for _, name := range []string{"gemini-4-carbon-cyber-permissive", "gemini-4-carbon-preview", "gemini-4-unknown", "gemini-3.8-flash"} {
		require.False(t, IsGemini4Model(name), "do not guess an unknown model's contract")
	}
}
