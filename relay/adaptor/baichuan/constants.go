package baichuan

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Baichuan Inc. operates the Baichuan / Baichuan2 model family. Reference docs:
//   - https://platform.baichuan-ai.com/docs/api (chat API spec)
//   - https://platform.baichuan-ai.com/price (model pricing)
//
// Baichuan2 base / chat weights (7B, 13B) are open-source on HuggingFace
// (baichuan-inc/Baichuan2-13B-*). The hosted Baichuan2-Turbo-* and Baichuan2-53B
// SKUs are closed-weight productionized variants. The text embedding model is
// closed-weight as well.

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

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
var ModelRatios = map[string]adaptor.ModelConfig{
	// Baichuan Models - Based on https://platform.baichuan-ai.com/price
	"Baichuan2-Turbo": {
		Ratio:                       0.008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan2-Turbo production chat model derived from the open-weight Baichuan2 family.",
	},
	"Baichuan2-Turbo-192k": {
		Ratio:                       0.016 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               192000,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Description:                 "Baichuan2-Turbo with extended 192k context window for long-document workloads.",
	},
	"Baichuan2-53B": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             baichuanTextInputs,
		OutputModalities:            baichuanTextOutputs,
		SupportedFeatures:           baichuanChatFeatures,
		SupportedSamplingParameters: baichuanSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "baichuan-inc/Baichuan2-13B-Chat",
		Description:                 "Baichuan2 53B-class chat model. Smaller open-weight Baichuan2 variants are published on HuggingFace.",
	},
	"Baichuan-Text-Embedding": {
		Ratio:            0.002 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    512,
		InputModalities:  baichuanTextInputs,
		OutputModalities: baichuanTextOutputs,
		Description:      "Baichuan text embedding model used for semantic similarity and retrieval workloads.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// baichuanToolingDefaults documents that Baichuan's public console does not expose tool billing without authentication (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://platform.baichuan-ai.com/price (returns 404)
var BaichuanToolingDefaults = adaptor.ChannelToolConfig{}
