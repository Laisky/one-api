package replicate

import "github.com/Laisky/one-api/relay/adaptor"

// replicateOfficialImageAdditions contains fixed-price Replicate Official image
// endpoints that are absent from the original catalog snapshot. Models whose
// current multi-property billing cannot be represented by ImagePricingConfig are
// intentionally excluded rather than assigned an estimated price.
var replicateOfficialImageAdditions = map[string]adaptor.ModelConfig{
	"black-forest-labs/flux-kontext-max": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Kontext [max] premium text-guided image editing model with improved typography.",
	},
	"bytedance/seedream-5-pro": {
		// Replicate bills 1K outputs at $0.045 and 2K outputs at $0.09.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.09,
			DefaultSize:      "2K",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"1K": 0.5,
				"2K": 1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 5.0 Pro image generation and editing model with 1K and 2K output tiers.",
	},
	"google/nano-banana": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.039),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "Google Nano Banana (Gemini 2.5 Flash Image) generation and conversational editing model.",
	},
	"google/nano-banana-2-lite": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.034,
			DefaultSize:      "1K",
			MinImages:        1,
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "Google Nano Banana 2 Lite fast 1K image generation and editing model with multi-image references.",
	},
	"openai/gpt-image-1.5": {
		// Replicate exposes quality tiers: auto/high $0.136, medium $0.05, low $0.013.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.136,
			DefaultQuality:   "auto",
			MinImages:        1,
			QualityMultipliers: map[string]float64{
				"low":    0.013 / 0.136,
				"medium": 0.05 / 0.136,
				"high":   1.0,
				"auto":   1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "OpenAI GPT Image 1.5 generation and editing model with low, medium, and high quality tiers.",
	},
	"recraft-ai/recraft-v4": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "1024x1024",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4 standard-resolution image generator focused on prompt accuracy and design quality.",
	},
	"recraft-ai/recraft-v4-pro": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.25,
			DefaultSize:      "2048x2048",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4 Pro high-resolution image generator for print-ready and large-format output.",
	},
	"recraft-ai/recraft-v4.1": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "1024x1024",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4.1 standard-resolution image generator with art-directed composition and text rendering.",
	},
	"recraft-ai/recraft-v4.1-pro": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.25,
			DefaultSize:      "2048x2048",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4.1 Pro high-resolution image generator for production-quality design assets.",
	},
	"recraft-ai/recraft-v4.1-utility": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "1024x1024",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4.1 Utility throughput-optimized standard-resolution image generator.",
	},
	"recraft-ai/recraft-v4.1-utility-pro": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.25,
			DefaultSize:      "2048x2048",
			MinImages:        1,
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft V4.1 Utility Pro throughput-optimized high-resolution image generator.",
	},
	"wan-video/wan-2.7-image": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "Wan 2.7 image generation and editing model with multi-reference and coherent image-set support.",
	},
	"wan-video/wan-2.7-image-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "Wan 2.7 Image Pro generation and editing model with output up to 4K for text-to-image requests.",
	},
	"xai/grok-imagine-image": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image fast image generation and editing model with strong text rendering.",
	},
	"xai/grok-imagine-image-quality": {
		// Replicate bills 1K outputs at $0.05 and 2K outputs at $0.07. Editing
		// adds $0.01 per input image, which the current metadata schema cannot encode.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "2k",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"1k": 0.05 / 0.07,
				"2k": 1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image Quality high-fidelity generation and editing model with 1K and 2K tiers.",
	},
	"xai/grok-imagine-image-2": {
		// Replicate bills $0.04 per output image and an additional $0.01 per input
		// image for edits; the input-image surcharge is not representable here.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "2k",
			DefaultQuality:   "medium",
			MinImages:        1,
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image 2.0 generation and editing model with selectable quality and output up to 2K.",
	},
}
