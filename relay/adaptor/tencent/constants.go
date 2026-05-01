package tencent

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Tencent Hunyuan chat models. Values are reused
// across ModelRatios entries to keep the table compact and consistent.
var (
	// hunyuanTextInputs lists the input modalities for text-only Hunyuan chat models.
	hunyuanTextInputs = []string{"text"}
	// hunyuanVisionInputs lists the input modalities for the multimodal hunyuan-vision endpoint.
	hunyuanVisionInputs = []string{"text", "image"}
	// hunyuanTextOutputs lists the output modalities for all Hunyuan chat completions.
	hunyuanTextOutputs = []string{"text"}

	// hunyuanChatFeatures advertises the capability set for Hunyuan chat models with
	// tool-calling and JSON mode support per Tencent Cloud Hunyuan API docs.
	hunyuanChatFeatures = []string{"tools", "json_mode"}
	// hunyuanLiteFeatures lists the capability set for hunyuan-lite, which historically
	// does not advertise JSON mode in the public Tencent docs (retrieved 2026-05-01).
	hunyuanLiteFeatures = []string{"tools"}

	// hunyuanSamplingParameters lists the sampling parameters Tencent Hunyuan accepts.
	// In addition to the OpenAI-compatible knobs, Hunyuan exposes top_k and
	// repetition_penalty as documented for Chinese-cloud chat APIs.
	hunyuanSamplingParameters = []string{
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
// Pricing source (verified 2026-05-01):
//   - https://cloud.tencent.com/document/product/1729/97731
//
// Capability metadata sources:
//   - https://cloud.tencent.com/document/product/1729 (Hunyuan model overview)
//   - https://hunyuan.tencent.com/ (model lineup, multimodal capabilities)
//   - https://huggingface.co/tencent/Hunyuan-A13B-Instruct (only Hunyuan family open-weight on HF;
//     Lite/Standard/Pro/Vision/Embedding are closed-weight Tencent Cloud services)
var ModelRatios = map[string]adaptor.ModelConfig{
	// Hunyuan Models - Based on https://cloud.tencent.com/document/product/1729/97731
	"hunyuan-lite": {
		Ratio:                       0.75 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               262144,
		MaxOutputTokens:             6144,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanLiteFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Lite: lightweight closed-weight chat model with a 256K context window.",
	},
	"hunyuan-standard": {
		Ratio:                       4.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Standard: closed-weight general-purpose chat model with a 32K context window.",
	},
	"hunyuan-standard-256K": {
		Ratio:                       15 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               262144,
		MaxOutputTokens:             6144,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Standard 256K: long-context closed-weight chat model with a 256K context window.",
	},
	"hunyuan-pro": {
		Ratio:                       30 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Pro: flagship closed-weight chat model targeted at complex reasoning and agent tasks.",
	},
	"hunyuan-vision": {
		Ratio:                       18 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanVisionInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Vision: closed-weight multimodal chat model accepting text and image inputs.",
	},
	"hunyuan-embedding": {
		Ratio:            0.7 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    1024,
		InputModalities:  hunyuanTextInputs,
		OutputModalities: hunyuanTextOutputs,
		Description:      "Tencent Hunyuan Embedding: closed-weight Chinese-first text embedding model.",
	},
}

// TencentToolingDefaults notes that Tencent Hunyuan pricing covers models only; no tool tariffs are posted (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.tencent.com/document/product/1729/97731
var TencentToolingDefaults = adaptor.ChannelToolConfig{}
