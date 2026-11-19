package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// sentenceTransformersModels returns independently owned sentence transformers defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func sentenceTransformersModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"sentence-transformers/all-MiniLM-L12-v2": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/all-MiniLM-L12-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/all-MiniLM-L6-v2": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/all-MiniLM-L6-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/all-mpnet-base-v2": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/all-mpnet-base-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/clip-ViT-B-32": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    77,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/clip-ViT-B-32 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/clip-ViT-B-32-multilingual-v1": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/clip-ViT-B-32-multilingual-v1 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/multi-qa-mpnet-base-dot-v1": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/multi-qa-mpnet-base-dot-v1 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sentence-transformers/paraphrase-MiniLM-L6-v2": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "sentence-transformers/paraphrase-MiniLM-L6-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
