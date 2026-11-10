package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestClaudeFable52ModelCatalog checks discovery, billing buckets, limits,
// adaptive thinking, and preservation of older models' prices.
func TestClaudeFable52ModelCatalog(t *testing.T) {
	t.Parallel()
	models := (&Adaptor{}).GetModelList()
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[name]
			require.True(t, ok)
			require.Contains(t, models, name)
			require.InDelta(t, 8*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 5, cfg.CompletionRatio, 1e-12)
			require.InDelta(t, 40*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
			require.InDelta(t, .2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, 16*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
			require.EqualValues(t, 1000000, cfg.ContextLength)
			require.EqualValues(t, 128000, cfg.MaxOutputTokens)
			require.Equal(t, claudeAdaptiveOnlySamplingParams, cfg.SupportedSamplingParameters)
			require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "high", cfg.DefaultReasoningEffort)
			require.True(t, IsClaudeAdaptiveThinkingModel(name))
			require.Empty(t, cfg.TimeWindows)
		})
	}
	require.NotContains(t, models, "claude-fable-5-2-latest")
	require.NotContains(t, models, "claude-mythos-5-2-20261109")
	require.InDelta(t, 10*ratio.MilliTokensUsd, ModelRatios["claude-fable-5-1"].Ratio, 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, ModelRatios["claude-mythos-5-1"].Ratio, 1e-12)
}
