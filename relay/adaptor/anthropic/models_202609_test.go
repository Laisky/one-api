package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

func TestClaudeSeptember2026ModelCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		inputUsd         float64
		outputMultiplier float64
		cacheReadUsd     float64
		cacheWrite5mUsd  float64
		cacheWrite1hUsd  float64
	}{
		{name: "claude-fable-5-1", inputUsd: 10, outputMultiplier: 5, cacheReadUsd: 0.25, cacheWrite5mUsd: 12.5, cacheWrite1hUsd: 20},
		{name: "claude-mythos-5-1", inputUsd: 10, outputMultiplier: 5, cacheReadUsd: 0.25, cacheWrite5mUsd: 12.5, cacheWrite1hUsd: 20},
		{name: "claude-sonnet-5", inputUsd: 2, outputMultiplier: 5, cacheReadUsd: 0.2, cacheWrite5mUsd: 2.5, cacheWrite1hUsd: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config, ok := ModelRatios[tt.name]
			require.True(t, ok)
			require.InDelta(t, tt.inputUsd*ratio.MilliTokensUsd, config.Ratio, 1e-12)
			require.InDelta(t, tt.outputMultiplier, config.CompletionRatio, 1e-12)
			require.InDelta(t, tt.cacheReadUsd*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
			require.InDelta(t, tt.cacheWrite5mUsd*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, tt.cacheWrite1hUsd*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
			require.EqualValues(t, 1000000, config.ContextLength)
			require.EqualValues(t, 128000, config.MaxOutputTokens)
			require.Equal(t, claudeAdaptiveOnlySamplingParams, config.SupportedSamplingParameters)
			require.EqualValues(t, 0, config.MaxReasoningTokens)
		})
	}
}

func TestClaudeSonnet5PermanentPricingHasNoExpiryWindow(t *testing.T) {
	t.Parallel()

	config := ModelRatios["claude-sonnet-5"]
	require.Empty(t, config.TimeWindows)
	require.InDelta(t, 2*ratio.MilliTokensUsd, config.Ratio, 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, config.Ratio*config.CompletionRatio, 1e-12)
}

func TestClaudeSeptember2026ModelsUseAdaptiveThinking(t *testing.T) {
	t.Parallel()

	require.True(t, IsClaudeAdaptiveThinkingModel("claude-fable-5-1"))
	require.True(t, IsClaudeAdaptiveThinkingModel("claude-mythos-5-1"))
}
