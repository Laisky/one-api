package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// wanAiModels returns independently owned wan ai defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func wanAiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"Wan-AI/Wan2.6-Image-Edit": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Wan 2.6 image generation and editing model.",
		},
		"Wan-AI/Wan2.6-T2I": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "Wan 2.6 text-to-image model.",
		},
		"Wan-AI/Wan2.7-Image-Edit": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Wan 2.7 image generation and editing model.",
		},
	}
}
