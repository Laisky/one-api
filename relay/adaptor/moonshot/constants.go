package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Current model IDs and capabilities were checked on 2026-10-12:
// https://platform.kimi.com/docs/models
// https://platform.kimi.com/docs/guide/kimi-k3-quickstart
// https://platform.kimi.com/docs/pricing/chat
//
// Existing CNY tariffs are retained from the July 20 audit. The current pricing
// page's dynamic numeric table was not available to this audit; this is not a
// fresh verification of every amount. In particular K3 now documents separately
// charged 5-minute/1-hour cache writes. Their numeric rates and usage conversion
// need verification before adding defaults; unknown write prices are not free.
// International USD prices are a separate tariff, not an FX conversion.
const moonshotRetiredNotice = " Retired on 2026-08-31; no longer supported by the Kimi API. Use kimi-k3. This entry retains historical pricing for existing configurations, not current upstream availability."

var (
	moonshotTextInputs              = []string{"text"}
	moonshotVisionInputs            = []string{"text", "image"}
	moonshotMultimodalInputs        = []string{"text", "image", "video"}
	moonshotTextOutputs             = []string{"text"}
	moonshotChatFeatures            = []string{"tools", "json_mode", "structured_outputs"}
	moonshotReasoningFeatures       = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	moonshotMultimodalFeatures      = []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"}
	moonshotSamplingParams          = []string{"temperature", "top_p", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"}
	moonshotReasoningSamplingParams = []string{"temperature", "top_p", "max_tokens", "stop", "seed"}
	// K3 fixes temperature/top_p/penalties; they are not effective controls.
	moonshotK3SamplingParams   = []string{"max_tokens", "max_completion_tokens", "stop", "reasoning_effort"}
	moonshotK3ReasoningEfforts = []string{"low", "high", "max"}
)

// ModelRatios contains complete definitions for current and retained legacy IDs.
// No catalogue metadata is used to rewrite a caller's model or impose access caps.
var ModelRatios = map[string]adaptor.ModelConfig{
	"kimi-k3": {
		Ratio: 20.0 * ratio.MilliTokensRmb, CompletionRatio: 100.0 / 20.0,
		CachedInputRatio: 2.0 * ratio.MilliTokensRmb,
		ContextLength:    1048576, MaxOutputTokens: 1048576,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK3SamplingParams,
		SupportedReasoningEfforts: moonshotK3ReasoningEfforts, DefaultReasoningEffort: "max",
		Description: "Kimi K3: 1M context, text/image/video input, always-on low/high/max reasoning (default max). max_completion_tokens defaults to 131072 and can reach 1048576. Images require base64 or ms:// file references, not public URLs. K3 cache writes have separate 5m/1h fees; write prices and usage accounting are not yet represented by these retained input/cache-read/output defaults. Upstream search is being updated and is not recommended for production.",
	},
	"kimi-k2.7-code": {
		Ratio: 6.5 * ratio.MilliTokensRmb, CompletionRatio: 27.0 / 6.5,
		CachedInputRatio: 1.3 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 32768,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens: 49152, HuggingFaceID: "moonshotai/Kimi-K2.7-Code",
		Description: "Moonshot Kimi K2.7 Code, current top coding model (1.1T MoE, open weights); text + image + video input, 256k ctx, thinking-only deep reasoning.",
	},
	"kimi-k2.7-code-highspeed": {
		Ratio: 13.0 * ratio.MilliTokensRmb, CompletionRatio: 54.0 / 13.0,
		CachedInputRatio: 2.6 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 32768,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotReasoningSamplingParams,
		MaxReasoningTokens: 49152, HuggingFaceID: "moonshotai/Kimi-K2.7-Code",
		Description: "Moonshot Kimi K2.7 Code HighSpeed, high-throughput (~180 tok/s) variant of K2.7 Code; text + image + video input, 256k ctx, thinking-only deep reasoning. Priced at 2x standard K2.7 Code.",
	},
	"kimi-k2.6": {
		Ratio: 6.5 * ratio.MilliTokensRmb, CompletionRatio: 27.0 / 6.5,
		CachedInputRatio: 1.1 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 32768,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotMultimodalFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		MaxReasoningTokens: 49152, HuggingFaceID: "moonshotai/Kimi-K2.6",
		Description: "Moonshot Kimi K2.6 multimodal flagship (text + image + video, 256k ctx) with thinking and non-thinking modes; open weights on HuggingFace.",
	},
	"kimi-k2.5": {
		Ratio: 4.0 * ratio.MilliTokensRmb, CompletionRatio: 21.0 / 4.0,
		CachedInputRatio: 0.7 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 32768,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotMultimodalFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		MaxReasoningTokens: 49152, HuggingFaceID: "moonshotai/Kimi-K2.5",
		Description: "Moonshot Kimi K2.5 multimodal model with 256K context." + moonshotRetiredNotice,
	},
	"moonshot-v1-8k": {
		Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 10.0 / 2.0,
		ContextLength: 8192, MaxOutputTokens: 4096,
		InputModalities: moonshotTextInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 classic chat model with 8k context." + moonshotRetiredNotice,
	},
	"moonshot-v1-32k": {
		Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 20.0 / 5.0,
		ContextLength: 32768, MaxOutputTokens: 4096,
		InputModalities: moonshotTextInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 classic chat model with 32k context." + moonshotRetiredNotice,
	},
	"moonshot-v1-128k": {
		Ratio: 10 * ratio.MilliTokensRmb, CompletionRatio: 30.0 / 10.0,
		ContextLength: 131072, MaxOutputTokens: 4096,
		InputModalities: moonshotTextInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 classic chat model with 128k context." + moonshotRetiredNotice,
	},
	"moonshot-v1-8k-vision-preview": {
		Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 10.0 / 2.0,
		ContextLength: 8192, MaxOutputTokens: 4096,
		InputModalities: moonshotVisionInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 vision-preview chat model with 8k context (text + image inputs)." + moonshotRetiredNotice,
	},
	"moonshot-v1-32k-vision-preview": {
		Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 20.0 / 5.0,
		ContextLength: 32768, MaxOutputTokens: 4096,
		InputModalities: moonshotVisionInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 vision-preview chat model with 32k context (text + image inputs)." + moonshotRetiredNotice,
	},
	"moonshot-v1-128k-vision-preview": {
		Ratio: 10 * ratio.MilliTokensRmb, CompletionRatio: 30.0 / 10.0,
		ContextLength: 131072, MaxOutputTokens: 4096,
		InputModalities: moonshotVisionInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotChatFeatures, SupportedSamplingParameters: moonshotSamplingParams,
		Description: "Moonshot V1 vision-preview chat model with 128k context (text + image inputs)." + moonshotRetiredNotice,
	},
}

// ModelList retains the compatibility list derived from ModelRatios.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MoonshotToolingDefaults does not configure client-executed Formula tools as
// server-side metered calls. Empty defaults do not imply that tool use is free.
var MoonshotToolingDefaults = adaptor.ChannelToolConfig{}
