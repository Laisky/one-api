package siliconflow

import "github.com/Laisky/one-api/relay/adaptor"

// proModels returns the pro model defaults for siliconflow.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func proModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"Pro/BAAI/bge-m3": {
			Ratio:           nativeRate(0.07),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.07),
			},
			Description: "Pro/BAAI/bge-m3 on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Pro/BAAI/bge-reranker-v2-m3": {
			Ratio:           nativeRate(0.07),
			CompletionRatio: 0,
			Description:     "Pro/BAAI/bge-reranker-v2-m3 on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Pro/deepseek-ai/DeepSeek-V3.1-Terminus": {
			Ratio:            nativeRate(4),
			CompletionRatio:  3,
			CachedInputRatio: nativeRate(0.4),
			Description:      "Pro/deepseek-ai/DeepSeek-V3.1-Terminus on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Pro/deepseek-ai/DeepSeek-V3.2": {
			Ratio:            nativeRate(4),
			CompletionRatio:  1.5,
			CachedInputRatio: nativeRate(0.4),
			Description:      "Pro/deepseek-ai/DeepSeek-V3.2 on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Pro/moonshotai/Kimi-K2.6": {
			Ratio:            nativeRate(6.5),
			CompletionRatio:  4.153846153846154,
			CachedInputRatio: nativeRate(1.1),
			Description:      "Pro/moonshotai/Kimi-K2.6 on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Pro/zai-org/GLM-5.1": {
			Ratio:            nativeRate(6),
			CompletionRatio:  4,
			CachedInputRatio: nativeRate(1.3),
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(8),
				CompletionRatio:     3.5,
				CachedInputRatio:    nativeRate(2),
				InputTokenThreshold: 32000,
			},
			},
			Description: "Pro/zai-org/GLM-5.1 on siliconflow; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
