package replicate

import "github.com/Laisky/one-api/relay/adaptor"

// replicateOfficialImageAdditions contains Replicate Official image endpoints
// whose deterministic output pricing can be represented by ImagePricingConfig.
// Models or request modes with unrepresentable provider charges are intentionally
// excluded rather than assigned an estimated or underpriced configuration.
var replicateOfficialImageAdditions = map[string]adaptor.ModelConfig{
	"black-forest-labs/flux-kontext-max": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Kontext [max] premium text-guided image editing model with improved typography.",
	},
	"bytedance/seedream-5-pro": {
		// Replicate also offers a discounted 1K tier, but the current relay does
		// not forward Seedream's model-specific size field. Expose only the 2K
		// upstream default so every accepted request is billed at provider cost.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.09,
			DefaultSize:      "2K",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"2k": 1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 5.0 Pro image generation and editing model using the upstream-default 2K output tier.",
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
			SizeMultipliers: map[string]float64{
				"1k": 1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "Google Nano Banana 2 Lite fast 1K image generation and editing model with multi-image references.",
	},
	"openai/gpt-image-1.5": {
		// Replicate also offers discounted low and medium tiers, but the current
		// relay does not forward the quality selector. Accept only auto/high,
		// which share the $0.136 output price, to prevent underbilling.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.136,
			DefaultQuality:   "auto",
			MinImages:        1,
			QualityMultipliers: map[string]float64{
				"auto": 1.0,
				"high": 1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "OpenAI GPT Image 1.5 generation and editing model using the auto/high output price.",
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
		// Replicate also offers a discounted 1K tier and charges $0.01 per input
		// image for edits. The current relay forwards neither the resolution field
		// nor the edit surcharge, so expose only 2K text-to-image generation.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "2k",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"2k": 1.0,
			},
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image Quality high-fidelity text-to-image model using the upstream-default 2K tier.",
	},
	"xai/grok-imagine-image-2": {
		// Replicate charges $0.01 per input image for edits. The current pricing
		// schema cannot represent that surcharge, so expose generation only and
		// restrict metadata to the upstream-default 2K/medium request settings.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "2k",
			DefaultQuality:   "medium",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"2k": 1.0,
			},
			QualityMultipliers: map[string]float64{
				"medium": 1.0,
			},
		},
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image 2.0 text-to-image model using the upstream-default 2K medium-quality settings.",
	},
}
