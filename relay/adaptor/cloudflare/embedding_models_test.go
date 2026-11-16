package cloudflare

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddingModelsDeclareEmbeddingCapability verifies every Cloudflare
// embedding model is typed explicitly, even when its identifier does not
// contain the word "embedding" (for example, bge-m3).
// Parameters: t is the test handle used to run assertions and subtests.
// Returns: no values.
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

// cloudflareEmbeddingModelNames lists Workers AI models that accept text as
// input but return embedding vectors rather than generated chat text. Keeping
// this type metadata explicit prevents generic channel-test selection from
// treating text-input embedding models as chat models.
var cloudflareEmbeddingModelNames = []string{
	"@cf/baai/bge-small-en-v1.5",
	"@cf/baai/bge-base-en-v1.5",
	"@cf/baai/bge-large-en-v1.5",
	"@cf/baai/bge-m3",
	"@cf/pfnet/plamo-embedding-1b",
	"@cf/qwen/qwen3-embedding-0.6b",
}
