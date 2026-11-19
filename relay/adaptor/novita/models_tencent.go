package novita

import "github.com/Laisky/one-api/relay/adaptor"

// tencentModels returns independently owned tencent defaults for novita.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.novita.ai/v3/openai/models
func tencentModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"tencent/hy3": {
			Ratio:             nativeRate(0.14),
			CompletionRatio:   0.58 / 0.14,
			CachedInputRatio:  nativeRate(0.035),
			ContextLength:     262144,
			MaxOutputTokens:   262144,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "tencent/hy3 on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"tencent/hy4": {
			Ratio:             nativeRate(0.417),
			CompletionRatio:   2.99880095923261,
			CachedInputRatio:  nativeRate(0.021),
			ContextLength:     1000000,
			MaxOutputTokens:   64000,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "tencent/hy4 on Novita; exact USD decimal tariff and metadata checked 2026-11-16. No creator tariff, priority surcharge, or unpublished reasoning-effort ladder is inferred.",
		},
		"tencent/hy4-flash": {
			Ratio:             nativeRate(0.07),
			CompletionRatio:   4.01428571428571,
			CachedInputRatio:  nativeRate(0.003),
			ContextLength:     1000000,
			MaxOutputTokens:   64000,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "tencent/hy4-flash on Novita; exact USD decimal tariff and metadata checked 2026-11-16. No creator tariff, priority surcharge, or unpublished reasoning-effort ladder is inferred.",
		},
		"tencent/hy4-preview": {
			Ratio:             nativeRate(0.834),
			CompletionRatio:   2.501 / 0.834,
			CachedInputRatio:  nativeRate(0.042),
			ContextLength:     1000000,
			MaxOutputTokens:   64000,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "tencent/hy4-preview on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
