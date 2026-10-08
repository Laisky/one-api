package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers the published Haiku 5.5 ID and standard Messages tariffs.
// It takes no parameters and returns nothing. The upper tier applies to the
// entire prompt and output, including cache buckets, rather than just the excess.
// Sources (verified 2026-10-08):
//   - https://platform.claude.com/docs/en/models/haiku-5-5/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://platform.claude.com/docs/en/build-with-claude/effort
func init() {
	ModelRatios["claude-haiku-5-5"] = adaptor.ModelConfig{
		Ratio:             0.1 * ratio.MilliTokensUsd,
		CompletionRatio:   5,
		CachedInputRatio:  0.01 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 0.125 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.2 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{
			// Exactly 100,000 input tokens still uses the base tier.
			InputTokenThreshold: 100001,
			Ratio:               0.5 * ratio.MilliTokensUsd,
			CompletionRatio:     5,
			CachedInputRatio:    0.05 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   0.625 * ratio.MilliTokensUsd,
			CacheWrite1hRatio:   1 * ratio.MilliTokensUsd,
		}},
		ContextLength: 1000000,
		// 300K output is a Batch-only beta, not the ordinary Messages limit.
		MaxOutputTokens:             128000,
		InputModalities:             claudeVisionInputs,
		OutputModalities:            claudeTextOutputs,
		SupportedFeatures:           claudeFeaturesWithReasoning,
		SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "medium",
		Description:                 "Claude Haiku 5.5 with 1M-token context and 128K output. Adaptive thinking defaults to medium effort; thinking can be disabled at low, medium or high effort. Standard input/output costs $0.10/$0.50 per million tokens through 100K prompt tokens and $0.50/$2.50 above 100K. Forced tool choice is supported; manual thinking budgets and sampling controls are not.",
	}
}
