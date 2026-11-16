package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// imageModels returns independently owned image defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func imageModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ali-stable-diffusion-v1.5": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.02,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x576":  1,
					"1024x768":  1,
					"512x1024":  1,
					"576x1024":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "Alibaba-hosted Stable Diffusion v1.5 text-to-image endpoint.",
		},
		"ali-stable-diffusion-xl": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.02,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x576":  1,
					"1024x768":  1,
					"512x1024":  1,
					"576x1024":  1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "Alibaba-hosted Stable Diffusion XL text-to-image endpoint.",
		},
		"z-image-turbo": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.015,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1536x1536": 1,
					"2048x2048": 1,
				},
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "DashScope z-image-turbo: lightweight fast text-to-image model, fixed 1 image/request, up to 2048x2048 ($0.015/image, $0.03 with prompt_extend).",
		},
	}
}
