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
//
// Additional cards checked 2026-09-24 through 2026-11-16 against
// https://docs.fireworks.ai/serverless/pricing
// https://fireworks.ai/models/fireworks/ember-1
// https://fireworks.ai/models/fireworks/muse-spark-open
// https://fireworks.ai/models/fireworks/ultramarine-1-flash-us
var frontierModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/ember-1": {
		Ratio: 3 * ratio.MilliTokensUsd, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 15 / 3,
		ContextLength:   1048576,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
		Description:                 "Fireworks Ember-1, a Kimi K3-based vision and reasoning model released 2026-09-22. Uses Fireworks serverless rates, not dedicated GPU pricing.",
		HuggingFaceID:               "",
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
	"accounts/fireworks/models/muse-spark-open": {
		Ratio:                       1.25 * ratio.MilliTokensUsd,
		CachedInputRatio:            .15 * ratio.MilliTokensUsd,
		CompletionRatio:             fireworksOutputRatio(4.25, 1.25),
		ContextLength:               1_048_576,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice"},
		Description:                 "Fireworks-hosted Muse Spark Open with text/image input and 1M context. US-only capacity uses the published Standard tariff because the normal US premium is explicitly waived." + " Maximum output and hosted precision are unspecified. Priority, Fast, Ultra-fast and deployment-specific charges are separate operator configuration.",
		SupportedFeatures:           []string{"tools", "reasoning"},
	},
	"accounts/fireworks/models/ultramarine-1-flash-us": {
		Ratio:                       .15 * ratio.MilliTokensUsd,
		CachedInputRatio:            .03 * ratio.MilliTokensUsd,
		CompletionRatio:             fireworksOutputRatio(.50, .15),
		ContextLength:               0,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop"},
		Description:                 "Fireworks-hosted Ultramarine 1 Flash LoRA with text/image input and 1M context. On 2026-11-16 both the model card and central table agree on 0.15/0.03/0.50 USD/M input/cache/output. The current card describes global capacity despite the -us ID and explicitly does not support function calling; no base-GLM tool or effort metadata is inherited." + " Maximum output and hosted precision are unspecified. Priority, Fast, Ultra-fast and deployment-specific charges are separate operator configuration." + " The card publishes an approximate 1M context; the exact token ceiling is unconfirmed.",
	},
}
