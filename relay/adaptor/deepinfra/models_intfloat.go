package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// intfloatModels returns independently owned intfloat defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func intfloatModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"intfloat/e5-base-v2": {
			Ratio:           nativeRate(0.005),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.005),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "intfloat/e5-base-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"intfloat/e5-large-v2": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "intfloat/e5-large-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"intfloat/multilingual-e5-large": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "intfloat/multilingual-e5-large on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"intfloat/multilingual-e5-large-instruct": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    512,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "intfloat/multilingual-e5-large-instruct on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
