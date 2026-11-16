package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// blackForestLabsModels returns independently owned black forest labs defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func blackForestLabsModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"black-forest-labs/FLUX-1.1-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 1.1 Pro text-to-image model.",
		},
		"black-forest-labs/FLUX-2-klein-4b": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.014,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"512x512":   0.25,
					"768x768":   0.5625,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX.2 Klein 4B text-to-image and image editing model.",
		},
		"black-forest-labs/FLUX-2-klein-9b": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.015,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"512x512":   0.25,
					"768x768":   0.5625,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX.2 Klein 9B text-to-image and image editing model.",
		},
		"black-forest-labs/FLUX-2-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 2 Max image generation and editing model.",
		},
		"black-forest-labs/FLUX-2-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.015,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 2 Pro image generation and editing model.",
		},
	}
}
