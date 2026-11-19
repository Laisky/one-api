package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// glmModels returns independently owned glm defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// Additional model and cache sources checked 2026-11-16:
// https://help.aliyun.com/zh/model-studio/context-cache
// https://help.aliyun.com/zh/model-studio/qwen-api-via-dashscope
func glmModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ZHIPU/GLM-5": {
			Ratio:           nativeRate(6),
			CompletionRatio: 22.0 / 6.0,
			Description:     "ZHIPU/GLM-5 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.1": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.1 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.2": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.2 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.3": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.3-Flash": {
			Ratio:           nativeRate(0.8),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.3-Flash on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.3-FlashX": {
			Ratio:           nativeRate(2),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.3-FlashX on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.5": {
			Ratio:           nativeRate(3),
			CompletionRatio: 14.0 / 3.0,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.5 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.5-air": {
			Ratio:           nativeRate(0.8),
			CompletionRatio: 7.5,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(1.2),
				CompletionRatio:     8.0 / 1.2,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.5-air on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.6": {
			Ratio:           nativeRate(3),
			CompletionRatio: 14.0 / 3.0,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.6 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.7": {
			Ratio:           nativeRate(3),
			CompletionRatio: 14.0 / 3.0,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.7 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4.5,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(6),
				CompletionRatio:     22.0 / 6.0,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-5 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.1": {
			Ratio:           nativeRate(6),
			CompletionRatio: 4,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(8),
				CompletionRatio:     3.5,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-5.1 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.2": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "glm-5.2 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.2-fast-preview": {
			Ratio:           nativeRate(16),
			CompletionRatio: 3.5,
			Description:     "glm-5.2-fast-preview on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.3": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "glm-5.3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.3-prime": {
			Ratio: nativeRate(16), CompletionRatio: 3.5, CachedInputRatio: nativeRate(4),
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			Description: "Alibaba-deployed GLM 5.3 Prime, Beijing CNY 16 input, 4 cached input and 56 output per million tokens. Other deployment regions have different prices and cache fractions. Provider-specific limits and control vocabularies remain unspecified.",
		},
	}
}
