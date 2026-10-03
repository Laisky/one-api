package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// glmModels returns the glm model defaults for alibailian.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func glmModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"glm-4.5": {
			Ratio:           nativeRate(3),
			CompletionRatio: 4.666666666666667,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.5-air": {
			Ratio:           nativeRate(0.8),
			CompletionRatio: 7.5,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(1.2),
				CompletionRatio:     6.666666666666667,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.5-air on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.6": {
			Ratio:           nativeRate(3),
			CompletionRatio: 4.666666666666667,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.6 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-4.7": {
			Ratio:           nativeRate(3),
			CompletionRatio: 4.666666666666667,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(4),
				CompletionRatio:     4,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-4.7 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4.5,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(6),
				CompletionRatio:     3.6666666666666665,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-5 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.1": {
			Ratio:           nativeRate(6),
			CompletionRatio: 4,
			Tiers: []adaptor.ModelRatioTier{adaptor.ModelRatioTier{
				Ratio:               nativeRate(8),
				CompletionRatio:     3.5,
				InputTokenThreshold: 32001,
			},
			},
			Description: "glm-5.1 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.2": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "glm-5.2 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.2-fast-preview": {
			Ratio:           nativeRate(16),
			CompletionRatio: 3.5,
			Description:     "glm-5.2-fast-preview on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"glm-5.3": {
			Ratio:           nativeRate(8),
			CompletionRatio: 3.5,
			Description:     "glm-5.3 on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
