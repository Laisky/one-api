package siliconflow

import "github.com/Laisky/one-api/relay/adaptor"

// glm53FlashModels returns the separately priced SiliconFlow GLM 5.3 Flash
// configuration. It takes no arguments and returns a newly allocated catalogue.
// Source verified 2026-10-12: https://www.siliconflow.cn/pricing
// The authenticated model card was unavailable, so deployment limits, non-text
// modalities, hosted precision, and advanced controls remain unspecified.
func glm53FlashModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"zai-org/GLM-5.3-Flash": {
			Ratio:            nativeRate(0.80),
			CompletionRatio:  2.80 / 0.80,
			CachedInputRatio: nativeRate(0.23),
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
			Description:      "GLM 5.3 Flash hosted by SiliconFlow. Published CNY per million tokens: 0.80 input, 0.23 cached input, 2.80 output. This is a paid model, not GLM 4.7 Flash's free offer or Z.AI's USD tariff. Text chat uses the existing compatible endpoint; deployment limits and additional capabilities were not independently verified from the authenticated SiliconFlow model card. No provider-specific reasoning-effort controls are advertised.",
		},
	}
}
