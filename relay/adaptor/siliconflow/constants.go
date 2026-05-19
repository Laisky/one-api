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
// SiliconFlow's public site is partially account-gated, so this file
// intentionally avoids speculative churn and only enumerates models with
// publicly visible USD pricing on the English pricing page.
//
// Pricing sources (retrieved 2026-05-18):
//   - https://www.siliconflow.com/pricing (English; USD per million tokens / image / second)
//   - https://siliconflow.cn/pricing (Chinese; CNY per million tokens — converted at 0.14 USD/CNY for legacy aliases)
//   - https://siliconflow.cn/zh-cn/models (catalog, partially login-gated)
//
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

	// === 2026-era models (USD pricing from https://www.siliconflow.com/pricing) ===

	"deepseek-ai/DeepSeek-V3.2-Exp": {
		// $0.27/M input, $0.42/M output (sparse-attention long-context variant).
		Ratio:                       0.27 * ratio.MilliTokensUsd,
		CompletionRatio:             0.42 / 0.27,
		ContextLength:               163840,
		MaxOutputTokens:             16384,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.2-Exp",
		Description:                 "DeepSeek V3.2 Exp sparse-attention hybrid reasoning + chat MoE model on SiliconFlow.",
	},
	"Qwen/Qwen3-235B-A22B-Thinking-2507": {
		// $0.35/M input, $1.42/M output thinking variant.
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.42 / 0.35,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp8",
		HuggingFaceID:               "Qwen/Qwen3-235B-A22B-Thinking-2507",
		Description:                 "Alibaba Qwen3 235B/A22B Thinking MoE reasoning model with 256K context on SiliconFlow.",
	},
	"Qwen/Qwen3-235B-A22B-Instruct-2507": {
		// SiliconFlow publishes the instruct variant alongside the thinking variant.
		// Same input price tier as the thinking model on the public board.
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.42 / 0.35,
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "Qwen/Qwen3-235B-A22B-Instruct-2507",
		Description:                 "Alibaba Qwen3 235B/A22B Instruct MoE chat model with 256K context on SiliconFlow.",
	},
	"moonshotai/Kimi-K2.5-Instruct": {
		// $0.45/M input, $2.25/M output.
		Ratio:                       0.45 * ratio.MilliTokensUsd,
		CompletionRatio:             2.25 / 0.45,
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp8",
		HuggingFaceID:               "moonshotai/Kimi-K2-Instruct",
		Description:                 "Moonshot Kimi K2.5 Instruct long-context MoE chat model on SiliconFlow.",
	},
	"moonshotai/Kimi-K2.6-Instruct": {
		// $0.90/M input, $4.00/M output.
		Ratio:                       0.90 * ratio.MilliTokensUsd,
		CompletionRatio:             4.00 / 0.90,
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Quantization:                "fp8",
		HuggingFaceID:               "moonshotai/Kimi-K2-Instruct",
		Description:                 "Moonshot Kimi K2.6 Instruct long-context MoE chat model on SiliconFlow.",
	},
	"zai-org/GLM-4.7": {
		// $0.42/M input, $2.20/M output.
		Ratio:                       0.42 * ratio.MilliTokensUsd,
		CompletionRatio:             2.20 / 0.42,
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             siliconflowTextModalities,
		OutputModalities:            siliconflowTextModalities,
		SupportedFeatures:           siliconflowChatFeatures,
		SupportedSamplingParameters: siliconflowChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "zai-org/GLM-4.7",
		Description:                 "Zhipu GLM-4.7 chat model with native tool calling and 128K context on SiliconFlow.",
	},

	// === Image models (USD per image) ===
	"black-forest-labs/FLUX.1-schnell": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.0014,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		HuggingFaceID:    "black-forest-labs/FLUX.1-schnell",
		Description:      "FLUX.1 [schnell] 12B Apache-licensed text-to-image model on SiliconFlow.",
	},
	"black-forest-labs/FLUX.1.1-pro": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "FLUX 1.1 [pro] managed text-to-image generator on SiliconFlow.",
	},
	"black-forest-labs/FLUX.2-pro": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.03,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "FLUX.2 [pro] high-resolution text-to-image and multi-reference editing on SiliconFlow.",
	},
	"black-forest-labs/FLUX.2-flex": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.06,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "FLUX.2 [flex] tunable-step text-to-image variant on SiliconFlow.",
	},
	"Bytedance/Z-Image-Turbo": {
		Ratio: 0, CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.005,
			MinImages:        1,
			DefaultSize:      "1024x1024",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "ByteDance Z-Image Turbo fast text-to-image generator on SiliconFlow.",
	},

	// === Audio models ===
	"FunAudioLLM/CosyVoice2-0.5B": {
		// SiliconFlow bills CosyVoice2 at $7.15 per 1M UTF-8 input bytes.
		// Approximate 1 token ≈ 4 UTF-8 bytes for English/Chinese mix → ~$0.029 / M tokens
		// expressed as audio prompt-token billing for accounting purposes.
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio: (7.15 / 4.0) * ratio.MilliTokensUsd,
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		HuggingFaceID:    "FunAudioLLM/CosyVoice2-0.5B",
		Description:      "Alibaba FunAudioLLM CosyVoice 2 0.5B multilingual TTS model on SiliconFlow.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// SiliconFlowToolingDefaults notes that SiliconFlow public docs focus on model usage; no separate tool fees are published (retrieved 2026-04-28).
// Source: https://siliconflow.cn/pricing
var SiliconFlowToolingDefaults = adaptor.ChannelToolConfig{}
