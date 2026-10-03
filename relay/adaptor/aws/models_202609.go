package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// awsClaudeAdaptiveOnlySamplingParams lists the parameters accepted by Claude
// models whose adaptive thinking is always enabled.
var awsClaudeAdaptiveOnlySamplingParams = []string{"stop", "max_tokens"}

// claudeSeptember2026BedrockModelPricing adds the Claude 5.1 releases and
// replaces Sonnet 5's expired introductory-price window with its permanent
// pricing.
//
// Sources:
//   - https://platform.claude.com/docs/en/models/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
var claudeSeptember2026BedrockModelPricing = map[string]adaptor.ModelConfig{
	"claude-fable-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Fable 5.1 on AWS Bedrock with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-mythos-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Mythos 5.1 invite-only model on AWS Bedrock with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-sonnet-5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd, CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Sonnet 5 on AWS Bedrock with 1M-token context, adaptive thinking, and permanent $2/$10 per million token pricing.",
	},
}

// init installs the September 2026 Claude entries in the canonical Bedrock
// pricing registry. It takes no parameters and returns no values.
func init() {
	for model, config := range claudeSeptember2026BedrockModelPricing {
		awsBedrockModelPricing[model] = config
	}
}
