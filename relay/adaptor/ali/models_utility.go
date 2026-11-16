package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// utilityModels returns independently owned utility defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func utilityModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"tongyi-intent-detect-v3": {
			Ratio:           nativeRate(0.4),
			CompletionRatio: 2.5,
			Description:     "tongyi-intent-detect-v3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"tongyi-xiaomi-analysis-flash": {
			Ratio:           nativeRate(0.2),
			CompletionRatio: 2,
			Description:     "tongyi-xiaomi-analysis-flash on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"tongyi-xiaomi-analysis-pro": {
			Ratio:           nativeRate(1),
			CompletionRatio: 2.7,
			Description:     "tongyi-xiaomi-analysis-pro on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"unisound/unisound-u2": {
			Ratio:           nativeRate(1),
			CompletionRatio: 2,
			Description:     "unisound/unisound-u2 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
