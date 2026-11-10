package aws

import (
	"testing"

	"github.com/stretchr/testify/require"

	awsclaude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestClaudeFable52BedrockCatalog checks effective pricing, model ID
// translation, and the parent AWS dispatcher for both new models.
func TestClaudeFable52BedrockCatalog(t *testing.T) {
	t.Parallel()
	pricing := (&Adaptor{}).GetDefaultModelPricing()
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, ok := pricing[name]
			require.True(t, ok)
			require.Contains(t, (&Adaptor{}).GetModelList(), name)
			require.NotNil(t, GetAdaptor(name))
			require.True(t, IsClaudeModel(name))
			modelID, err := awsclaude.AwsModelID(name)
			require.NoError(t, err)
			require.Equal(t, "anthropic."+name, modelID)
			require.InDelta(t, 8*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 5, cfg.CompletionRatio, 1e-12)
			require.InDelta(t, .2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, 16*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
			require.EqualValues(t, 1000000, cfg.ContextLength)
			require.EqualValues(t, 128000, cfg.MaxOutputTokens)
			require.Equal(t, "high", cfg.DefaultReasoningEffort)
		})
	}
}
