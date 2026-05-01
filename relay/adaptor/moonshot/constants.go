package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Moonshot AI hosts the Kimi family of long-context chat models. Production
// chat APIs are OpenAI-compatible and advertise tool calling, JSON mode and
// structured outputs. Reference docs:
//   - https://platform.moonshot.cn/docs/intro
//   - https://platform.moonshot.cn/docs/pricing
//
// The Kimi-K2-Instruct-0905 weights are open on HuggingFace; the legacy
// moonshot-v1 SKUs and the *-thinking / *-turbo variants remain closed weights.

// moonshotTextInputs is the input modality set for text-only Kimi chat models.
var moonshotTextInputs = []string{"text"}

// moonshotTextOutputs is the output modality set used by all Kimi chat models.
var moonshotTextOutputs = []string{"text"}

// moonshotChatFeatures captures tool-calling / JSON mode / structured outputs
// advertised by the Moonshot OpenAI-compatible API.
var moonshotChatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// moonshotReasoningFeatures extends the chat feature set with the reasoning
// flag for Kimi-K2-Thinking variants that emit a chain-of-thought channel.
var moonshotReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// moonshotSamplingParams lists OpenAI-compatible sampling controls supported
// by Moonshot chat completions.
var moonshotSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// moonshotReasoningSamplingParams omits frequency / presence penalties that
// Kimi-K2 thinking endpoints reject.
var moonshotReasoningSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Moonshot pricing: https://platform.moonshot.cn/docs/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Moonshot legacy models (keep for compatibility)
	// "moonshot-v1-8k":   {Ratio: 12 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// "moonshot-v1-32k":  {Ratio: 24 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// "moonshot-v1-128k": {Ratio: 60 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Kimi-K2 models (2025-11)
	// All prices per 1M tokens, in RMB
	// input: cache-hit, input: cache-miss, output, context
	"kimi-k2-0905-preview": {
		Ratio:                       4 * ratio.MilliTokensRmb,  // input (cache-miss)
		CompletionRatio:             16 * ratio.MilliTokensRmb, // output
		CachedInputRatio:            1 * ratio.MilliTokensRmb,  // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "moonshotai/Kimi-K2-Instruct-0905",
		Description:                 "Moonshot Kimi-K2 0905 preview, 262k context MoE chat model with open weights on HuggingFace.",
	},
	"kimi-k2-0711-preview": {
		Ratio:                       4 * ratio.MilliTokensRmb,
		CompletionRatio:             16 * ratio.MilliTokensRmb,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               131072,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "moonshotai/Kimi-K2-Instruct",
		Description:                 "Moonshot Kimi-K2 0711 preview, 128k context MoE chat model with open weights on HuggingFace.",
	},
	"kimi-k2-turbo-preview": {
		Ratio:                       8 * ratio.MilliTokensRmb,
		CompletionRatio:             58 * ratio.MilliTokensRmb,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot Kimi-K2 turbo preview, faster latency tier of the Kimi-K2 family (closed-weight serving).",
	},
	"kimi-k2-thinking": {
		Ratio:                       4 * ratio.MilliTokensRmb,
		CompletionRatio:             16 * ratio.MilliTokensRmb,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		Description:                 "Moonshot Kimi-K2 thinking variant exposing an explicit reasoning channel.",
	},
	"kimi-k2-thinking-turbo": {
		Ratio:                       8 * ratio.MilliTokensRmb,
		CompletionRatio:             58 * ratio.MilliTokensRmb,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		Description:                 "Moonshot Kimi-K2 thinking turbo, lower-latency reasoning tier.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MoonshotToolingDefaults notes that Moonshot's pricing page lists model fees only; no tool metering is published (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://platform.moonshot.cn/docs/pricing
var MoonshotToolingDefaults = adaptor.ChannelToolConfig{}
