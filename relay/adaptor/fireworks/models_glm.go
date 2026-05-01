package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// glmModels contains Z.ai GLM family models served by Fireworks (GLM-4.7,
// GLM-5, GLM-5.1). Sources:
//   - https://fireworks.ai/models/fireworks/glm-4p7
//   - https://fireworks.ai/models/fireworks/glm-5
//   - https://fireworks.ai/models/fireworks/glm-5p1
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
		Description:                 "Z.ai GLM-4.7 (352.8B MoE) general-purpose model with interleaved/preserved/turn-level thinking controls for long-horizon agents.",
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
		Description:                 "Z.ai GLM-5 (744B MoE, 40B active) flagship with DeepSeek Sparse Attention for long-context systems engineering and agentic tasks.",
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
		Description:                 "Z.ai GLM-5.1 (754B MoE) flagship for agentic engineering with stronger coding and sustained long-horizon multi-turn performance.",
	},
}
