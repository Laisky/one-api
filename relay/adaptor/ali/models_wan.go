package ali

import "github.com/Laisky/one-api/relay/adaptor"

// wanModels returns the wan model defaults for ali.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func wanModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"wan2.2-t2i-flash": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0175,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1152x768":  1,
					"1280x720":  1,
					"720x1280":  1,
					"768x1152":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.2-t2i-flash: Wan 2.2 cost-optimized text-to-image (¥0.14/image).",
		},
		"wan2.2-t2i-plus": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.025,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "hd",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1152x768":  1,
					"1280x720":  1,
					"720x1280":  1,
					"768x1152":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.2-t2i-plus: Wan 2.2 high-fidelity text-to-image (¥0.20/image).",
		},
		"wan2.5-t2i-preview": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.025,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1280x720":  1,
					"720x1280":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.5-t2i-preview: Wan 2.5 preview text-to-image (¥0.20/image).",
		},
		"wan2.6-image": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.025,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"2048x2048": 1,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.6-image: Wan 2.6 text/image-to-image tier (¥0.20/image).",
		},
		"wan2.6-t2i": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.025,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1152x768":  1,
					"1280x720":  1,
					"720x1280":  1,
					"768x1152":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.6-t2i: Wan 2.6 text-to-image (¥0.20/image).",
		},
		"wan2.7-image": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.025,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"2048x2048": 1,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.7-image: Wan 2.7 fast text/image-to-image tier (¥0.20/image).",
		},
		"wan2.7-image-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0625,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "hd",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"2048x2048": 1,
					"2160x3840": 1,
					"3840x2160": 1,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope wan2.7-image-pro: Wan 2.7 professional text/image-to-image tier with 4K support (¥0.50/image).",
		},
	}
}
