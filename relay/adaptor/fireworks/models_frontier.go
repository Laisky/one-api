package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// frontierModels contains current Fireworks serverless models that do not fit
// the provider-specific family files.
// Sources:
//   - https://app.fireworks.ai/models/fireworks/muse-glimmer-30b
//   - https://app.fireworks.ai/models/fireworks/inkling
var frontierModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/muse-glimmer-30b": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             1.50 / 0.35,
		CachedInputRatio:            0.04 * ratio.MilliTokensUsd,
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "meta-models/Muse-Glimmer-30B",
		Description:                 "Meta Muse Glimmer 30B dense multimodal agent model with 131K context, schema-based tool calling, failure recovery, and selectable reasoning strength.",
	},
	"accounts/fireworks/models/inkling": {
		Ratio:                       1.00 * ratio.MilliTokensUsd,
		CompletionRatio:             4.05 / 1.00,
		CachedInputRatio:            0.17 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "thinkingmachines/Inkling",
		Description:                 "Thinking Machines Inkling 975B MoE with 41B active parameters, 1M-token context, image input, function calling, and controllable thinking effort.",
	},
}
