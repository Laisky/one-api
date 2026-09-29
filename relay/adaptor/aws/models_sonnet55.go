package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers Bedrock Sonnet 5.5 with Claude-equivalent default prices,
// explicitly approved by the repository owner on 2026-09-28. It takes no
// arguments and returns nothing. Account-specific tariffs remain configurable.
// Sources (2026-09-28):
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
//   - https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5#pricing
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://aws.amazon.com/bedrock/pricing/
func init() {
	awsBedrockModelPricing["claude-sonnet-5-5"] = adaptor.ModelConfig{
		Ratio:                       2 * ratio.MilliTokensUsd,
		CompletionRatio:             5,
		CachedInputRatio:            0.2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           2.5 * ratio.MilliTokensUsd,
		CacheWrite1hRatio:           4 * ratio.MilliTokensUsd,
		ContextLength:               1000000,
		MaxOutputTokens:             128000,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"stop", "max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Sonnet 5.5 on Amazon Bedrock: 1M context, 128K output, adaptive thinking (default high). Commercial-region Invoke calls use a global inference profile. Native structured outputs and Batch are not supported by the launch model card. Default input/output prices are $2/$10 per million tokens, cache read $0.20 and 5m/1h writes $2.50/$4, following the owner-approved Claude price-parity policy. Channel price overrides remain supported.",
	}
}
