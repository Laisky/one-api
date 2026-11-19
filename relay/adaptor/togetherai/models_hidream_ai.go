package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// hidreamAiModels returns independently owned hidream ai defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func hidreamAiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"HiDream-ai/HiDream-I1-Dev": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.004718592,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.0045),
			},
		},
		"HiDream-ai/HiDream-I1-Fast": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0033554432,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.0032),
			},
		},
		"HiDream-ai/HiDream-I1-Full": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.009437184,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.009),
			},
		},
	}
}
