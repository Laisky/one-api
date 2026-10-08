package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers the published, undated Sonnet 5.5 model ID. It takes no
// parameters and returns no values. Standard Messages pricing excludes Batch
// discounts, data-residency premiums, and account-specific contracts.
//
// Sources (verified 2026-10-08):
//   - https://platform.claude.com/docs/en/models/sonnet-5-5/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide
func init() {
	ModelRatios["claude-sonnet-5-5"] = adaptor.ModelConfig{
		Ratio:             2 * ratio.MilliTokensUsd,
		CompletionRatio:   5,
		CachedInputRatio:  0.1 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
		ContextLength:     1000000,
		// The 300K output limit requires the Batch-only beta, not ordinary Messages.
		MaxOutputTokens:             128000,
		InputModalities:             claudeVisionInputs,
		OutputModalities:            claudeTextOutputs,
		SupportedFeatures:           claudeFeaturesWithReasoning,
		SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Sonnet 5.5 with 1M-token context and 128K output. Adaptive thinking defaults to high effort. Forced tool choice is not supported. Standard input/output pricing is $2/$10 per million tokens; cache reads cost $0.10, 5-minute writes $2.50, and 1-hour writes $4 per million tokens.",
	}
}
