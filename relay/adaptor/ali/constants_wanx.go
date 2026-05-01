package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// wanxModelRatios captures pricing and metadata for Alibaba's image generation
// models exposed via DashScope (the wanx family, plus the legacy Stable Diffusion
// XL / 1.5 SaaS endpoints). These are pure text-to-image models: input is text
// prompts and output is rendered images. SupportedFeatures and
// SupportedSamplingParameters are empty because the image-generation endpoint
// does not accept chat-style sampling controls.
//
// Pricing rows are immutable; the metadata below only adds context-length /
// modality / description hints used by capability surfaces.
var wanxModelRatios = map[string]adaptor.ModelConfig{
	"ali-stable-diffusion-xl": {
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
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
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
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
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
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
}
