package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers Opus 5.5 standard global-inference pricing, not a regional
// contract quote. AWS's model card does not support native structured outputs,
// so that first-party capability is deliberately not inherited.
// Sources (2026-09-24): https://aws.amazon.com/bedrock/pricing/ and
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
func init() {
	awsBedrockModelPricing["claude-opus-5-5"] = adaptor.ModelConfig{
		Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 8 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"max_tokens", "stop"},
		Description:                 "Claude Opus 5.5 on Amazon Bedrock; always-on adaptive thinking, 1M context and 128K output. Standard global inference $4/$20 input/output per million tokens, $0.20 cache reads, $5/$8 5-minute/1-hour cache writes. Regional or contracted prices require operator overrides. Native structured outputs are not supported on this Bedrock model card.",
	}
}
