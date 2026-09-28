package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers Sonnet 5.5's standard launch reference prices and Bedrock's
// independently documented capabilities. It takes no arguments and returns nothing.
// The prices follow Anthropic's explicit Sonnet 5 price-parity announcement and
// the existing Bedrock Sonnet 5 defaults, not a separately retrieved AWS tariff.
// Confirm the AWS Marketplace agreement or configure channel overrides before
// deployment; do not treat these defaults as a verified account-specific quote.
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
		Description:                 "Claude Sonnet 5.5 on Amazon Bedrock: 1M context, 128K output, adaptive thinking (default high). Commercial-region Invoke calls use a global inference profile. Native structured outputs and Batch are not supported by the launch model card. Reference input/output prices are $2/$10 per million tokens, cache read $0.20 and 5m/1h writes $2.50/$4, following the published Sonnet 5 price parity; independently confirm the AWS tariff or configure channel overrides before deployment.",
	}
}
