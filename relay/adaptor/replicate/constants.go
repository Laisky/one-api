package replicate

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
)

// Shared metadata helpers for Replicate-hosted models. Replicate hosts a mix of
// open-weight LLMs, image generators, video generators and audio models. The
// metadata below is intentionally additive — pricing fields remain immutable,
// and missing context-length / sampling info is left zero so callers fall back
// to OpenRouter's defaults.

// imageInputs lists input modalities for text-to-image generators.
var imageInputs = []string{"text"}

// imageEditInputs lists input modalities for img2img / inpainting / variation
// generators that accept a reference image alongside the prompt.
var imageEditInputs = []string{"text", "image"}

// imageOutputs declares image-only output modality.
var imageOutputs = []string{"image"}

// textOnlyInputs is the input modality set for text-only chat models.
var textOnlyInputs = []string{"text"}

// textOnlyOutputs is the output modality set for chat models.
var textOnlyOutputs = []string{"text"}

// visionTextInputs marks vision-capable chat models that accept text and image.
var visionTextInputs = []string{"text", "image"}

// chatFeatures is the baseline feature set for OpenAI-compatible chat models
// hosted on Replicate (tool calling + JSON / structured outputs).
var chatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// reasoningFeatures advertises tools, JSON mode, structured outputs and
// reasoning for thinking-style models (DeepSeek-R1, GPT-5, Grok-4, Kimi-K2, …).
var reasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// basicChatFeatures covers older open-weight chat models that do not natively
// support tool calling or JSON mode (Llama 2, Mistral 7B, …).
var basicChatFeatures = []string{}

// commonSamplingParams enumerates sampling parameters most chat models accept.
var commonSamplingParams = []string{
	"temperature",
	"top_p",
	"top_k",
	"stop",
	"max_tokens",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// reasoningSamplingParams trims sampling controls that GPT-5 / o-style
// reasoning endpoints typically reject (frequency / presence / top_k).
var reasoningSamplingParams = []string{
	"temperature",
	"top_p",
	"stop",
	"max_tokens",
	"seed",
}

// replicateImageConfig builds an ImagePricingConfig with the canonical single-image
// minimum used by every Replicate text-to-image model. Signature preserved for
// backward compatibility.
func replicateImageConfig(pricePerImage float64) *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: pricePerImage,
		MinImages:        1,
	}
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Based on Replicate pricing and explicit official model pages retrieved 2026-04-28.
// Replicate collection pages are treated additively for discovery; absence from a collection is not used for removals.
//
// The map is assembled in init() from family-specific maps declared in
// constants_image.go and constants_language.go to keep individual files under
// the 600-line project guideline.
//
// Sources consulted:
//   - https://replicate.com/explore
//   - Per-model pages under https://replicate.com/<owner>/<model>
//   - HuggingFace cards for open-weight models (referenced via HuggingFaceID)
//
// Image / video / audio generators leave ContextLength and MaxOutputTokens as 0
// (token concept does not apply to their prompt limits). LLMs hosted on
// Replicate fill in full chat metadata (context, sampling, features, HF id).
var ModelRatios = map[string]adaptor.ModelConfig{}

// ModelList derived from ModelRatios for backward compatibility.
// Populated in init() after ModelRatios is assembled.
var ModelList []string

// ReplicateToolingDefaults notes that Replicate bills per model runtime without separate tool pricing (retrieved 2026-04-28).
// Source: https://replicate.com/pricing
var ReplicateToolingDefaults = adaptor.ChannelToolConfig{}

// init assembles ModelRatios from per-family maps and derives ModelList. The
// assembly is deterministic across runs because Go map iteration in
// adaptor.GetModelListFromPricing already produces stable identity (callers do
// not assume ordering).
func init() {
	maps.Copy(ModelRatios, replicateImageModelRatios)
	maps.Copy(ModelRatios, replicateLanguageModelRatios)
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
