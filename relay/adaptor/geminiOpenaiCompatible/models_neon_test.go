package geminiOpenaiCompatible_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiNeonProviderTariffs checks independently published Developer and
// Vertex Standard tariffs, synchronous limits, and catalog isolation. Parameter t asserts.
func TestGeminiNeonProviderTariffs(t *testing.T) {
	t.Parallel()
	const name = geminiOpenaiCompatible.Gemini4NeonModel
	for _, cfg := range []adaptor.ModelConfig{
		geminiOpenaiCompatible.ModelRatios[name], vertexai.VertexGoogleModelConfigs()[name],
	} {
		require.InDelta(t, 3, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, 15, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, .15, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.Zero(t, cfg.CacheWrite5mRatio, "Vertex writes start January 12, 2027; do not charge them early")
		require.Zero(t, cfg.CacheWrite1hRatio)
		require.Empty(t, cfg.TimeWindows, "no introductory token-price discount is published")
		require.Empty(t, cfg.Tiers)
		require.EqualValues(t, 1_048_576, cfg.ContextLength)
		require.Equal(t, []string{"low", "medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
		require.Equal(t, "high", cfg.DefaultReasoningEffort)
		require.Equal(t, []string{"max_tokens"}, cfg.SupportedSamplingParameters)
	}
	require.EqualValues(t, 65_536, geminiOpenaiCompatible.ModelRatios[name].MaxOutputTokens,
		"Developer continuation is not a single foreground response")
	require.EqualValues(t, 32_768, vertexai.VertexGoogleModelConfigs()[name].MaxOutputTokens,
		"Vertex paid-failure usage above 32K is not implemented")
	vertex := vertexai.VertexGoogleModelConfigs()[name]
	vertex.SupportedReasoningEfforts[0] = "mutated"
	require.Equal(t, "low", vertexai.VertexGoogleModelConfigs()[name].SupportedReasoningEfforts[0])
	require.Equal(t, "low", geminiOpenaiCompatible.ModelRatios[name].SupportedReasoningEfforts[0])
	for _, unsupported := range []string{"gemini-4-neon-preview", "gemini-4-neon-2026-11-18"} {
		require.NotContains(t, geminiOpenaiCompatible.ModelList, unsupported)
	}
}
