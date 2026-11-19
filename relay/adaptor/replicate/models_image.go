package replicate

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// replicateImageModelRatios enumerates the text-to-image and image-edit models
// hosted on Replicate. ContextLength / MaxOutputTokens are intentionally zero
// because prompt limits for these models are not measured in tokens.
//
// https://replicate.com/collections/text-to-image
// Official endpoints with representable output pricing share this catalog.
// Narrow size and quality profiles retain their exact request defaults and tariffs.
var replicateImageModelRatios = map[string]adaptor.ModelConfig{

	// black-forest-labs models.
	"black-forest-labs/flux-1.1-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "FLUX 1.1 [pro] high-quality text-to-image generator from Black Forest Labs.",
	},
	"black-forest-labs/flux-1.1-pro-ultra": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "FLUX 1.1 [pro] Ultra mode for higher fidelity and 4MP output.",
	},
	"black-forest-labs/flux-2-dev": {
		// FLUX.2 [dev] is the open-weight distilled variant. $0.012/MP applies to go_fast=true
		// (default); go_fast=false is $0.014/MP. The base config assumes a single 1024x1024 (1MP)
		// output, so a default 1MP text-to-image = $0.012.
		// https://replicate.com/black-forest-labs/flux-2-dev#pricing
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.012),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.2-dev",
		Description:   "FLUX.2 [dev] open-weight 32B distilled rectified-flow text-to-image and editing model.",
	},
	"black-forest-labs/flux-2-flex": {
		// FLUX.2 [flex] is priced per megapixel on Replicate: $0.06 per input-image MP +
		// $0.06 per output-image MP. This adaptor's helper only encodes a flat per-image
		// price, so we bill the 1MP-equivalent ($0.06), matching a default single 1024x1024
		// (1MP) text-to-image. NOTE: high-resolution outputs and multi-image edits (which add
		// per-input-MP charges) UNDERCHARGE here because per-MP pricing is not representable.
		// https://replicate.com/black-forest-labs/flux-2-flex
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [flex] tunable-quality text-to-image and multi-reference editing model (per-megapixel priced upstream).",
	},
	"black-forest-labs/flux-2-klein-4b": {
		// FLUX.2 [klein 4B] is priced per megapixel on Replicate: $1 per 1,000 input-image MP +
		// $1 per 1,000 output-image MP (i.e. $0.001/MP each). This adaptor's helper only encodes a
		// flat per-image price, so we bill the 1MP-equivalent ($0.001), matching a default single
		// 1024x1024 (1MP) text-to-image. NOTE: high-resolution outputs and multi-image edits
		// UNDERCHARGE here because per-MP pricing is not representable.
		// https://replicate.com/black-forest-labs/flux-2-klein-4b
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.001),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [klein 4B] cheapest open-weight FLUX.2 text-to-image and editing tier (per-megapixel priced upstream).",
	},
	"black-forest-labs/flux-2-max": {
		// FLUX.2 [max] is priced at $0.04/run + $0.03 per output-image MP (+$0.03 per input-image
		// MP for edits) on Replicate; ~$0.07 for a default 1MP text-to-image.
		// https://replicate.com/black-forest-labs/flux-2-max#pricing
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.07),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [max] highest-quality FLUX.2 tier with strongest prompt following and up to 8 reference images.",
	},
	"black-forest-labs/flux-2-pro": {
		// Replicate prices FLUX.2 [pro] at $0.015/run + $0.015 per output-image MP (+$0.015 per
		// input-image MP for edits). A default single 1024x1024 (1MP) text-to-image is
		// $0.015+$0.015 = $0.03, matching the value below.
		// https://replicate.com/black-forest-labs/flux-2-pro#pricing
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [pro] high-resolution text-to-image and multi-reference editing model.",
	},
	"black-forest-labs/flux-canny-dev": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-Canny-dev",
		Description:   "FLUX.1 Canny [dev] open-weight ControlNet that conditions generation on Canny edges.",
	},
	"black-forest-labs/flux-canny-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Canny [pro] managed ControlNet conditioned on Canny edges.",
	},
	"black-forest-labs/flux-depth-dev": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-Depth-dev",
		Description:   "FLUX.1 Depth [dev] open-weight ControlNet conditioned on depth maps.",
	},
	"black-forest-labs/flux-depth-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Depth [pro] managed ControlNet conditioned on depth maps.",
	},
	"black-forest-labs/flux-dev": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-dev",
		Description:   "FLUX.1 [dev] 12B open-weight rectified-flow text-to-image model.",
	},
	"black-forest-labs/flux-dev-lora": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.032),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-dev",
		Description:   "FLUX.1 [dev] with custom LoRA weights merged at inference time.",
	},
	"black-forest-labs/flux-fill-dev": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-Fill-dev",
		Description:   "FLUX.1 Fill [dev] open-weight inpainting and outpainting model.",
	},
	"black-forest-labs/flux-fill-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Fill [pro] managed inpainting and outpainting service.",
	},
	"black-forest-labs/flux-kontext-max": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Kontext [max] premium text-guided image editing model with improved typography.",
	},
	"black-forest-labs/flux-kontext-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Kontext [pro] image editing model that takes a prompt plus a reference image.",
	},
	"black-forest-labs/flux-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 [pro] flagship managed text-to-image generator.",
	},
	"black-forest-labs/flux-redux-dev": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-Redux-dev",
		Description:   "FLUX.1 Redux [dev] image-variation adapter conditioned on a reference image.",
	},
	"black-forest-labs/flux-redux-schnell": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.003),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Redux [schnell] fast image-variation pipeline.",
	},
	"black-forest-labs/flux-schnell": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.003),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-schnell",
		Description:   "FLUX.1 [schnell] 12B open-weight 1-4 step text-to-image model under Apache 2.0.",
	},
	"black-forest-labs/flux-schnell-lora": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.1-schnell",
		Description:   "FLUX.1 [schnell] with custom LoRA weights merged at inference time.",
	},

	// bytedance models.
	"bytedance/dreamina-3.1": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Dreamina 3.1 text-to-image generator.",
	},
	"bytedance/seedream-3": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 3 text-to-image generator.",
	},
	"bytedance/seedream-4": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 4 next-gen text-to-image generator.",
	},
	"bytedance/seedream-4.5": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 4.5 text-to-image generator with improved fidelity.",
	},
	"bytedance/seedream-5-lite": {
		// $0.035/image flat rate; image-to-image and text-to-image share the same price.
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.035),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "ByteDance Seedream 5.0 Lite image model with built-in reasoning and example-based editing.",
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

	// google models.
	"google/imagen-3": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Google Imagen 3 managed text-to-image model.",
	},
	"google/imagen-3-fast": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.025),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Google Imagen 3 Fast lower-latency text-to-image model.",
	},
	"google/imagen-4": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Google Imagen 4 high-quality managed text-to-image model.",
	},
	"google/imagen-4-fast": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Google Imagen 4 Fast lower-latency text-to-image model.",
	},
	"google/imagen-4-ultra": {
		// Flat $0.06 per output image, text-to-image only.
		// https://replicate.com/google/imagen-4-ultra
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Google Imagen 4 Ultra highest-fidelity tier of the Imagen 4 text-to-image family.",
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

	// ideogram-ai models.
	"ideogram-ai/ideogram-v2": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v2 text-to-image model with strong typography rendering.",
	},
	"ideogram-ai/ideogram-v2-turbo": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.05),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v2 Turbo lower-latency variant of Ideogram v2.",
	},
	"ideogram-ai/ideogram-v3-balanced": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v3 Balanced quality/speed text-to-image variant.",
	},
	"ideogram-ai/ideogram-v3-quality": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.09),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v3 Quality highest-fidelity text-to-image variant.",
	},
	"ideogram-ai/ideogram-v3-turbo": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v3 Turbo fastest text-to-image variant in the v3 family.",
	},

	// openai models.
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
	"openai/gpt-image-2": {
		// Replicate exposes quality tiers; the default "auto" / "high" tier is $0.128/image,
		// medium is $0.047/image, low is $0.012/image. The base config uses the default tier;
		// downstream callers should override when emitting low/medium-quality requests.
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.128,
			MinImages:        1,
			DefaultQuality:   "auto",
			QualityMultipliers: map[string]float64{
				"low":    0.012 / 0.128,
				"medium": 0.047 / 0.128,
				"high":   1.0,
				"auto":   1.0,
			},
		},
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "OpenAI GPT-Image-2 multimodal image generation and editing model.",
	},

	// recraft-ai models.
	"recraft-ai/recraft-v3": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft v3 text-to-image generator with strong vector and design styles.",
	},
	"recraft-ai/recraft-v3-svg": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.08),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Recraft v3 SVG variant that emits vector (SVG) artwork.",
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

	// stability-ai models.
	"stability-ai/stable-diffusion-3": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.035),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Stability AI Stable Diffusion 3 managed text-to-image model.",
	},
	"stability-ai/stable-diffusion-3.5-large": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.065),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "stabilityai/stable-diffusion-3.5-large",
		Description:   "Stable Diffusion 3.5 Large 8B open-weight text-to-image model.",
	},
	"stability-ai/stable-diffusion-3.5-large-turbo": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "stabilityai/stable-diffusion-3.5-large-turbo",
		Description:   "Stable Diffusion 3.5 Large Turbo distilled few-step variant.",
	},
	"stability-ai/stable-diffusion-3.5-medium": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.035),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "stabilityai/stable-diffusion-3.5-medium",
		Description:   "Stable Diffusion 3.5 Medium 2.6B open-weight text-to-image model.",
	},

	// wan-video models.
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

	// xai models.
	"xai/grok-imagine-image": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.02),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "xAI Grok Imagine Image fast image generation and editing model with strong text rendering.",
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
}
