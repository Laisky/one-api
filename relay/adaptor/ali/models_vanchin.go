package ali

import "github.com/Laisky/one-api/relay/adaptor"

// vanchinModels returns the vanchin model defaults for ali.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func vanchinModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"vanchin/deepseek-r1": {
			Ratio:           nativeRate(4),
			CompletionRatio: 4,
			Description:     "vanchin/deepseek-r1 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v3": {
			Ratio:           nativeRate(2),
			CompletionRatio: 4,
			Description:     "vanchin/deepseek-v3 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v3.1-terminus": {
			Ratio:           nativeRate(4),
			CompletionRatio: 3,
			Description:     "vanchin/deepseek-v3.1-terminus on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v3.2-think": {
			Ratio:           nativeRate(2),
			CompletionRatio: 1.5,
			Description:     "vanchin/deepseek-v3.2-think on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v4-pro": {
			Ratio:           nativeRate(12),
			CompletionRatio: 2,
			Description:     "vanchin/deepseek-v4-pro on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v4-pro-0813": {
			Ratio:           nativeRate(9),
			CompletionRatio: 3,
			Description:     "vanchin/deepseek-v4-pro-0813 on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"vanchin/deepseek-v4.1-flash": {
			Ratio:           nativeRate(2),
			CompletionRatio: 4,
			Description:     "vanchin/deepseek-v4.1-flash on ali; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
