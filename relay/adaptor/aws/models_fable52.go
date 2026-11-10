package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// awsFable52ModelPricing registers standard global tariff defaults. AWS
// account/region-specific prices still require explicit channel overrides.
// Sources:
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html
//   - https://platform.claude.com/docs/en/models/fable-5-2/whats-new-fable-5-2
var awsFable52ModelPricing = map[string]adaptor.ModelConfig{
	"claude-fable-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Fable 5.2 on Bedrock: 1M context, 128K output, always-on adaptive thinking; standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens. Requires cross-region profile and AWS data-retention opt-in.",
	},
	"claude-mythos-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Mythos 5.2 on Bedrock: restricted access, 1M context, 128K output, always-on adaptive thinking; standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens. Requires a supported inference profile.",
	},
}

// init adds Bedrock Claude 5.2 prices without changing earlier model defaults.
func init() {
	for model, config := range awsFable52ModelPricing {
		awsBedrockModelPricing[model] = config
	}
}
