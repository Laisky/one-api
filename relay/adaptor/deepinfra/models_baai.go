package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// baaiModels returns the baai model defaults for deepinfra.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func baaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"BAAI/bge-base-en-v1.5": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "BAAI/bge-base-en-v1.5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"BAAI/bge-en-icl": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "BAAI/bge-en-icl on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"BAAI/bge-large-en-v1.5": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "BAAI/bge-large-en-v1.5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"BAAI/bge-m3": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "BAAI/bge-m3 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"BAAI/bge-m3-multi": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "BAAI/bge-m3-multi on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"BAAI/bge-m3-multi-8k": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength: 8192,
			Description:   "BAAI/bge-m3-multi-8k on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
