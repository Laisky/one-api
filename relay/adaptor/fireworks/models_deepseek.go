package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepseekModels contains DeepSeek family models served by Fireworks (V3, V3.1,
// V3.2, R1-0528, V4-Pro). Capability metadata sourced from the per-model
// Fireworks cards under https://fireworks.ai/models/{fireworks,deepseek-ai}/...
var deepseekModels = map[string]adaptor.ModelConfig{
	// DeepSeek V4 Pro — $1.74 in / $3.48 out, discounted cached input listed separately.
	"accounts/fireworks/models/deepseek-v4-pro": {
		Ratio:                       1.74 * ratio.MilliTokensUsd,
		CompletionRatio:             3.48 / 1.74,
		CachedInputRatio:            0.15 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             1048576,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Pro",
		Description:                 "DeepSeek V4 Pro flagship MoE (1.6T params) with hybrid attention for 1M-token context, frontier reasoning, and advanced coding.",
	},

	// DeepSeek V3 family — $0.56 in / $1.68 out, 50% cached discount.
	"accounts/fireworks/models/deepseek-v3": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		CachedInputRatio:            0.28 * ratio.MilliTokensUsd,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Description:                 "DeepSeek V3 (671B MoE, 37B active per token) general-purpose model served by Fireworks at FP8.",
	},
	"accounts/fireworks/models/deepseek-v3p1": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		CachedInputRatio:            0.28 * ratio.MilliTokensUsd,
		ContextLength:               163840,
		MaxOutputTokens:             163840,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.1",
		Description:                 "DeepSeek V3.1 (674B MoE) with extended 128K-context post-training and UE8M0 FP8 quantization.",
	},
	"accounts/fireworks/models/deepseek-v3p2": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		CachedInputRatio:            0.28 * ratio.MilliTokensUsd,
		ContextLength:               163840,
		MaxOutputTokens:             163840,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.2",
		Description:                 "DeepSeek V3.2 (671B MoE) tuned for high computational efficiency with superior reasoning and agent performance.",
	},
	"accounts/fireworks/models/deepseek-r1-0528": {
		Ratio:                       0.56 * ratio.MilliTokensUsd,
		CompletionRatio:             1.68 / 0.56,
		CachedInputRatio:            0.28 * ratio.MilliTokensUsd,
		ContextLength:               163840,
		MaxOutputTokens:             163840,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-0528",
		Description:                 "DeepSeek R1 05/28 reasoning checkpoint (674B MoE) approaching o3/Gemini 2.5 Pro on complex reasoning benchmarks.",
	},
}
