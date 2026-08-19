package cloudflare

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddingModelsDeclareEmbeddingCapability verifies every Cloudflare
// embedding model is typed explicitly, even when its identifier does not
// contain the word "embedding" (for example, bge-m3).
func TestEmbeddingModelsDeclareEmbeddingCapability(t *testing.T) {
	t.Parallel()

	for _, modelName := range cloudflareEmbeddingModelNames {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()

			cfg, ok := ModelRatios[modelName]
			require.True(t, ok, "%s missing from Cloudflare pricing metadata", modelName)
			require.NotNil(t, cfg.Embedding, "%s must be classified as an embedding model", modelName)
			require.InDelta(t, cfg.Ratio, cfg.Embedding.TextTokenRatio, 1e-12)
		})
	}
}
