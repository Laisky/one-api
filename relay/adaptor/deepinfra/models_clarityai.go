package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// clarityaiModels returns independently owned clarityai defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func clarityaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ClarityAI/creative": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "ClarityAI creative image upscaler.",
		},
		"ClarityAI/crystal": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "ClarityAI portrait and product upscaler.",
		},
		"ClarityAI/flux": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.2,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "ClarityAI FLUX-powered image upscaler.",
		},
	}
}
