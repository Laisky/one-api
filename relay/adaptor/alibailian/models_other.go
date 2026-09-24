package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// otherModels returns the other model defaults for alibailian.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func otherModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"MiniMax-M2.1": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax-M2.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax-M2.5": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax-M2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.1": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.5": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M2.7": {
			Ratio:           nativeRate(2.1),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M2.7 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"MiniMax/MiniMax-M3": {
			Ratio:           nativeRate(4.2),
			CompletionRatio: 4,
			Description:     "MiniMax/MiniMax-M3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"Moonshot-Kimi-K2-Instruct": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "Moonshot-Kimi-K2-Instruct on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2-thinking": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "kimi-k2-thinking on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi-k2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 4.153846153846154,
			Description:     "kimi-k2.6 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 4.153846153846154,
			Description:     "kimi-k2.7-code on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi-k3": {
			Ratio:           nativeRate(2e+01),
			CompletionRatio: 5,
			Description:     "kimi-k3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 5.25,
			Description:     "kimi/kimi-k2.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.6": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 4.153846153846154,
			Description:     "kimi/kimi-k2.6 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code": {
			Ratio:           nativeRate(6.5),
			CompletionRatio: 4.153846153846154,
			Description:     "kimi/kimi-k2.7-code on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k2.7-code-highspeed": {
			Ratio:           nativeRate(13),
			CompletionRatio: 4.153846153846154,
			Description:     "kimi/kimi-k2.7-code-highspeed on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"kimi/kimi-k3": {
			Ratio:                       nativeRate(2e+01),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(2),
			ContextLength:               1048576,
			MaxOutputTokens:             1048576,
			InputModalities:             []string{"text", "image", "video"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"max_tokens", "stop"},
			SupportedReasoningEfforts:   []string{"max"},
			DefaultReasoningEffort:      "max",
			Description:                 "kimi/kimi-k3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qvq-max": {
			Ratio:                       nativeRate(8),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(1.6480000000000001),
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"seed", "max_tokens"},
			MaxReasoningTokens:          38912,
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/QVQ-72B-Preview",
			Description:                 "qvq-max on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qvq-plus": {
			Ratio:           nativeRate(2),
			CompletionRatio: 2.5,
			Description:     "qvq-plus on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwq-32b-preview": {
			Ratio:                       nativeRate(2.06),
			CompletionRatio:             2.995,
			CachedInputRatio:            nativeRate(0.41200000000000003),
			ContextLength:               32768,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"seed", "max_tokens"},
			MaxReasoningTokens:          38912,
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/QwQ-32B-Preview",
			Description:                 "QwQ-32B-Preview on Bailian: experimental open-weight reasoning model.",
		},
		"qwq-plus": {
			Ratio:                       nativeRate(1.6),
			CompletionRatio:             2.5,
			CachedInputRatio:            0.023571428571428573,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"seed", "max_tokens"},
			MaxReasoningTokens:          38912,
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/QwQ-32B",
			Description:                 "qwq-plus on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"siliconflow/deepseek-r1-0528": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "siliconflow/deepseek-r1-0528 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"siliconflow/deepseek-v3-0324": {
			Ratio:           nativeRate(2),
			CompletionRatio: 4,
			Description:     "siliconflow/deepseek-v3-0324 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"siliconflow/deepseek-v3.1-terminus": {
			Ratio:           nativeRate(4),
			CompletionRatio: 3,
			Description:     "siliconflow/deepseek-v3.1-terminus on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"siliconflow/deepseek-v3.2": {
			Ratio:           nativeRate(2),
			CompletionRatio: 1.5,
			Description:     "siliconflow/deepseek-v3.2 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"stepfun/step-3.7-flash": {
			Ratio:           nativeRate(1.35),
			CompletionRatio: 5.999999999999999,
			Description:     "stepfun/step-3.7-flash on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"stepfun/step-5-preview": {
			Ratio:           nativeRate(7),
			CompletionRatio: 2.857142857142857,
			Description:     "stepfun/step-5-preview on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
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
		"tongyi-intent-detect-v3": {
			Ratio:           nativeRate(0.4),
			CompletionRatio: 2.5,
			Description:     "tongyi-intent-detect-v3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"tongyi-xiaomi-analysis-flash": {
			Ratio:           nativeRate(0.2),
			CompletionRatio: 2,
			Description:     "tongyi-xiaomi-analysis-flash on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"tongyi-xiaomi-analysis-pro": {
			Ratio:           nativeRate(1),
			CompletionRatio: 2.7,
			Description:     "tongyi-xiaomi-analysis-pro on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"unisound/unisound-u2": {
			Ratio:           nativeRate(1),
			CompletionRatio: 2,
			Description:     "unisound/unisound-u2 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"xiaomi/mimo-v2.5-pro": {
			Ratio:           nativeRate(7),
			CompletionRatio: 3,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(14),
				CompletionRatio:     3,
				InputTokenThreshold: 256001,
			},
			},
			Description: "xiaomi/mimo-v2.5-pro on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
