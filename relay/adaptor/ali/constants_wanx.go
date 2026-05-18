package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// wanxModelRatios captures pricing and metadata for Alibaba's image generation
// models exposed via DashScope: the Wanx / Wan2.x family, the Qwen-Image tier,
// and the legacy Stable Diffusion XL / 1.5 SaaS endpoints. These are
// text-to-image (some support image input) models: input is text prompts
// (optionally an image), output is rendered images.
//
// Pricing is expressed via the Image config's PricePerImageUsd. Aliyun publishes
// per-image prices in CNY which we convert to USD using the project's
// 8 RMB/USD reference rate (matches ratio.ExchangeRateRmb). Where Aliyun lists
// CNY 0.145 the encoded USD price is 0.145 / 8 = 0.01813.
//
// Sources verified 2026-05-18:
//   - https://www.alibabacloud.com/help/en/model-studio/model-pricing
//   - https://www.alibabacloud.com/help/en/model-studio/image-model
//   - https://help.aliyun.com/zh/model-studio/wan-image-generation
var wanxModelRatios = map[string]adaptor.ModelConfig{
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
				"512x1024":  1,
				"1024x768":  1,
				"1024x1024": 1,
				"576x1024":  1,
				"1024x576":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "Alibaba-hosted Stable Diffusion XL text-to-image endpoint.",
	},
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
				"512x1024":  1,
				"1024x768":  1,
				"1024x1024": 1,
				"576x1024":  1,
				"1024x576":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "Alibaba-hosted Stable Diffusion v1.5 text-to-image endpoint.",
	},
	"wanx-v1": {
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
				"720x1280":  1,
				"1280x720":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wanx-v1: Alibaba's first-generation Wanx text-to-image model.",
	},
	"wanx2.1-t2i-turbo": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.145/image; ¥0.145 / 8 RMB-per-USD = $0.01813.
			PricePerImageUsd: 0.01813,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
				"768x1152":  1,
				"1152x768":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wanx2.1-t2i-turbo: cost-optimized Wan 2.1 text-to-image tier (¥0.145/image).",
	},
	"wanx2.1-t2i-plus": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.20/image for wanx2.1-t2i-plus (estimated; verify against
			// console). ¥0.20 / 8 = $0.025. Marked unverified — see commit notes.
			PricePerImageUsd: 0.025,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
				"768x1152":  1,
				"1152x768":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wanx2.1-t2i-plus: higher-fidelity Wan 2.1 text-to-image tier (~¥0.20/image).",
	},
	"qwen-image-plus": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.206/image; ¥0.206 / 8 = $0.02575.
			PricePerImageUsd: 0.02575,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
				"768x1152":  1,
				"1152x768":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope qwen-image-plus: cost-effective Qwen Image text-to-image tier (¥0.206/image).",
	},
	"qwen-image": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.206/image for qwen-image-2.0 standard tier;
			// ¥0.206 / 8 = $0.02575.
			PricePerImageUsd: 0.02575,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope qwen-image: Qwen Image 2.0 standard tier (¥0.206/image).",
	},
	"qwen-image-pro": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.515/image for qwen-image-2.0-pro;
			// ¥0.515 / 8 = $0.06438.
			PricePerImageUsd: 0.06438,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "hd",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
				"1024x1536": 1,
				"1536x1024": 1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope qwen-image-pro: Qwen Image 2.0 high-fidelity tier (¥0.515/image).",
	},
	"qwen-image-edit-plus": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.206/image for qwen-image-edit-plus.
			PricePerImageUsd: 0.02575,
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
		Description:      "DashScope qwen-image-edit-plus: image editing tier (¥0.206/image).",
	},
	"qwen-image-edit-max": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.515/image for qwen-image-edit-max.
			PricePerImageUsd: 0.06438,
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
		Description:      "DashScope qwen-image-edit-max: high-fidelity image editing tier (¥0.515/image).",
	},
}
