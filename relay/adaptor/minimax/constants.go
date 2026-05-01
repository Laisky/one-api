package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// MiniMax operates the abab and MiniMax-Text / VL families. Reference docs:
//   - https://platform.minimaxi.com/document/Models (model overview)
//   - https://api.minimax.chat/document/price (pricing)
//
// abab* and the MiniMax-Text-01 / MiniMax-VL-01 production endpoints are
// closed-weight. The MiniMax-M1 reasoning model is open-weight on HuggingFace
// (MiniMaxAI/MiniMax-M1-80k); when surfaced via this adaptor we record the HF
// id and quantization. MiniMax-VL-01 accepts text + image inputs.

// minimaxTextInputs is the input modality set for text-only MiniMax models.
var minimaxTextInputs = []string{"text"}

// minimaxVisionInputs is the modality set for MiniMax-VL multimodal models.
var minimaxVisionInputs = []string{"text", "image"}

// minimaxTextOutputs is the text-only output modality used by chat APIs.
var minimaxTextOutputs = []string{"text"}

// minimaxChatFeatures advertises tool / JSON capabilities exposed by the
// MiniMax chat API for production abab and MiniMax-Text/VL endpoints.
var minimaxChatFeatures = []string{"tools", "json_mode"}

// minimaxSamplingParams enumerates sampling parameters accepted by MiniMax
// chat completions.
var minimaxSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Minimax pricing: https://api.minimax.chat/document/price
var ModelRatios = map[string]adaptor.ModelConfig{
	// Minimax Models - Based on https://api.minimax.chat/document/price
	"abab6.5-chat": {
		Ratio:                       0.03 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               245760,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6.5 long-context closed-weight chat model.",
	},
	"abab6.5s-chat": {
		Ratio:                       0.01 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               245760,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6.5s cost-optimized variant of abab6.5.",
	},
	"abab6-chat": {
		Ratio:                       0.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab6 closed-weight chat model with 32k context.",
	},
	"abab5.5-chat": {
		Ratio:                       0.015 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab5.5 closed-weight chat model.",
	},
	"abab5.5s-chat": {
		Ratio:                       0.005 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax abab5.5s cost-optimized chat model.",
	},
	"MiniMax-VL-01": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxVisionInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Description:                 "MiniMax-VL-01 multimodal chat model accepting text and image inputs (estimated pricing).",
	},
	"MiniMax-Text-01": {
		Ratio:                       0.015 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             minimaxTextInputs,
		OutputModalities:            minimaxTextOutputs,
		SupportedFeatures:           minimaxChatFeatures,
		SupportedSamplingParameters: minimaxSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "MiniMaxAI/MiniMax-Text-01",
		Description:                 "MiniMax-Text-01 long-context chat model with open weights on HuggingFace (estimated pricing).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MinimaxToolingDefaults notes that MiniMax's pricing reference lists model rates only (no tool pricing) as of 2025-11-12.
// Source: https://r.jina.ai/https://api.minimax.chat/document/price
var MinimaxToolingDefaults = adaptor.ChannelToolConfig{}
