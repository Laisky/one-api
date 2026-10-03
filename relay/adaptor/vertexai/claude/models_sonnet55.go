package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers Sonnet 5.5 and refreshes the child model list before the parent
// Vertex registry initializes. It takes no parameters and returns no values.
// Rates are for global, standard inference. Regional and multi-region endpoints
// carry a 10% premium and require the operator's channel pricing override.
//
// Sources (verified 2026-09-28):
//   - https://platform.claude.com/docs/en/models/sonnet-5-5/overview
//   - https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
//   - https://cloud.google.com/vertex-ai/generative-ai/pricing
func init() {
	ModelRatios["claude-sonnet-5-5"] = adaptor.ModelConfig{
		Ratio:             2 * ratio.MilliTokensUsd,
		CompletionRatio:   5,
		CachedInputRatio:  0.2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
		ContextLength:     1000000,
		MaxOutputTokens:   128000,
		InputModalities:   []string{"text", "image", "file"},
		OutputModalities:  []string{"text"},
		// Keep the common converter's feature profile; do not inherit first-party tools.
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"stop", "max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Sonnet 5.5 on Vertex AI with 1M-token context and 128K output. Adaptive thinking defaults to high effort. Forced tool choice is not supported. Global standard input/output pricing is $2/$10 per million tokens; cache reads $0.20, 5-minute writes $2.50, and 1-hour writes $4. Regional and multi-region premiums are not included.",
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
