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
	// hunyuanVisionInputs lists the input modalities for multimodal Hunyuan vision endpoints.
	hunyuanVisionInputs = []string{"text", "image"}
	// hunyuanVisionVideoInputs lists the input modalities for Hunyuan vision endpoints that also accept video frames.
	hunyuanVisionVideoInputs = []string{"text", "image", "file"}
	// hunyuanTextOutputs lists the output modalities for all Hunyuan chat completions.
	hunyuanTextOutputs = []string{"text"}

	// hunyuanChatFeatures advertises the capability set for Hunyuan chat models with
	// tool-calling and JSON mode support per Tencent Cloud Hunyuan API docs.
	hunyuanChatFeatures = []string{"tools", "json_mode"}
	// hunyuanLiteFeatures lists the capability set for hunyuan-lite, which historically
	// does not advertise JSON mode in the public Tencent docs (retrieved 2026-05-18).
	hunyuanLiteFeatures = []string{"tools"}
	// hunyuanReasoningFeatures lists the capability set for the Hunyuan T1 reasoning
	// family. Tencent exposes reasoning as a binary switch rather than a tunable budget,
	// so DefaultReasoningEffort / MaxReasoningTokens stay unset per memory feedback.
	hunyuanReasoningFeatures = []string{"tools", "json_mode", "reasoning"}
	// hunyuanVisionFeatures advertises the capability set for Hunyuan multimodal models.
	hunyuanVisionFeatures = []string{"tools", "json_mode"}

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
// Pricing source (verified 2026-05-18):
//   - https://cloud.tencent.com/document/product/1729/97731 (Hunyuan generative pricing)
//   - https://cloud.tencent.com/document/product/1729/97732 (Hunyuan API overview / model catalog)
//
// Capability metadata sources:
//   - https://cloud.tencent.com/document/product/1729 (Hunyuan model overview)
//   - https://hunyuan.tencent.com/ (model lineup, multimodal capabilities)
//   - https://huggingface.co/tencent/Hunyuan-A13B-Instruct (Hunyuan A13B open-weight release on HF)
//   - https://huggingface.co/tencent/Hunyuan-Large (Hunyuan Large open-weight MoE release on HF)
//
// Notes:
//   - hunyuan-lite is offered free of charge per the May 2026 pricing page; we keep Ratio at 0.
//   - Legacy hunyuan-standard, hunyuan-standard-256K, and hunyuan-pro entries are retained even
//     though they no longer appear on the current pricing page — Tencent has not announced
//     deprecation, and channels with grandfathered access continue to bill at the previous rates.
//   - hunyuan-t1 exposes reasoning via a binary thinking mode; MaxReasoningTokens is intentionally
//     left at zero per the project-wide guidance that Tencent thinking is not tunable by budget.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Hunyuan Lite (free tier per https://cloud.tencent.com/document/product/1729/97731)
	"hunyuan-lite": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               262144,
		MaxOutputTokens:             6144,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanLiteFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Lite: lightweight closed-weight chat model with a 256K context window, offered free of charge.",
	},

	// Hunyuan Standard family — retained for backward compatibility with channels grandfathered
	// onto the legacy SKUs. The current public pricing page no longer lists these models.
	"hunyuan-standard": {
		Ratio:                       4.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Standard: legacy closed-weight general-purpose chat model with a 32K context window.",
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
		Description:                 "Tencent Hunyuan Standard 256K: legacy long-context closed-weight chat model with a 256K context window.",
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
		Description:                 "Tencent Hunyuan Pro: legacy flagship closed-weight chat model targeted at complex reasoning and agent tasks.",
	},

	// Hunyuan Turbo family — current flagship balanced-cost tier per the May 2026 pricing page.
	"hunyuan-turbo": {
		Ratio:                       2.4 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Turbo: closed-weight balanced-cost chat tier; predecessor of hunyuan-turbos.",
	},
	"hunyuan-turbos": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             2.5,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan TurboS: Mamba-Transformer hybrid closed-weight chat model with a 256K context window.",
	},

	// Hunyuan T1 reasoning family — exposes a binary thinking mode (no tunable budget).
	"hunyuan-t1": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanReasoningFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan T1: closed-weight deep-thinking reasoning chat model with a 256K context window.",
	},

	// Hunyuan A13B — open-weight (HuggingFace tencent/Hunyuan-A13B-Instruct).
	"hunyuan-a13b": {
		Ratio:                       0.5 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		HuggingFaceID:               "tencent/Hunyuan-A13B-Instruct",
		Description:                 "Tencent Hunyuan A13B: open-weight 13B-active MoE chat model hosted on Tencent Cloud.",
	},

	// Hunyuan Large — open-weight (HuggingFace tencent/Hunyuan-Large).
	"hunyuan-large-role": {
		Ratio:                       2.4 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Large Role: closed-weight role-play chat model derived from Hunyuan-Large.",
	},

	// Hunyuan Translation models.
	"hunyuan-translation": {
		Ratio:                       1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Translation: closed-weight translation-tuned chat model.",
	},
	"hunyuan-translation-lite": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanTextInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanChatFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Translation Lite: cost-efficient closed-weight translation chat model.",
	},

	// Hunyuan multimodal vision family — accept text + image inputs.
	"hunyuan-vision": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             hunyuanVisionInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanVisionFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan Vision: legacy closed-weight multimodal chat model accepting text and image inputs.",
	},
	"hunyuan-turbos-vision": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanVisionInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanVisionFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan TurboS Vision: closed-weight multimodal chat model based on the TurboS hybrid architecture.",
	},
	"hunyuan-turbos-vision-video": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanVisionVideoInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanVisionFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan TurboS Vision Video: closed-weight multimodal chat model accepting text, image, and video inputs.",
	},
	"hunyuan-t1-vision": {
		Ratio:                       3 * ratio.MilliTokensRmb,
		CompletionRatio:             3,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             hunyuanVisionInputs,
		OutputModalities:            hunyuanTextOutputs,
		SupportedFeatures:           hunyuanReasoningFeatures,
		SupportedSamplingParameters: hunyuanSamplingParameters,
		Description:                 "Tencent Hunyuan T1 Vision: closed-weight multimodal reasoning chat model with binary thinking mode.",
	},

	// Hunyuan Embedding — single-modality text embedding model.
	"hunyuan-embedding": {
		Ratio:            0.7 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    1024,
		InputModalities:  hunyuanTextInputs,
		OutputModalities: hunyuanTextOutputs,
		Description:      "Tencent Hunyuan Embedding: closed-weight Chinese-first text embedding model.",
	},
}

// TencentToolingDefaults notes that Tencent Hunyuan pricing covers models only; no tool tariffs are posted (retrieved 2026-05-18).
// Source: https://r.jina.ai/https://cloud.tencent.com/document/product/1729/97731
var TencentToolingDefaults = adaptor.ChannelToolConfig{}
