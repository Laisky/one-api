package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Moonshot AI hosts the Kimi family of long-context chat models. Production
// chat APIs are OpenAI-compatible and advertise tool calling, JSON mode and
// structured outputs. Reference docs:
//   - https://platform.kimi.com/docs/intro
//   - https://platform.kimi.com/docs/pricing (chat / k2 / k2.6 / v1 sub-pages)
//   - https://platform.moonshot.cn/docs/pricing (legacy, 301 → platform.kimi.com)
//
// The Kimi-K2-Instruct-0905 weights are open on HuggingFace; the legacy
// moonshot-v1 SKUs and the *-thinking / *-turbo variants remain closed weights.
// Kimi K2 entries are being phased out (notice: discontinuation 2026-05-25) in
// favor of Kimi K2.6 (kimi-k2.6 and kimi-k2.6-vision). All prices retrieved
// 2026-05-18 from platform.kimi.com pricing pages.

// moonshotTextInputs is the input modality set for text-only Kimi chat models.
var moonshotTextInputs = []string{"text"}

// moonshotVisionInputs is the modality set for Kimi vision-capable models.
var moonshotVisionInputs = []string{"text", "image"}

// moonshotMultimodalInputs is the modality set for Kimi K2.6 (text + image + video).
var moonshotMultimodalInputs = []string{"text", "image", "file"}

// moonshotTextOutputs is the output modality set used by all Kimi chat models.
var moonshotTextOutputs = []string{"text"}

// moonshotChatFeatures captures tool-calling / JSON mode / structured outputs
// advertised by the Moonshot OpenAI-compatible API.
var moonshotChatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// moonshotReasoningFeatures extends the chat feature set with the reasoning
// flag for Kimi-K2-Thinking variants that emit a chain-of-thought channel.
var moonshotReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// moonshotK26Features advertises K2.6's tool calling, JSON mode, structured
// outputs, web search and reasoning channels.
var moonshotK26Features = []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"}

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
// Based on Moonshot pricing: https://platform.kimi.com/docs/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Kimi K2.6 (2026-05, current flagship). Multimodal text + image + video,
	// supports thinking and non-thinking modes, automatic context caching,
	// tool calls, JSON mode and web search.
	"kimi-k2.6": {
		Ratio:                       6.5 * ratio.MilliTokensRmb, // input (cache-miss)
		CompletionRatio:             27.0 / 6.5,                 // output / input
		CachedInputRatio:            1.1 * ratio.MilliTokensRmb, // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotMultimodalInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotK26Features,
		SupportedSamplingParameters: moonshotSamplingParams,
		MaxReasoningTokens:          49152,
		Description:                 "Moonshot Kimi K2.6 multimodal flagship (text + image + video, 262k ctx) with thinking and non-thinking modes.",
	},

	// Kimi-K2 models (2025-11). The Kimi K2 line is scheduled for retirement
	// on 2026-05-25 per the official notice; entries kept until that date.
	// All prices per 1M tokens, in RMB. (cache-hit, cache-miss, output)
	"kimi-k2-0905-preview": {
		Ratio:                       4 * ratio.MilliTokensRmb,  // input (cache-miss)
		CompletionRatio:             16.0 / 4.0,                // output / input
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
		CompletionRatio:             16.0 / 4.0,
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
		CompletionRatio:             58.0 / 8.0,
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
		CompletionRatio:             16.0 / 4.0,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens:          49152,
		Description:                 "Moonshot Kimi-K2 thinking variant exposing an explicit reasoning channel.",
	},
	"kimi-k2-thinking-turbo": {
		Ratio:                       8 * ratio.MilliTokensRmb,
		CompletionRatio:             58.0 / 8.0,
		CachedInputRatio:            1 * ratio.MilliTokensRmb,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens:          49152,
		Description:                 "Moonshot Kimi-K2 thinking turbo, lower-latency reasoning tier.",
	},

	// Moonshot V1 classic chat models (closed-weight). Cache-hit pricing is
	// not published on the V1 page, so CachedInputRatio is omitted.
	"moonshot-v1-8k": {
		Ratio:                       2 * ratio.MilliTokensRmb,
		CompletionRatio:             10.0 / 2.0,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 classic chat model with 8k context.",
	},
	"moonshot-v1-32k": {
		Ratio:                       5 * ratio.MilliTokensRmb,
		CompletionRatio:             20.0 / 5.0,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 classic chat model with 32k context.",
	},
	"moonshot-v1-128k": {
		Ratio:                       10 * ratio.MilliTokensRmb,
		CompletionRatio:             30.0 / 10.0,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotTextInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 classic chat model with 128k context.",
	},

	// Moonshot V1 vision preview variants. Identical pricing to text-only
	// counterparts per the V1 pricing page.
	"moonshot-v1-8k-vision-preview": {
		Ratio:                       2 * ratio.MilliTokensRmb,
		CompletionRatio:             10.0 / 2.0,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotVisionInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 vision-preview chat model with 8k context (text + image inputs).",
	},
	"moonshot-v1-32k-vision-preview": {
		Ratio:                       5 * ratio.MilliTokensRmb,
		CompletionRatio:             20.0 / 5.0,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotVisionInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 vision-preview chat model with 32k context (text + image inputs).",
	},
	"moonshot-v1-128k-vision-preview": {
		Ratio:                       10 * ratio.MilliTokensRmb,
		CompletionRatio:             30.0 / 10.0,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             moonshotVisionInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotChatFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		Description:                 "Moonshot V1 vision-preview chat model with 128k context (text + image inputs).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MoonshotToolingDefaults notes that Moonshot's pricing pages list model fees only; no tool metering is published (retrieved 2026-05-18).
// Source: https://platform.kimi.com/docs/pricing
var MoonshotToolingDefaults = adaptor.ChannelToolConfig{}
