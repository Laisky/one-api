package geminiOpenaiCompatible

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGeminiOctober13ArgonCatalog verifies published capabilities and conservative
// synchronous limits. Parameter t is the test handle; it returns no values.
func TestGeminiOctober13ArgonCatalog(t *testing.T) {
	const modelName = "gemini-4-argon"
	cfg, exists := ModelRatios[modelName]
	require.True(t, exists, "Argon must have explicit pricing, not fallback rates")
	require.Contains(t, ModelList, modelName)
	require.Equal(t, int32(1_048_576), cfg.ContextLength)
	require.Equal(t, int32(65_536), cfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"text", "image", "audio", "video", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.ElementsMatch(t, []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}, cfg.SupportedFeatures)
	require.Equal(t, []string{"max_tokens"}, cfg.SupportedSamplingParameters)
	require.Equal(t, []string{"low", "medium", "high"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.Zero(t, cfg.MaxReasoningTokens, "no documented integer thinking-budget ceiling")
	require.Nil(t, cfg.Image)
	require.Nil(t, cfg.Audio)
	require.Nil(t, cfg.Video)
	require.Nil(t, cfg.Embedding)
	require.Empty(t, cfg.Tiers, "do not inherit Pro's 200K long-context surcharge")
	require.Contains(t, cfg.Description, "implicit only")
	require.Contains(t, geminiWebSearchModels, modelName)
	require.Nil(t, GetModelModalities(modelName))
	require.InDelta(t, 0.014, GeminiToolingDefaultsForModel(modelName).Pricing["web_search"].UsdPerCall, 1e-12)
}

// TestGeminiOctober13ArgonPricingBoundary exercises the production time-window
// resolver with fixed documented prices. Parameter t is the test handle; it
// returns no values. The date-only tariff follows the repository's UTC policy.
func TestGeminiOctober13ArgonPricingBoundary(t *testing.T) {
	cfg, exists := ModelRatios["gemini-4-argon"]
	require.True(t, exists)
	boundary := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name   string
		at     time.Time
		input  float64
		output float64
		cache  float64
	}{
		{"release", time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC), 2, 10, 0.10},
		{"last-promotional-nanosecond", boundary.Add(-time.Nanosecond), 2, 10, 0.10},
		{"first-standard-nanosecond", boundary, 4, 20, 0.20},
		{"same-instant-west-of-UTC", boundary.In(time.FixedZone("UTC-5", -5*60*60)), 4, 20, 0.20},
		{"same-instant-east-of-UTC", boundary.In(time.FixedZone("UTC+8", 8*60*60)), 4, 20, 0.20},
		{"standard-persists", time.Date(2030, time.July, 1, 12, 0, 0, 0, time.UTC), 4, 20, 0.20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resolved := pricing.ApplyTimeWindow(cfg, tt.at)
			require.InDelta(t, tt.input*ratio.MilliTokensUsd, resolved.Ratio, 1e-12)
			require.InDelta(t, tt.output*ratio.MilliTokensUsd, resolved.Ratio*resolved.CompletionRatio, 1e-12)
			require.InDelta(t, tt.cache*ratio.MilliTokensUsd, resolved.CachedInputRatio, 1e-12)
			require.Equal(t, cfg.ContextLength, resolved.ContextLength)
			require.Equal(t, cfg.MaxOutputTokens, resolved.MaxOutputTokens)
		})
	}
	// Resolving future prices must not mutate the shared introductory defaults.
	require.InDelta(t, 2*ratio.MilliTokensUsd, ModelRatios["gemini-4-argon"].Ratio, 1e-12)
	require.InDelta(t, 0.10*ratio.MilliTokensUsd, ModelRatios["gemini-4-argon"].CachedInputRatio, 1e-12)
}

// TestGeminiOctober13PreservesExistingTariffs protects unrelated Flash tariffs
// while Argon crosses its earlier boundary. Parameter t is the test handle; it
// returns no values.
func TestGeminiOctober13PreservesExistingTariffs(t *testing.T) {
	at := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	for _, modelName := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		t.Run(modelName, func(t *testing.T) {
			cfg, exists := ModelRatios[modelName]
			require.True(t, exists)
			resolved := pricing.ApplyTimeWindow(cfg, at)
			require.InDelta(t, 0.75*ratio.MilliTokensUsd, resolved.Ratio, 1e-12)
			require.InDelta(t, 3.75*ratio.MilliTokensUsd, resolved.Ratio*resolved.CompletionRatio, 1e-12)
			require.InDelta(t, 0.075*ratio.MilliTokensUsd, resolved.CachedInputRatio, 1e-12)
		})
	}
}
