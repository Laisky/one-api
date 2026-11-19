package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenOpenWeightsModels returns independently owned qwen open weights defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenOpenWeightsModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen3.5-122b-a10b": {
			Ratio:           nativeRate(0.8),
			CompletionRatio: 8,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(2),
				CompletionRatio:     8,
				InputTokenThreshold: 128001,
			},
			},
			Description: "qwen3.5-122b-a10b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.5-27b": {
			Ratio:           nativeRate(0.6),
			CompletionRatio: 8,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(1.8),
				CompletionRatio:     8,
				InputTokenThreshold: 128001,
			},
			},
			Description: "qwen3.5-27b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.5-35b-a3b": {
			Ratio:           nativeRate(0.4),
			CompletionRatio: 8,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(1.6),
				CompletionRatio:     8,
				InputTokenThreshold: 128001,
			},
			},
			Description: "qwen3.5-35b-a3b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.5-397b-a17b": {
			Ratio:           nativeRate(1.2),
			CompletionRatio: 6,
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(3),
				CompletionRatio:     6,
				InputTokenThreshold: 128001,
			},
			},
			Description: "qwen3.5-397b-a17b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.6-27b": {
			Ratio:           nativeRate(3),
			CompletionRatio: 6,
			Description:     "qwen3.6-27b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.6-35b-a3b": {
			Ratio:           nativeRate(1.8),
			CompletionRatio: 6,
			Description:     "qwen3.6-35b-a3b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.8-2.4t-a95b": {
			Ratio:           nativeRate(12),
			CompletionRatio: 3,
			Description:     "qwen3.8-2.4t-a95b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"qwen3.8-27b": {
			Ratio:           nativeRate(3),
			CompletionRatio: 4,
			Description:     "qwen3.8-27b on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
