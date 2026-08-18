package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepseekModels contains DeepSeek family models served by Fireworks. Sources:
//   - https://app.fireworks.ai/models/fireworks/deepseek-v4-pro-0813
//   - https://app.fireworks.ai/models/fireworks/deepseek-v4-flash-0731
//   - Historical per-model cards under https://app.fireworks.ai/models/{fireworks,deepseek-ai}/...
var deepseekModels = map[string]adaptor.ModelConfig{
	// Official DeepSeek V4 releases (August 2026).
	"accounts/fireworks/models/deepseek-v4-pro-0813": {
		Ratio:                       1.32 * ratio.MilliTokensUsd,
		CompletionRatio:             3.96 / 1.32,
		CachedInputRatio:            0.044 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Pro-0813",
		Description:                 "Official DeepSeek V4 Pro release (1.6T MoE) with DSpark speculative decoding, 1M-token context, stronger production agent performance, and function calling.",
	},
	"accounts/fireworks/models/deepseek-v4-flash-0731": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		CachedInputRatio:            0.028 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Flash-0731",
		Description:                 "Official DeepSeek V4 Flash release (304B MoE) with DSpark speculative decoding, 1M-token context, enhanced agentic capability, and function calling.",
	},

	// DeepSeek V4 Pro preview — $1.74 in / $3.48 out, discounted cached input listed separately.
	"accounts/fireworks/models/deepseek-v4-pro": {
		Ratio:                       1.74 * ratio.MilliTokensUsd,
		CompletionRatio:             3.48 / 1.74,
		CachedInputRatio:            0.145 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             1048576,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Pro",
		Description:                 "DeepSeek V4 Pro preview (1.6T MoE) with hybrid attention and 1M-token context. Still listed on Fireworks serverless as of 2026-08-18, but superseded by deepseek-v4-pro-0813.",
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
		Description:                 "DeepSeek V3 (671B MoE, 37B active per token) general-purpose model served by Fireworks at FP8. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
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
		Description:                 "DeepSeek V3.1 (674B MoE) with extended 128K-context post-training and UE8M0 FP8 quantization. Retired from Fireworks serverless 2026-05-14; on-demand/dedicated only; migrate to Kimi K2.6 or GLM 5.1.",
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
		Description:                 "DeepSeek V3.2 (671B MoE) tuned for high computational efficiency with superior reasoning and agent performance. Retired from Fireworks serverless 2026-05-14; on-demand/dedicated only; migrate to Kimi K2.6 or GLM 5.1.",
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
		// Fireworks expanded reasoning_effort controls to DeepSeek-R1 (low/medium/high).
		// Sources: https://docs.fireworks.ai/guides/reasoning, https://fireworks.ai/blog/deepseek-r1-deepdive
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		Quantization:              "fp8",
		HuggingFaceID:             "deepseek-ai/DeepSeek-R1-0528",
		Description:               "DeepSeek R1 05/28 reasoning checkpoint (674B MoE) approaching o3/Gemini 2.5 Pro on complex reasoning benchmarks. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
	},

	// DeepSeek V4 Flash preview — retained for dedicated/on-demand compatibility.
	"accounts/fireworks/models/deepseek-v4-flash": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		CachedInputRatio:            0.028 * ratio.MilliTokensUsd,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Flash",
		Description:                 "DeepSeek V4 Flash preview with 1M-token context. No longer listed as serverless on 2026-08-18 and superseded by deepseek-v4-flash-0731; retained for dedicated/on-demand deployments.",
	},
}
