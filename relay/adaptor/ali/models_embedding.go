package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// embeddingModels returns independently owned embedding defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func embeddingModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"text-embedding-async-v1": {
			Ratio:           nativeRate(0.7000000000000001),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.7000000000000001),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "DashScope text-embedding-async-v1: batched asynchronous embedding endpoint.",
		},
		"text-embedding-async-v2": {
			Ratio:           nativeRate(0.7000000000000001),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.7000000000000001),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "DashScope text-embedding-async-v2: batched asynchronous multilingual embedding endpoint.",
		},
		"text-embedding-v1": {
			Ratio:           nativeRate(0.7000000000000001),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.7000000000000001),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "DashScope text-embedding-v1: legacy embedding endpoint, 8192-token input.",
		},
		"text-embedding-v2": {
			Ratio:           nativeRate(0.7000000000000001),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.7000000000000001),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "DashScope text-embedding-v2: improved multilingual embedding endpoint, 8192-token input.",
		},
		"text-embedding-v3": {
			Ratio:           nativeRate(0.5),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.5),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text"},
			OutputModalities: []string{},
			Description:      "DashScope text-embedding-v3: 1024-dim embedding endpoint, 8192-token input, 50+ languages.",
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
			Description:      "DashScope text-embedding-v4 (Qwen3-Embedding): flexible 64-2048 dim output, 8192-token input, 100+ languages.",
		},
		"tongyi-embedding-vision-plus": {
			Ratio:           nativeRate(0.5),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.5),
			},
			ContextLength:    8192,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{},
			Description:      "DashScope tongyi-embedding-vision-plus: multimodal (text + image) embedding endpoint.",
		},
	}
}
