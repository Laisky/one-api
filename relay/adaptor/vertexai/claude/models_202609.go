package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// claudeSeptember2026ModelRatios adds the Claude model IDs released on
// 2026-09-01 and applies Anthropic's permanent Sonnet 5 pricing to Vertex AI.
//
// Sources:
//   - https://platform.claude.com/docs/en/models/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
var claudeSeptember2026ModelRatios = map[string]adaptor.ModelConfig{
	"claude-fable-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
	},
	"claude-mythos-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
	},
	"claude-sonnet-5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd, CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
	},
}

func init() {
	for model, config := range claudeSeptember2026ModelRatios {
		ModelRatios[model] = config
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
