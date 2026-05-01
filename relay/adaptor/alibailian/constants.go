package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Alibaba Bailian (Model Studio) hosted Qwen and
// DeepSeek chat models. Reused across ModelRatios entries so the table stays
// compact and consistent.
var (
	// bailianTextInputs lists the input modalities for text-only Bailian chat models.
	bailianTextInputs = []string{"text"}
	// bailianTextOutputs lists the output modalities for Bailian chat completions.
	bailianTextOutputs = []string{"text"}

	// bailianChatFeatures advertises the capability set for non-reasoning Qwen
	// chat/coder/translate tiers on Bailian: tool-calling, JSON mode and
	// structured outputs are supported per Model Studio docs.
	bailianChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// bailianReasoningFeatures advertises the capability set for Qwen reasoning
	// models (QwQ) and hosted DeepSeek thinking models.
	bailianReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// bailianSamplingParameters lists the OpenAI-compatible sampling parameters
	// Bailian accepts. Bailian additionally exposes top_k and repetition_penalty
	// alongside the standard OpenAI knobs for Chinese-cloud chat APIs.
	bailianSamplingParameters = []string{
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
	// bailianReasoningSamplingParameters lists the constrained sampling-parameter
	// set supported by Qwen-style reasoning models on Bailian (QwQ etc.), which
	// reject most decoding-tuning knobs and reliably accept only seed + max_tokens.
	bailianReasoningSamplingParameters = []string{"seed", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing/configuration metadata.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Pricing/metadata sources (verified 2026-05-01):
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
//   - https://www.alibabacloud.com/help/en/model-studio/models
//   - https://huggingface.co/Qwen
//   - https://huggingface.co/deepseek-ai
var ModelRatios = map[string]adaptor.ModelConfig{
	// Qwen Models
	"qwen-turbo": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Turbo",
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Turbo on Bailian: cost-efficient chat tier with up to 1M context.",
	},
	"qwen-plus": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Plus on Bailian: balanced flagship chat tier with 128K context.",
	},
	"qwen-long": {
		Ratio:                       0.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               10000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Long on Bailian: longest-context chat tier (up to 10M tokens) for document QA.",
	},
	"qwen-max": {
		Ratio:                       20.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Max on Bailian: most capable Qwen2.5-class chat tier with 32K context.",
	},
	"qwen-coder-plus": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-32B-Instruct",
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Coder Plus on Bailian: code-specialized chat tier with 128K context.",
	},
	"qwen-coder-plus-latest": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-32B-Instruct",
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Coder Plus (latest alias) on Bailian: code-specialized chat tier.",
	},
	"qwen-coder-turbo": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-7B-Instruct",
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Coder Turbo on Bailian: cost-efficient code-specialized chat tier.",
	},
	"qwen-coder-turbo-latest": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-7B-Instruct",
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen Coder Turbo (latest alias) on Bailian: cost-efficient code-specialized chat tier.",
	},
	"qwen-mt-plus": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen MT Plus on Bailian: flagship machine-translation model.",
	},
	"qwen-mt-turbo": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Alibaba Qwen MT Turbo on Bailian: cost-efficient machine-translation model.",
	},
	"qwq-32b-preview": {
		Ratio:                       0.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		HuggingFaceID:               "Qwen/QwQ-32B-Preview",
		Quantization:                "bf16",
		Description:                 "Alibaba QwQ-32B-Preview on Bailian: experimental open-weight reasoning model with 32K context.",
	},

	// DeepSeek Models (hosted on Alibaba)
	"deepseek-r1": {
		Ratio:                       1.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Quantization:                "fp8",
		Description:                 "DeepSeek R1 hosted on Bailian: open-weight reasoning chat model (thinking mode).",
	},
	"deepseek-v3": {
		Ratio:                       0.07 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Quantization:                "fp8",
		Description:                 "DeepSeek V3 hosted on Bailian: open-weight MoE chat model (non-thinking mode).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AlibailianToolingDefaults reflects that Bailian's public docs do not disclose per-tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.alibabacloud.com/help/en/model-studio/latest/billing (returns 404 for unauthenticated access)
var AlibailianToolingDefaults = adaptor.ChannelToolConfig{}
