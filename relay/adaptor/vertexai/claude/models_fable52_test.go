package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
)

// TestClaudeFable52VertexCatalog checks discovery, standard-price parity,
// output limits and the Vertex-specific feature subset.
func TestClaudeFable52VertexCatalog(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, ok := ModelRatios[name]
			require.True(t, ok)
			require.Contains(t, ModelList, name)
			native := anthropic.ModelRatios[name]
			require.Equal(t, native.Ratio, cfg.Ratio)
			require.Equal(t, native.CompletionRatio, cfg.CompletionRatio)
			require.Equal(t, native.CachedInputRatio, cfg.CachedInputRatio)
			require.Equal(t, native.CacheWrite5mRatio, cfg.CacheWrite5mRatio)
			require.Equal(t, native.CacheWrite1hRatio, cfg.CacheWrite1hRatio)
			require.Equal(t, native.ContextLength, cfg.ContextLength)
			require.Equal(t, native.MaxOutputTokens, cfg.MaxOutputTokens)
			require.Equal(t, native.SupportedReasoningEfforts, cfg.SupportedReasoningEfforts)
			require.Equal(t, []string{"tools", "reasoning"}, cfg.SupportedFeatures)
		})
	}
}
