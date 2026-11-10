package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// claudeFable52ModelRatios lists the published November 9, 2026 models and
// standard Messages tariffs. Mythos requires verified organizational access.
// Sources:
//   - https://platform.claude.com/docs/en/models/fable-5-2/overview
//   - https://platform.claude.com/docs/en/models/mythos-5-2/overview
//   - https://platform.claude.com/docs/en/models/fable-5-2/whats-new-fable-5-2
var claudeFable52ModelRatios = map[string]adaptor.ModelConfig{
	"claude-fable-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:    "high",
		Description:               "Claude Fable 5.2: 1M context, 128K output, always-on adaptive thinking (high default). Standard $8/$40 input/output, $0.20 cache reads, $10/$16 5m/1h cache writes per million tokens. Fast mode and Batch premiums/discounts are excluded.",
	},
	"claude-mythos-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts: []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:    "high",
		Description:               "Claude Mythos 5.2 for verified organizations only. 1M context, 128K output, always-on adaptive thinking (high default). Standard $8/$40 input/output, $0.20 cache reads, $10/$16 5m/1h cache writes per million tokens. Discovery does not grant access.",
	},
}

// init registers Claude Fable and Mythos 5.2 without overwriting legacy IDs.
func init() {
	for model, config := range claudeFable52ModelRatios {
		ModelRatios[model] = config
	}
}
