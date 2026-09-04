package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// glmModels contains Z.ai GLM family models served by Fireworks (GLM-4.7,
// GLM-5, GLM-5.1, GLM-5.2, GLM-5.3, and GLM-5.3-Flash). Sources:
//   - https://fireworks.ai/models/fireworks/glm-4p7
//   - https://fireworks.ai/models/fireworks/glm-5
//   - https://fireworks.ai/models/fireworks/glm-5p1
//   - https://fireworks.ai/models/fireworks/glm-5p2
//   - https://fireworks.ai/models/fireworks/glm-5p3
//   - https://fireworks.ai/models/fireworks/glm-5p3-flash
var glmModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/glm-4p7": {
		Ratio:                       0.60 * ratio.MilliTokensUsd,
		CompletionRatio:             2.20 / 0.60,
		ContextLength:               202752,
		MaxOutputTokens:             202752,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "zai-org/GLM-4.7",
		Description:                 "Z.ai GLM-4.7 (352.8B MoE) general-purpose model with interleaved/preserved/turn-level thinking controls for long-horizon agents. Retired from Fireworks serverless 2026-05-14; on-demand/dedicated only; migrate to GLM 5.1.",
	},
	"accounts/fireworks/models/glm-5": {
		Ratio:                       1.00 * ratio.MilliTokensUsd,
		CompletionRatio:             3.20 / 1.00,
		CachedInputRatio:            0.20 * ratio.MilliTokensUsd,
		ContextLength:               202752,
		MaxOutputTokens:             202752,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "zai-org/GLM-5",
		Description:                 "Z.ai GLM-5 (744B MoE, 40B active) flagship with DeepSeek Sparse Attention for long-context systems engineering and agentic tasks. Retired from Fireworks serverless 2026-05-14; on-demand/dedicated only; migrate to GLM 5.1.",
	},
	"accounts/fireworks/models/glm-5p1": {
		Ratio:                       1.40 * ratio.MilliTokensUsd,
		CompletionRatio:             4.40 / 1.40,
		CachedInputRatio:            0.26 * ratio.MilliTokensUsd,
		ContextLength:               202752,
		MaxOutputTokens:             202752,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "zai-org/GLM-5.1-FP8",
		Description:                 "Z.ai GLM-5.1 (754B MoE) for agentic engineering and sustained long-horizon work. No longer listed as serverless; retained for dedicated/on-demand deployments and superseded by newer GLM releases.",
	},
	"accounts/fireworks/models/glm-5p2": {
		Ratio:                       1.40 * ratio.MilliTokensUsd,
		CompletionRatio:             4.40 / 1.40,
		CachedInputRatio:            0.14 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "zai-org/GLM-5.2",
		Description:                 "Z.ai GLM-5.2 (743B MoE) flagship with 1M-token context for long-horizon agentic engineering and coding.",
	},
	"accounts/fireworks/models/glm-5p3": {
		Ratio:                       1.40 * ratio.MilliTokensUsd,
		CompletionRatio:             4.40 / 1.40,
		CachedInputRatio:            0.26 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "zai-org/GLM-5.3",
		Description:                 "Z.ai GLM-5.3 (743B MoE) improves complex coding and long-horizon agent work through post-training while retaining the GLM-5.2 base architecture and 1M-token context.",
	},
	"accounts/fireworks/models/glm-5p3-flash": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             0.50 / 0.15,
		CachedInputRatio:            0.03 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "zai-org/GLM-5.3-Flash",
		Description:                 "Z.ai GLM-5.3-Flash is a 320B native multimodal model with 18B active parameters, 1M-token context, function calling, and lower-cost long-context inference.",
	},
}
