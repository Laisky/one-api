package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Moonshot AI hosts the Kimi family of long-context chat models. Production
// chat APIs are OpenAI-compatible and advertise tool calling, JSON mode and
// structured outputs. Reference docs:
//   - https://platform.kimi.com/docs/intro
//   - https://platform.kimi.com/docs/models   (canonical current model list)
//   - https://platform.kimi.com/docs/pricing  (chat-k27-code / chat-k26 / chat-k25 / chat-v1 sub-pages)
//   - https://platform.moonshot.cn/docs/pricing (legacy, 301 → platform.kimi.com)
//
// Current multimodal flagships are Kimi K2.7 Code, K2.6 and K2.5; all three are
// open-weight 1.1T MoE models on HuggingFace (moonshotai/Kimi-K2.7-Code,
// moonshotai/Kimi-K2.6, moonshotai/Kimi-K2.5) and accept text + image + video
// input. The legacy moonshot-v1 SKUs remain closed-weight.
//
// The entire kimi-k2 series (kimi-k2-0905-preview, kimi-k2-0711-preview,
// kimi-k2-turbo-preview, kimi-k2-thinking, kimi-k2-thinking-turbo) was
// discontinued on 2026-05-25 (per platform.kimi.com/docs/models 已下线模型) and
// is therefore removed here; kimi-latest (2026-01-28) and kimi-thinking-preview
// (2025-11-11) were retired earlier. All prices retrieved 2026-06-12 from
// platform.kimi.com pricing pages (RMB per 1M tokens).

// moonshotTextInputs is the input modality set for text-only Kimi chat models.
var moonshotTextInputs = []string{"text"}

// moonshotVisionInputs is the modality set for Kimi vision-capable models.
var moonshotVisionInputs = []string{"text", "image"}

// moonshotMultimodalInputs is the modality set for the Kimi K2.5/K2.6/K2.7 Code
// flagships (native text + image + video understanding).
var moonshotMultimodalInputs = []string{"text", "image", "video"}

// moonshotTextOutputs is the output modality set used by all Kimi chat models.
var moonshotTextOutputs = []string{"text"}

// moonshotChatFeatures captures tool-calling / JSON mode / structured outputs
// advertised by the Moonshot OpenAI-compatible API.
var moonshotChatFeatures = []string{"tools", "json_mode", "structured_outputs"}

// moonshotReasoningFeatures extends the chat feature set with the reasoning
// flag for thinking-only variants (e.g. Kimi K2.7 Code) that emit a
// chain-of-thought channel. Web search is intentionally excluded: the platform
// $web_search builtin is incompatible with always-on thinking.
var moonshotReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

// moonshotMultimodalFeatures advertises the full capability set of the
// thinking/non-thinking multimodal flagships (Kimi K2.5, K2.6): tool calling,
// JSON mode, structured outputs, reasoning channel and web search.
var moonshotMultimodalFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"}

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
// Kimi thinking endpoints reject.
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
	// Kimi K2.7 Code (2026-06, current top coding model). Multimodal text +
	// image + video, thinking-only deep reasoning, automatic context caching,
	// tool calls, JSON mode and Partial mode. Open weights on HuggingFace.
	"kimi-k2.7-code": {
		Ratio:                       6.5 * ratio.MilliTokensRmb, // input (cache-miss)
		CompletionRatio:             27.0 / 6.5,                 // output / input
		CachedInputRatio:            1.3 * ratio.MilliTokensRmb, // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotMultimodalInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens:          49152,
		HuggingFaceID:               "moonshotai/Kimi-K2.7-Code",
		Description:                 "Moonshot Kimi K2.7 Code, current top coding model (1.1T MoE, open weights); text + image + video input, 256k ctx, thinking-only deep reasoning.",
	},

	// Kimi K2.7 Code HighSpeed (2026-06). High-throughput (~180 tok/s) variant
	// of K2.7 Code; same multimodal text + image + video input, thinking-only
	// deep reasoning and context caching, priced at 2x standard K2.7 Code.
	"kimi-k2.7-code-highspeed": {
		Ratio:                       13.0 * ratio.MilliTokensRmb, // input (cache-miss)
		CompletionRatio:             54.0 / 13.0,                 // output / input
		CachedInputRatio:            2.6 * ratio.MilliTokensRmb,  // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotMultimodalInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotReasoningFeatures,
		SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens:          49152,
		HuggingFaceID:               "moonshotai/Kimi-K2.7-Code",
		Description:                 "Moonshot Kimi K2.7 Code HighSpeed, high-throughput (~180 tok/s) variant of K2.7 Code; text + image + video input, 256k ctx, thinking-only deep reasoning. Priced at 2x standard K2.7 Code.",
	},

	// Kimi K2.6 (2026-05). Multimodal text + image + video, supports thinking
	// and non-thinking modes, automatic context caching, tool calls, JSON
	// mode and web search. Open weights on HuggingFace.
	"kimi-k2.6": {
		Ratio:                       6.5 * ratio.MilliTokensRmb, // input (cache-miss)
		CompletionRatio:             27.0 / 6.5,                 // output / input
		CachedInputRatio:            1.1 * ratio.MilliTokensRmb, // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotMultimodalInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotMultimodalFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		MaxReasoningTokens:          49152,
		HuggingFaceID:               "moonshotai/Kimi-K2.6",
		Description:                 "Moonshot Kimi K2.6 multimodal flagship (text + image + video, 256k ctx) with thinking and non-thinking modes; open weights on HuggingFace.",
	},

	// Kimi K2.5 (2026-04). Multimodal predecessor of K2.6: text + image +
	// video, thinking and non-thinking modes, tool calls, JSON mode and web
	// search. Open weights on HuggingFace.
	"kimi-k2.5": {
		Ratio:                       4.0 * ratio.MilliTokensRmb, // input (cache-miss)
		CompletionRatio:             21.0 / 4.0,                 // output / input
		CachedInputRatio:            0.7 * ratio.MilliTokensRmb, // input (cache-hit)
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             moonshotMultimodalInputs,
		OutputModalities:            moonshotTextOutputs,
		SupportedFeatures:           moonshotMultimodalFeatures,
		SupportedSamplingParameters: moonshotSamplingParams,
		MaxReasoningTokens:          49152,
		HuggingFaceID:               "moonshotai/Kimi-K2.5",
		Description:                 "Moonshot Kimi K2.5 multimodal model (text + image + video, 256k ctx) with thinking and non-thinking modes; open weights on HuggingFace.",
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

// MoonshotToolingDefaults notes that Moonshot's pricing pages list model fees only; no tool metering is published (retrieved 2026-06-12).
// Source: https://platform.kimi.com/docs/pricing
var MoonshotToolingDefaults = adaptor.ChannelToolConfig{}
