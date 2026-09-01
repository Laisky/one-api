package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// claudeSeptember2026ModelRatios adds the Claude model IDs released on
// 2026-09-01 and replaces Sonnet 5's superseded introductory-price window with
// Anthropic's permanent $2/$10 per million token pricing.
//
// Sources:
//   - https://platform.claude.com/docs/en/models/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
var claudeSeptember2026ModelRatios = map[string]adaptor.ModelConfig{
	"claude-fable-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		Description: "Claude Fable 5.1 flagship model with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-mythos-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		Description: "Claude Mythos 5.1 invite-only model with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-sonnet-5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd, CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		Description: "Claude Sonnet 5 balanced flagship with 1M-token context, adaptive thinking, and permanent $2/$10 per million token pricing.",
	},
}

// init installs the September 2026 Claude entries in the shared Anthropic model
// registry. It takes no parameters and returns no values.
func init() {
	for model, config := range claudeSeptember2026ModelRatios {
		ModelRatios[model] = config
	}
}
