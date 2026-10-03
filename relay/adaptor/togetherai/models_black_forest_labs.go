package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// black_forest_labsModels returns the black_forest_labs model defaults for togetherai.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func black_forest_labsModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"black-forest-labs/FLUX.1-kontext-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.08388608,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.1-kontext-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.1-krea-dev": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0262144,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
			Description: "FLUX.1 Krea Dev image model. (RETIRED 2026-05-27)",
		},
		"black-forest-labs/FLUX.1-schnell": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0028311552,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.1.1-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.2-dev": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0161480704,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.2-flex": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03145728,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.2-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07340032,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"black-forest-labs/FLUX.2-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03,
				DefaultSize:      "1024x1024",
				MinImages:        1,
			},
		},
	}
}
