package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers Claude Opus 5.5, released on 2026-09-22. It takes no
// parameters and returns no values. Existing model IDs and prices are retained.
//
// Sources (verified 2026-09-22):
//   - https://platform.claude.com/docs/en/models/opus-5-5/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://platform.claude.com/docs/en/models/opus-5-5/migration-guide
func init() {
	ModelRatios["claude-opus-5-5"] = adaptor.ModelConfig{
		Ratio:             4 * ratio.MilliTokensUsd,
		CompletionRatio:   5,
		CachedInputRatio:  0.2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 5 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 8 * ratio.MilliTokensUsd,
		ContextLength:     1000000,
		// The 300K output beta is Batch-only, not the ordinary Messages limit.
		MaxOutputTokens:             128000,
		InputModalities:             claudeVisionInputs,
		OutputModalities:            claudeTextOutputs,
		SupportedFeatures:           claudeFeaturesWithReasoning,
		SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		Description:                 "Claude Opus 5.5 for agentic coding and knowledge work with 1M-token context, 128K output, and always-on adaptive thinking (default effort: medium). Standard input/output pricing is $4/$20 per million tokens; cache reads are $0.20 per million tokens (5% of input pricing).",
	}
}
