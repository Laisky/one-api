package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for DeepSeek chat models. Reused across ModelRatios
// entries to keep the table compact and consistent.
var (
	// deepseekTextInputs lists the input modalities supported by all DeepSeek chat models.
	deepseekTextInputs = []string{"text"}
	// deepseekTextOutputs lists the output modalities for DeepSeek chat completion.
	deepseekTextOutputs = []string{"text"}

	// deepseekChatFeatures advertises the capability set for non-thinking DeepSeek chat models.
	// DeepSeek chat completions support tools, JSON mode and structured outputs per the official docs.
	deepseekChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// deepseekReasoningFeatures advertises the capability set for thinking-mode DeepSeek models.
	deepseekReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// deepseekSamplingParams lists the OpenAI-compatible sampling parameters DeepSeek chat accepts.
	deepseekSamplingParams = []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "max_tokens"}
	// deepseekReasonerSamplingParams lists the restricted sampling set for the legacy reasoner model.
	// DeepSeek's reasoner endpoint historically ignored temperature/top_p; only the listed knobs apply.
	deepseekReasonerSamplingParams = []string{"max_tokens", "stop"}
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on official DeepSeek pricing: https://api-docs.deepseek.com/quick_start/pricing
// Capability metadata sources:
//   - https://api-docs.deepseek.com/quick_start/pricing
//   - https://huggingface.co/deepseek-ai/DeepSeek-V4-Flash
//   - https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro
var ModelRatios = map[string]adaptor.ModelConfig{
	// Legacy models (to be deprecated; kept for backward compatibility).
	// deepseek-chat historically pointed at DeepSeek V3 / V3.1; route now resolves to V4-Flash non-thinking.
	"deepseek-chat": {
		Ratio:                       0.28 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.42 / 0.28,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekChatFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.2",
		Description:                 "Legacy DeepSeek chat alias (DeepSeek V3.x, non-thinking mode).",
	},
	// deepseek-reasoner is the DeepSeek R1 lineage; thinking-mode chat only.
	"deepseek-reasoner": {
		Ratio:                       0.28 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.42 / 0.28,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekReasonerSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Description:                 "Legacy DeepSeek reasoning alias (DeepSeek R1 lineage, thinking mode).",
	},
	// Current models (retrieved 2026-04-24)
	// deepseek-v4-flash: $0.14/1M input (cache miss), $0.028/1M input (cache hit), $0.28/1M output, 1M context
	"deepseek-v4-flash": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Flash",
		Description:                 "DeepSeek V4 Flash MoE chat model with thinking and non-thinking modes; 1M context.",
	},
	// deepseek-v4-pro: $1.74/1M input (cache miss), $0.145/1M input (cache hit), $3.48/1M output, 1M context
	"deepseek-v4-pro": {
		Ratio:                       1.74 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.145 * ratio.MilliTokensUsd,
		CompletionRatio:             3.48 / 1.74,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Pro",
		Description:                 "DeepSeek V4 Pro MoE chat model with thinking and non-thinking modes; 1M context.",
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://api-docs.deepseek.com/quick_start/pricing
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
