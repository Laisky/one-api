package moonshot

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Current model IDs, capabilities, and native CNY tariffs were checked on 2026-10-24:
// https://platform.kimi.com/docs/models
// https://platform.kimi.com/docs/guide/kimi-k3-1-pro-quickstart
// https://platform.kimi.com/docs/pricing/chat
// https://platform.kimi.ai/docs/api/models-overview
//
// The numeric tables are embedded in the pricing HTML's compiled source.
// International USD prices are separate tariffs, not an FX conversion. Cache
// writes replace normal input charges for those tokens; native 1h receipt
// normalization remains a separate limitation. See docs/moonshot-model-audit-20261024.md.
const (
	moonshotRetiredNotice      = " Retired on 2026-08-31; no longer supported by the Kimi API. Use kimi-k3.1-pro. This entry retains historical pricing for existing configurations, not current upstream availability."
	moonshotK2RetirementNotice = " Unavailable to newly registered accounts from 2026-10-31; existing accounts remain eligible, with retirement planned for the end of December 2026."
	moonshotK3ControlsNotice   = " Text/image/video input; always-on low/high/max reasoning (default max). max_completion_tokens defaults to 131072 and can reach 1048576, subject to remaining context. Images require base64 or ms:// file references. Separate 5m/1h cache-write prices are published, but native 1h write accounting is not yet supported. Upstream search is not recommended for production."
)

var (
	moonshotTextInputs         = []string{"text"}
	moonshotVisionInputs       = []string{"text", "image"}
	moonshotMultimodalInputs   = []string{"text", "image", "video"}
	moonshotTextOutputs        = []string{"text"}
	moonshotChatFeatures       = []string{"tools", "json_mode", "structured_outputs"}
	moonshotReasoningFeatures  = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	moonshotMultimodalFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning", "web_search"}
	moonshotSamplingParams     = []string{"temperature", "top_p", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"}
	// Current K2 and K3 models fix temperature/top_p/penalties. Keep the
	// historical sampling metadata above only for retained legacy models.
	moonshotK2SamplingParams   = []string{"max_tokens", "max_completion_tokens", "stop"}
	moonshotK3SamplingParams   = []string{"max_tokens", "max_completion_tokens", "stop", "reasoning_effort"}
	moonshotK3ReasoningEfforts = []string{"low", "high", "max"}
)

// ModelRatios contains complete definitions for current and retained legacy IDs.
// No catalogue metadata is used to rewrite a caller's model or impose access caps.
var ModelRatios = map[string]adaptor.ModelConfig{
	"kimi-k3.1-pro": {
		Ratio: 13.0 * ratio.MilliTokensRmb, CompletionRatio: 68.0 / 13.0,
		CachedInputRatio:  1.3 * ratio.MilliTokensRmb,
		CacheWrite5mRatio: 13.0 * ratio.MilliTokensRmb, CacheWrite1hRatio: 26.0 * ratio.MilliTokensRmb,
		ContextLength: 1048576, MaxOutputTokens: 1048576,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK3SamplingParams,
		SupportedReasoningEfforts: moonshotK3ReasoningEfforts, DefaultReasoningEffort: "max",
		Description: "Kimi K3.1 Pro: flagship model with 1M context." + moonshotK3ControlsNotice,
	},
	"kimi-k3.1-pro-highspeed": {
		Ratio: 26.0 * ratio.MilliTokensRmb, CompletionRatio: 136.0 / 26.0,
		CachedInputRatio:  1.3 * ratio.MilliTokensRmb,
		CacheWrite5mRatio: 26.0 * ratio.MilliTokensRmb, CacheWrite1hRatio: 52.0 * ratio.MilliTokensRmb,
		ContextLength: 1048576, MaxOutputTokens: 1048576,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK3SamplingParams,
		SupportedReasoningEfforts: moonshotK3ReasoningEfforts, DefaultReasoningEffort: "max",
		Description: "Kimi K3.1 Pro Highspeed: faster serving with the same capabilities and cache-read price as Pro." + moonshotK3ControlsNotice,
	},
	"kimi-k3": {
		Ratio: 20.0 * ratio.MilliTokensRmb, CompletionRatio: 100.0 / 20.0,
		CachedInputRatio:  2.0 * ratio.MilliTokensRmb,
		CacheWrite5mRatio: 20.0 * ratio.MilliTokensRmb, CacheWrite1hRatio: 40.0 * ratio.MilliTokensRmb,
		ContextLength: 1048576, MaxOutputTokens: 1048576,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK3SamplingParams,
		SupportedReasoningEfforts: moonshotK3ReasoningEfforts, DefaultReasoningEffort: "max",
		Description: "Kimi K3: previous-generation flagship with 1M context." + moonshotK3ControlsNotice,
	},
	"kimi-k2.7-code": {
		Ratio: 6.5 * ratio.MilliTokensRmb, CompletionRatio: 27.0 / 6.5,
		CachedInputRatio: 1.3 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 262144,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK2SamplingParams,
		HuggingFaceID: "moonshotai/Kimi-K2.7-Code",
		Description:   "Kimi K2.7 Code: text/image/video coding model with 256K context and always-on preserved thinking. The 32768-token default completion budget can grow to remaining context." + moonshotK2RetirementNotice,
	},
	"kimi-k2.7-code-highspeed": {
		Ratio: 13.0 * ratio.MilliTokensRmb, CompletionRatio: 54.0 / 13.0,
		CachedInputRatio: 2.6 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 262144,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK2SamplingParams,
		HuggingFaceID: "moonshotai/Kimi-K2.7-Code",
		Description:   "Kimi K2.7 Code Highspeed: faster serving with the same 256K context, completion budget, and always-on preserved thinking as K2.7 Code. Priced at 2x standard K2.7 Code." + moonshotK2RetirementNotice,
	},
	"kimi-k2.6": {
		Ratio: 6.5 * ratio.MilliTokensRmb, CompletionRatio: 27.0 / 6.5,
		CachedInputRatio: 1.1 * ratio.MilliTokensRmb,
		ContextLength:    262144, MaxOutputTokens: 262144,
		InputModalities: moonshotMultimodalInputs, OutputModalities: moonshotTextOutputs,
		SupportedFeatures: moonshotReasoningFeatures, SupportedSamplingParameters: moonshotK2SamplingParams,
		HuggingFaceID: "moonshotai/Kimi-K2.6",
		Description:   "Kimi K2.6: text/image/video model with 256K context and switchable thinking. The 32768-token default completion budget can grow to remaining context. Sampling is fixed in each thinking mode; upstream search is not recommended for production." + moonshotK2RetirementNotice,
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
