package cloudflare

import "github.com/Laisky/one-api/relay/adaptor"

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

// init marks Cloudflare embedding models with explicit capability and pricing metadata.
// Parameters: none.
// Returns: no values.
func init() {
	for _, modelName := range cloudflareEmbeddingModelNames {
		cfg, ok := ModelRatios[modelName]
		if !ok {
			continue
		}
		cfg.Embedding = &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: cfg.Ratio,
		}
		ModelRatios[modelName] = cfg
	}
}
