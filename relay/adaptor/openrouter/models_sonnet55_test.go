package openrouter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestOpenRouterSonnet55Catalog checks OpenRouter's public ID, independent rates,
// and reasoning/parameter metadata. It takes a test handle and returns nothing.
func TestOpenRouterSonnet55Catalog(t *testing.T) {
	t.Parallel()
	const id = "anthropic/claude-sonnet-5.5"
	a := &Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.InDelta(t, 2*ratio.MilliTokensUsd, a.GetModelRatio(id), 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.Ratio*a.GetCompletionRatio(id), 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
	require.InDelta(t, 2.5*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 4*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"text", "image", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.ElementsMatch(t, []string{"max", "xhigh", "high", "medium", "low"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "high", cfg.DefaultReasoningEffort)
	require.Contains(t, cfg.SupportedSamplingParameters, "temperature", "OpenRouter's contract differs from the native Anthropic wire contract")
	for _, excluded := range []string{"claude-sonnet-5-5", "anthropic/claude-sonnet-5-5", id + "-20260928", id + ":batch"} {
		require.NotContains(t, a.GetModelList(), excluded)
	}
}

// TestOpenRouterSonnet55DefinitionsOwnMetadata verifies that constructing model
// definitions cannot mutate another catalog instance. It takes a test handle
// and returns nothing without touching shared production state.
func TestOpenRouterSonnet55DefinitionsOwnMetadata(t *testing.T) {
	t.Parallel()
	const id = "anthropic/claude-sonnet-5.5"
	first, second := sonnet55Models(), sonnet55Models()
	first[id].InputModalities[0] = "changed"
	first[id].SupportedSamplingParameters[0] = "changed"
	first[id].SupportedReasoningEfforts[0] = "changed"
	require.Equal(t, "text", second[id].InputModalities[0])
	require.Equal(t, "include_reasoning", second[id].SupportedSamplingParameters[0])
	require.Equal(t, "max", second[id].SupportedReasoningEfforts[0])
}
