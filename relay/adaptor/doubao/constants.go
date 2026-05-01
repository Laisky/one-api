package doubao

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Doubao (ByteDance Volcengine) is a closed-weight Chinese cloud LLM family.
// Public docs:
//   - https://www.volcengine.com/docs/82379/1099455 (model overview)
//   - https://www.volcengine.com/docs/82379/1099320 (pricing)
//
// All Doubao production models are accessed through OpenAI-compatible chat
// completion APIs that support tool calling and JSON mode. Vision-capable
// variants accept image inputs in addition to text. Weights are not published
// on HuggingFace, so HuggingFaceID and Quantization stay empty.

// doubaoTextInputs lists modalities accepted by text-only Doubao chat models.
var doubaoTextInputs = []string{"text"}

// doubaoVisionInputs lists modalities for vision-capable Doubao models.
var doubaoVisionInputs = []string{"text", "image"}

// doubaoTextOutputs declares the text-only output modality used by all chat
// models in the Doubao lineup.
var doubaoTextOutputs = []string{"text"}

// doubaoChatFeatures captures the tool / JSON capabilities advertised by the
// Volcengine Ark API for all Doubao chat models.
var doubaoChatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// doubaoSamplingParams enumerates OpenAI-compatible sampling parameters that
// the Doubao Ark endpoint accepts for chat generation.
var doubaoSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Based on Doubao pricing: https://www.volcengine.com/docs/82379/1099320
var ModelRatios = map[string]adaptor.ModelConfig{
	// Doubao Pro Models
	"Doubao-pro-128k": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 128k closed-weight chat model with tool calling and JSON mode.",
	},
	"Doubao-pro-32k": {
		Ratio:                       0.002 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 32k closed-weight chat model.",
	},
	"Doubao-pro-4k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Pro 4k closed-weight chat model for short prompts.",
	},

	// Doubao Lite Models
	"Doubao-lite-128k": {
		Ratio:                       0.0008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 128k cost-optimized closed-weight chat model.",
	},
	"Doubao-lite-32k": {
		Ratio:                       0.0006 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32000,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 32k cost-optimized closed-weight chat model.",
	},
	"Doubao-lite-4k": {
		Ratio:                       0.0003 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             doubaoTextInputs,
		OutputModalities:            doubaoTextOutputs,
		SupportedFeatures:           doubaoChatFeatures,
		SupportedSamplingParameters: doubaoSamplingParams,
		Description:                 "ByteDance Doubao Lite 4k cost-optimized closed-weight chat model.",
	},

	// Embedding Models
	"Doubao-embedding": {
		Ratio:            0.0002 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    4096,
		InputModalities:  doubaoTextInputs,
		OutputModalities: doubaoTextOutputs,
		Description:      "ByteDance Doubao text embedding model.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// DoubaoToolingDefaults documents that Bytedance's Doubao cloud pricing does not list per-tool fees publicly (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.volcengine.com/docs/82379/1099320
var DoubaoToolingDefaults = adaptor.ChannelToolConfig{}
