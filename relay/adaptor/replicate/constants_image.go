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
