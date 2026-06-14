package baichuan

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Baichuan Inc. operates the Baichuan / Baichuan2 / Baichuan3 / Baichuan4
// model families. Reference docs:
//   - https://platform.baichuan-ai.com/docs/api (chat API spec)
//   - https://platform.baichuan-ai.com/prices (model pricing, retrieved 2026-05-18)
//
// Baichuan2 base / chat weights (7B, 13B) are open-source on HuggingFace
// (baichuan-inc/Baichuan2-13B-*). The hosted Baichuan2-Turbo-* and Baichuan3/4
// SKUs are closed-weight productionized variants. The text embedding model is
// closed-weight as well. Baichuan2-Turbo-192k was discontinued 2024-08-16 and
// upstream now routes those requests to Baichuan3-Turbo-128k.

// baichuanTextInputs is the input modality set for Baichuan chat / embedding.
var baichuanTextInputs = []string{"text"}

// baichuanTextOutputs is the output modality set for Baichuan chat models.
var baichuanTextOutputs = []string{"text"}

// baichuanChatFeatures advertises tool calling / JSON mode support exposed by
// the Baichuan OpenAI-compatible chat completion API.
var baichuanChatFeatures = []string{"tools", "json_mode"}

// baichuanSamplingParams lists OpenAI-style sampling parameters accepted by
// Baichuan chat endpoints.
var baichuanSamplingParams = []string{
	"temperature",
	"top_p",
	"top_k",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Based on Baichuan pricing: https://platform.baichuan-ai.com/prices
// Most Baichuan rates have input == output pricing (CompletionRatio = 1); the
// medical-enhanced M-series (Baichuan-M2/M2-Plus/M3/M3-Plus) bills output above
// input, so those entries carry an explicit CompletionRatio.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Baichuan 4 family (current flagship).
	"Baichuan4": {
		Ratio:                       100 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan4 flagship closed-weight chat model with 32k context.",
	},
	"Baichuan4-Turbo": {
		Ratio:                       15 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan4-Turbo lower-latency tier of Baichuan4 with 32k context.",
	},
	"Baichuan4-Air": {
		Ratio:                       0.98 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan4-Air entry-tier closed-weight chat model with 32k context.",
	},

	// Baichuan 3 family.
	"Baichuan3-Turbo": {
		Ratio:                       12 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan3-Turbo production chat model with 32k context.",
	},
	"Baichuan3-Turbo-128k": {
		Ratio:                       24 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               128000,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan3-Turbo with extended 128k context for long-document workloads (replaces retired Baichuan2-Turbo-192k).",
	},

	// Baichuan 2 family. Baichuan2 base / chat weights (7B, 13B) are open on
	// HuggingFace; Baichuan2-Turbo is the productionized closed-weight tier.
	"Baichuan2-Turbo": {
		Ratio:                       8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baichuan-inc/Baichuan2-13B-Chat",
		Description:                 "Baichuan2-Turbo production chat model. Smaller Baichuan2 variants are open-weight on HuggingFace.",
	},

	// Baichuan medical-enhanced (M-series) chat models. Pricing from
	// https://platform.baichuan-ai.com/prices (verified 2026-06-14); these have
	// input != output rates. Auto-triggered 医疗搜索 (medical search) on the
	// M-Plus tiers is billed separately at ¥0.03/次 and is not modeled per-token.
	"Baichuan-M3-Plus": {
		Ratio:                       5 * ratio.MilliTokensRmb,
		CompletionRatio:             9.0 / 5.0,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan-M3-Plus medical-enhanced flagship chat model with 32k context (auto medical-search billed separately ¥0.03/call).",
	},
	"Baichuan-M3": {
		Ratio:                       10 * ratio.MilliTokensRmb,
		CompletionRatio:             30.0 / 10.0,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan-M3 medical-enhanced chat model with 32k context.",
	},
	"Baichuan-M2-Plus": {
		Ratio:                       10 * ratio.MilliTokensRmb,
		CompletionRatio:             30.0 / 10.0,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan-M2-Plus medical-enhanced chat model with 32k context (auto medical-search billed separately ¥0.03/call).",
	},
	"Baichuan-M2": {
		Ratio:                       2 * ratio.MilliTokensRmb,
		CompletionRatio:             20.0 / 2.0,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan-M2 medical-enhanced reasoning chat model with 32k context.",
	},

	// Embedding model.
	"Baichuan-Text-Embedding": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  baichuanTextInputs,
		OutputModalities: baichuanTextOutputs,
		Description:      "Baichuan text embedding model used for semantic similarity and retrieval workloads.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaichuanToolingDefaults documents that Baichuan's public pricing page lists only model token rates with no per-tool fees (retrieved 2026-05-18).
// Source: https://platform.baichuan-ai.com/prices
var BaichuanToolingDefaults = adaptor.ChannelToolConfig{}
