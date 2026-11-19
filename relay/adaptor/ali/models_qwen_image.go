package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenImageModels returns independently owned qwen image defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenImageModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen-image": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03125,
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
			Description:      "DashScope qwen-image: Qwen Image standard tier (¥0.25/image).",
		},
		"qwen-image-2.0": {
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
			Description:      "DashScope qwen-image-2.0: Qwen Image 2.0 standard tier (¥0.20/image).",
		},
		"qwen-image-2.0-pro": {
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
					"1024x1536": 1,
					"1280x720":  1,
					"1536x1024": 1,
					"720x1280":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-2.0-pro: canonical 2.0 high-fidelity tier with 4K support (¥0.50/image).",
		},
		"qwen-image-edit": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0375,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-edit: Qwen Image editing baseline tier (¥0.30/image).",
		},
		"qwen-image-edit-max": {
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
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-edit-max: high-fidelity image editing tier (¥0.50/image).",
		},
		"qwen-image-edit-plus": {
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
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-edit-plus: image editing tier (¥0.20/image).",
		},
		"qwen-image-max": {
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
					"1024x1536": 1,
					"1280x720":  1,
					"1536x1024": 1,
					"720x1280":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-max: top-tier Qwen Image text-to-image model (¥0.50/image).",
		},
		"qwen-image-plus": {
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
			Description:      "DashScope qwen-image-plus: cost-effective Qwen Image text-to-image tier (¥0.20/image).",
		},
		"qwen-image-pro": {
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
					"1024x1536": 1,
					"1280x720":  1,
					"1536x1024": 1,
					"720x1280":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope qwen-image-pro: Qwen Image 2.0 high-fidelity tier (¥0.50/image).",
		},
	}
}
