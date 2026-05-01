package baidu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Baidu Qianfan / Wenxin (ERNIE) v1 chat and embedding
// models. The legacy v1 ERNIE family is closed-weight; values are reused across
// ModelRatios entries to keep the table compact and consistent.
var (
	// ernieTextInputs lists the input modalities for text-only ERNIE chat models.
	ernieTextInputs = []string{"text"}
	// ernieTextOutputs lists the output modalities for ERNIE chat completions.
	ernieTextOutputs = []string{"text"}

	// ernieChatFeatures advertises the capability set for ERNIE chat models that
	// support tool-calling and JSON responses on the Qianfan v1 endpoint.
	ernieChatFeatures = []string{"tools", "json_mode"}
	// ernieSpeedFeatures lists the capability set for the Speed/Lite/Tiny/Character
	// tiers, which expose tool-calling but not structured JSON mode in the public docs.
	ernieSpeedFeatures = []string{"tools"}

	// ernieSamplingParameters lists the OpenAI-compatible sampling parameters Baidu
	// Qianfan accepts for ERNIE chat models. Baidu also exposes top_k and
	// repetition penalties via penalty_score on Qianfan, captured here as
	// repetition_penalty for portability with other Chinese-cloud adaptors.
	ernieSamplingParameters = []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
)

// ModelRatios contains all supported models and their pricing/configuration metadata.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Pricing source: https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf (verified 2026-05-01).
//
// Capability metadata sources:
//   - https://cloud.baidu.com/doc/WENXINWORKSHOP/index.html (ERNIE legacy v1 model lineup)
//   - https://ai.baidu.com/ai-doc/AISTUDIO/Mmhslv9lf (per-model context/output limits)
//
// Notes:
//   - All ERNIE legacy v1 chat models are closed-weight; HuggingFaceID and Quantization are intentionally empty.
//   - Embedding-V1 / bge-large-* / tao-8k are embedding/representation models; chat-only fields are omitted.
var ModelRatios = map[string]adaptor.ModelConfig{
	// ERNIE 4.0 Models
	"ERNIE-4.0-8K": {
		Ratio:                       12 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K: closed-weight flagship chat model on the Qianfan v1 API.",
	},

	// ERNIE 3.5 Models
	"ERNIE-3.5-8K": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K: closed-weight general-purpose chat model on the Qianfan v1 API.",
	},
	"ERNIE-3.5-8K-0205": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K (2024-02-05 snapshot): pinned closed-weight chat model.",
	},
	"ERNIE-3.5-8K-1222": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K (2023-12-22 snapshot): pinned closed-weight chat model.",
	},
	"ERNIE-Bot-8K": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE-Bot 8K: legacy alias for the ERNIE 3.5 8K chat model.",
	},
	"ERNIE-3.5-4K-0205": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieChatFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE 3.5 4K (2024-02-05 snapshot): short-context closed-weight chat model.",
	},

	// ERNIE Speed Models
	"ERNIE-Speed-8K": {
		Ratio:                       0.4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieSpeedFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE Speed 8K: throughput-optimized closed-weight chat tier.",
	},
	"ERNIE-Speed-128K": {
		Ratio:                       0.4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieSpeedFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE Speed 128K: long-context throughput-optimized closed-weight chat tier.",
	},

	// ERNIE Lite Models
	"ERNIE-Lite-8K-0922": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieSpeedFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE Lite 8K (2023-09-22 snapshot): cost-efficient closed-weight chat tier.",
	},
	"ERNIE-Lite-8K-0308": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieSpeedFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE Lite 8K (2024-03-08 snapshot): cost-efficient closed-weight chat tier.",
	},

	// ERNIE Tiny Models
	"ERNIE-Tiny-8K": {
		Ratio:                       0.4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedFeatures:           ernieSpeedFeatures,
		SupportedSamplingParameters: ernieSamplingParameters,
		Description:                 "Baidu ERNIE Tiny 8K: ultra low-cost closed-weight chat tier.",
	},

	// Other Models
	"BLOOMZ-7B": {
		Ratio:                       0.4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               2048,
		MaxOutputTokens:             1024,
		InputModalities:             ernieTextInputs,
		OutputModalities:            ernieTextOutputs,
		SupportedSamplingParameters: ernieSamplingParameters,
		HuggingFaceID:               "bigscience/bloomz-7b1",
		Quantization:                "fp16",
		Description:                 "BLOOMZ-7B: open-weight multilingual instruction-tuned model hosted on Qianfan.",
	},

	// Embedding Models
	"Embedding-V1": {
		Ratio:            0.2 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    384,
		InputModalities:  ernieTextInputs,
		OutputModalities: ernieTextOutputs,
		Description:      "Baidu Embedding-V1: closed-weight text embedding model on the Qianfan v1 API.",
	},
	"bge-large-zh": {
		Ratio:            0.2 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  ernieTextInputs,
		OutputModalities: ernieTextOutputs,
		HuggingFaceID:    "BAAI/bge-large-zh-v1.5",
		Quantization:     "fp16",
		Description:      "BAAI BGE Large ZH v1.5: open-weight Chinese text embedding model hosted on Qianfan.",
	},
	"bge-large-en": {
		Ratio:            0.2 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  ernieTextInputs,
		OutputModalities: ernieTextOutputs,
		HuggingFaceID:    "BAAI/bge-large-en-v1.5",
		Quantization:     "fp16",
		Description:      "BAAI BGE Large EN v1.5: open-weight English text embedding model hosted on Qianfan.",
	},

	// TAO Models
	"tao-8k": {
		Ratio:            0.8 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  ernieTextInputs,
		OutputModalities: ernieTextOutputs,
		Description:      "Baidu TAO 8K: closed-weight long-text embedding model on the Qianfan v1 API.",
	},
}

// BaiduToolingDefaults notes that Wenxin ModelBuilder documentation does not disclose per-tool billing publicly (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
var BaiduToolingDefaults = adaptor.ChannelToolConfig{}
