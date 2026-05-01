package siliconflow

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Reusable metadata fragments for SiliconFlow-served open-weight models. SiliconFlow
// re-hosts upstream HuggingFace builds and exposes them through an OpenAI-compatible
// chat completion API. Most models are served at BF16 unless the upstream card
// explicitly specifies a quantized variant.
//
// Sources:
//   - https://docs.siliconflow.cn/cn/userguide/capabilities/text-generation
//   - https://siliconflow.cn/zh-cn/models
//   - Per-model HuggingFace cards (linked via HuggingFaceID).
var (
	// siliconflowTextModalities advertises the text-only modality used by every
	// chat model currently enumerated in this adaptor.
	siliconflowTextModalities = []string{"text"}

	// siliconflowChatSamplingParams enumerates the OpenAI-compatible sampling
	// parameters SiliconFlow's /v1/chat/completions accepts. Per the upstream
	// guide, callers can additionally pass top_k and frequency_penalty alongside
	// the standard OAI knobs.
	siliconflowChatSamplingParams = []string{
		"temperature",
		"top_p",
		"top_k",
		"max_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"response_format",
		"tools",
		"tool_choice",
	}

	// siliconflowLegacySamplingParams is the conservative parameter set advertised
	// for older small models (Qwen2 0.5B/1.5B, ChatGLM3, Yi 1.5 6B, Mistral v0.2)
	// that predate widespread tool/JSON support on SiliconFlow's bridge.
	siliconflowLegacySamplingParams = []string{
		"temperature",
		"top_p",
		"top_k",
		"max_tokens",
		"stop",
		"frequency_penalty",
	}

	// siliconflowChatFeatures lists the tool/JSON capabilities advertised by
	// SiliconFlow chat models that support function calling per the upstream docs.
	siliconflowChatFeatures = []string{"tools", "json_mode"}
)

// ModelRatios contains a conservative SiliconFlow compatibility snapshot.
// Model list is derived from the keys of this map, eliminating redundancy.
// SiliconFlow's public site no longer exposes an authoritative machine-readable pricing table in this environment,
// and the richer catalog is effectively account-gated, so this file intentionally avoids speculative churn.
//
// Pricing source: https://siliconflow.cn/pricing (snapshot retained from prior import).
// Capability metadata is derived from upstream HuggingFace model cards plus the
// SiliconFlow text-generation guide.
var ModelRatios = map[string]adaptor.ModelConfig{
	// SiliconFlow Models - Based on https://siliconflow.cn/pricing
	"deepseek-chat": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V2.5",
		Description:                 "SiliconFlow legacy alias for DeepSeek V2.5 chat (fp8, 64K context).",
	},
	"deepseek-coder": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-Coder-V2-Instruct",
		Description:                 "SiliconFlow legacy alias for DeepSeek Coder V2 instruct (fp8, 64K context).",
	},
	"Qwen/Qwen2-72B-Instruct": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "Qwen/Qwen2-72B-Instruct",
		Description:                 "Alibaba Qwen2 72B Instruct dense chat model with 128K context and tool/JSON support.",
	},
	"Qwen/Qwen2-7B-Instruct": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "Qwen/Qwen2-7B-Instruct",
		Description:                 "Alibaba Qwen2 7B Instruct chat model with 128K context.",
	},
	"Qwen/Qwen2-1.5B-Instruct": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "Qwen/Qwen2-1.5B-Instruct",
		Description:                 "Alibaba Qwen2 1.5B Instruct compact chat model with 32K context.",
	},
	"Qwen/Qwen2-0.5B-Instruct": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "Qwen/Qwen2-0.5B-Instruct",
		Description:                 "Alibaba Qwen2 0.5B Instruct ultra-compact chat model with 32K context.",
	},
	"meta-llama/Meta-Llama-3-8B-Instruct": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:                 "Meta Llama 3 8B Instruct chat model with 8K context.",
	},
	"meta-llama/Meta-Llama-3-70B-Instruct": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3-70B-Instruct",
		Description:                 "Meta Llama 3 70B Instruct dense chat model with 8K context.",
	},
	"meta-llama/Meta-Llama-3.1-8B-Instruct": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3.1-8B-Instruct",
		Description:                 "Meta Llama 3.1 8B Instruct chat model with 128K context and tool support.",
	},
	"meta-llama/Meta-Llama-3.1-70B-Instruct": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "meta-llama/Meta-Llama-3.1-70B-Instruct",
		Description:                 "Meta Llama 3.1 70B Instruct dense chat model with 128K context and tool support.",
	},
	"meta-llama/Meta-Llama-3.1-405B-Instruct": {
		Ratio:                       2.8 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "meta-llama/Meta-Llama-3.1-405B-Instruct",
		Description:                 "Meta Llama 3.1 405B Instruct flagship dense chat model with 128K context; served at fp8 by SiliconFlow.",
	},
	"mistralai/Mistral-7B-Instruct-v0.2": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mistral-7B-Instruct-v0.2",
		Description:                 "Mistral 7B Instruct v0.2 chat model with 32K context (legacy build, no native tool calling).",
	},
	"mistralai/Mixtral-8x7B-Instruct-v0.1": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description:                 "Mistral Mixtral 8x7B Instruct v0.1 sparse MoE chat model with 32K context.",
	},
	"01-ai/Yi-1.5-9B-Chat-16K": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "01-ai/Yi-1.5-9B-Chat-16K",
		Description:                 "01.AI Yi 1.5 9B chat model with 16K context (long-context variant).",
	},
	"01-ai/Yi-1.5-6B-Chat": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "01-ai/Yi-1.5-6B-Chat",
		Description:                 "01.AI Yi 1.5 6B chat model with 4K context.",
	},
	"THUDM/glm-4-9b-chat": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "THUDM/glm-4-9b-chat",
		Description:                 "Zhipu THUDM GLM-4 9B chat model with 128K context and native tool calling.",
	},
	"THUDM/chatglm3-6b": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowLegacySamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "THUDM/chatglm3-6b",
		Description:                 "Zhipu THUDM ChatGLM3 6B legacy chat model with 8K context.",
	},
	"internlm/internlm2_5-7b-chat": {
		Ratio:                       0.07 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "internlm/internlm2_5-7b-chat",
		Description:                 "Shanghai AI Lab InternLM 2.5 7B chat model with 32K context and tool support.",
	},
	"google/gemma-2-9b-it": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-9b-it",
		Description:                 "Google Gemma 2 9B instruction-tuned chat model with 8K context.",
	},
	"google/gemma-2-27b-it": {
		Ratio:                       0.28 * ratio.MilliTokensUsd,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-27b-it",
		Description:                 "Google Gemma 2 27B instruction-tuned chat model with 8K context.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// SiliconFlowToolingDefaults notes that SiliconFlow public docs focus on model usage; no separate tool fees are published (retrieved 2026-04-28).
// Source: https://siliconflow.cn/pricing
var SiliconFlowToolingDefaults = adaptor.ChannelToolConfig{}
