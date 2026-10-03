package ali

import "github.com/Laisky/one-api/relay/adaptor"

// zhipuModels returns the zhipu model defaults for ali.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func zhipuModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ZHIPU/GLM-5": {
			Ratio:           nativeRate(6),
			CompletionRatio: 3.6666666666666665,
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
			CompletionRatio: 3.4999999999999996,
			Description:     "ZHIPU/GLM-5.3-Flash on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ZHIPU/GLM-5.3-FlashX": {
			Ratio:           nativeRate(2),
			CompletionRatio: 3.5,
			Description:     "ZHIPU/GLM-5.3-FlashX on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
