package replicate

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// replicateImageModelRatios enumerates the text-to-image and image-edit models
// hosted on Replicate. ContextLength / MaxOutputTokens are intentionally zero
// because prompt limits for these models are not measured in tokens.
//
// https://replicate.com/collections/text-to-image
var replicateImageModelRatios = map[string]adaptor.ModelConfig{
	"black-forest-labs/flux-kontext-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.1 Kontext [pro] image editing model that takes a prompt plus a reference image.",
	},
	"black-forest-labs/flux-1.1-pro": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.04),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "FLUX 1.1 [pro] high-quality text-to-image generator from Black Forest Labs.",
	},
	"black-forest-labs/flux-2-dev": {
		// FLUX.2 [dev] is the open-weight distilled variant priced at $0.012/MP for both
		// text-to-image and image editing. The base config assumes a single 1024x1024 (1MP)
		// output. Confirm at https://bfl.ai/pricing for higher-resolution requests.
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.012),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		HuggingFaceID: "black-forest-labs/FLUX.2-dev",
		Description:   "FLUX.2 [dev] open-weight 32B distilled rectified-flow text-to-image and editing model.",
	},
	"black-forest-labs/flux-2-max": {
		// FLUX.2 [max] is BFL's highest-quality FLUX.2 tier listed on Replicate at
		// roughly $0.25/image (25 credits/image with 1 credit = 1 cent). Confirm at
		// https://bfl.ai/pricing for current rates.
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.25),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [max] highest-quality FLUX.2 tier with strongest prompt following and up to 8 reference images.",
	},
	"black-forest-labs/flux-2-pro": {
		// Black Forest Labs publishes FLUX.2 [pro] at $0.03/megapixel for text-to-image
		// (image editing is $0.045/megapixel). Most Replicate requests default to a
		// single 1024x1024 (1MP) image, which lines up with $0.03/image. Confirm at
		// https://bfl.ai/pricing for higher-resolution requests.
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageEditInputs, OutputModalities: imageOutputs,
		Description: "FLUX.2 [pro] high-resolution text-to-image and multi-reference editing model.",
	},
	"black-forest-labs/flux-1.1-pro-ultra": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.06),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "FLUX 1.1 [pro] Ultra mode for higher fidelity and 4MP output.",
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
	"ideogram-ai/ideogram-v3-turbo": {
		Ratio: 0, CompletionRatio: 1.0, Image: replicateImageConfig(0.03),
		InputModalities: imageInputs, OutputModalities: imageOutputs,
		Description: "Ideogram v3 Turbo fastest text-to-image variant in the v3 family.",
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
}
