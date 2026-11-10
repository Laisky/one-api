package azure

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestClaudeFable52FoundryCatalog verifies the official Foundry IDs,
// standard rates, and its exclusion of the first-party max effort.
func TestClaudeFable52FoundryCatalog(t *testing.T) {
	t.Parallel()
	pricing := (&Adaptor{}).GetDefaultModelPricing()
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, FoundryClaudeModels, name)
			require.Contains(t, ModelList, name)
			cfg, ok := pricing[name]
			require.True(t, ok)
			require.InDelta(t, 8*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 5, cfg.CompletionRatio, 1e-12)
			require.InDelta(t, .2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, 16*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
			require.Equal(t, []string{"low", "medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
		})
	}
	require.NotContains(t, FoundryClaudeModels, "claude-fable-5-2-fast")
	require.NotContains(t, FoundryClaudeModels, "claude-mythos-5-2-fast")
}
