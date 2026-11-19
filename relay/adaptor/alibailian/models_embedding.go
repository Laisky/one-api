package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// embeddingModels returns independently owned embedding defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func embeddingModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"text-embedding-v3": {
			Ratio:           nativeRate(0.5),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.5),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "Bailian text-embedding-v3: 1024-dim embedding endpoint, 8192-token input.",
		},
		"text-embedding-v4": {
			Ratio:           nativeRate(0.5),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.5),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "Bailian text-embedding-v4 (Qwen3-Embedding): flexible 64-2048 dim output, 8192-token input.",
		},
	}
}
