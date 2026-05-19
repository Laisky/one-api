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
// Sources verified 2026-05-19:
//   - https://www.alibabacloud.com/help/en/model-studio/model-pricing
//   - https://www.alibabacloud.com/help/en/model-studio/image-model
//   - https://help.aliyun.com/zh/model-studio/wan-image-generation
//   - https://help.aliyun.com/zh/model-studio/wan-image-generation-and-editing-api-reference
//
// Mainland-China CNY/image pricing tables (Beijing, verified 2026-05-19):
//
//	Wan image+edit:    wan2.7-image-pro 0.50, wan2.7-image 0.20, wan2.6-image 0.20
//	Wan t2i (legacy):  wan2.6-t2i 0.20, wan2.5-t2i-preview 0.20,
//	                   wan2.2-t2i-plus 0.20, wan2.2-t2i-flash 0.14,
//	                   wanx2.1-t2i-plus 0.20, wanx2.1-t2i-turbo 0.14,
//	                   wanx2.0-t2i-turbo 0.04, wanx-v1 0.16
//	Qwen image (txt2img): qwen-image-2.0-pro 0.50, qwen-image-2.0 0.20,
//	                      qwen-image-max 0.50, qwen-image-plus 0.20, qwen-image 0.25
//	Qwen image (edit):    qwen-image-edit-max 0.50, qwen-image-edit-plus 0.20,
//	                      qwen-image-edit 0.30
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
			// Aliyun lists ¥0.20/image (qwen-image alias); ¥0.20 / 8 = $0.025.
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
		Description:      "DashScope qwen-image-plus: cost-effective Qwen Image text-to-image tier (¥0.20/image).",
	},
	"qwen-image": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.25/image for qwen-image standard tier;
			// ¥0.25 / 8 = $0.03125.
			PricePerImageUsd: 0.03125,
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
		Description:      "DashScope qwen-image: Qwen Image standard tier (¥0.25/image).",
	},
	"qwen-image-2.0": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.20/image; ¥0.20 / 8 = $0.025.
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
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope qwen-image-2.0: Qwen Image 2.0 standard tier (¥0.20/image).",
	},
	"qwen-image-pro": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.50/image for qwen-image-2.0-pro;
			// ¥0.50 / 8 = $0.0625.
			PricePerImageUsd: 0.0625,
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
		Description:      "DashScope qwen-image-pro: Qwen Image 2.0 high-fidelity tier (¥0.50/image).",
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
				"720x1280":  1,
				"1280x720":  1,
				"1024x1536": 1,
				"1536x1024": 1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope qwen-image-2.0-pro: canonical 2.0 high-fidelity tier with 4K support (¥0.50/image).",
	},
	"qwen-image-max": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.50/image for qwen-image-max.
			PricePerImageUsd: 0.0625,
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
		Description:      "DashScope qwen-image-max: top-tier Qwen Image text-to-image model (¥0.50/image).",
	},
	"qwen-image-edit-plus": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.20/image for qwen-image-edit-plus.
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
	"qwen-image-edit-max": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.50/image for qwen-image-edit-max.
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
	"qwen-image-edit": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.30/image for qwen-image-edit; ¥0.30 / 8 = $0.0375.
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

	// ----- Wan 2.7 / 2.6 image (combined gen+edit) -----------------------------
	"wan2.7-image-pro": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.50/image for wan2.7-image-pro; ¥0.50 / 8 = $0.0625.
			PricePerImageUsd: 0.0625,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "hd",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"2048x2048": 1,
				"3840x2160": 1,
				"2160x3840": 1,
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wan2.7-image-pro: Wan 2.7 professional text/image-to-image tier with 4K support (¥0.50/image).",
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

	// ----- Wan 2.6 / 2.5 / 2.2 / 2.1 t2i (text-to-image only) ------------------
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
				"720x1280":  1,
				"1280x720":  1,
				"768x1152":  1,
				"1152x768":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wan2.6-t2i: Wan 2.6 text-to-image (¥0.20/image).",
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
				"720x1280":  1,
				"1280x720":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wan2.5-t2i-preview: Wan 2.5 preview text-to-image (¥0.20/image).",
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
				"720x1280":  1,
				"1280x720":  1,
				"768x1152":  1,
				"1152x768":  1,
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DashScope wan2.2-t2i-plus: Wan 2.2 high-fidelity text-to-image (¥0.20/image).",
	},
	"wan2.2-t2i-flash": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.14/image; ¥0.14 / 8 = $0.0175.
			PricePerImageUsd: 0.0175,
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
		Description:      "DashScope wan2.2-t2i-flash: Wan 2.2 cost-optimized text-to-image (¥0.14/image).",
	},
	"wanx2.0-t2i-turbo": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			// Aliyun lists ¥0.04/image; ¥0.04 / 8 = $0.005.
			PricePerImageUsd: 0.005,
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
		Description:      "DashScope wanx2.0-t2i-turbo: legacy Wan 2.0 cost-optimized text-to-image (¥0.04/image).",
	},
}
