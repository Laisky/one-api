package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers the official undated Haiku 5.5 publisher ID and independently
// verified global Standard tariffs, then rebuilds discovery before the parent
// registry initializes. It accepts no arguments and returns nothing.
// Sources checked 2026-10-12:
// https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
// https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
func init() {
	ModelRatios["claude-haiku-5-5"] = adaptor.ModelConfig{
		Ratio: .1 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .01 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: .125 * ratio.MilliTokensUsd, CacheWrite1hRatio: .2 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{InputTokenThreshold: 100001,
			Ratio: .5 * ratio.MilliTokensUsd, CompletionRatio: 5, CachedInputRatio: .05 * ratio.MilliTokensUsd,
			CacheWrite5mRatio: .625 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1 * ratio.MilliTokensUsd}},
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"stop", "max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"}, DefaultReasoningEffort: "medium",
		Description: "Claude Haiku 5.5 on Vertex AI, global Standard inference. 1M context, 128K output and medium default effort. All prompt buckets count toward the 100K tier threshold. Input/output prices are USD 0.10/0.50 per million through 100K and 0.50/2.50 above it. Regional and multi-region prices carry a separately configured 10% premium; Batch rates and account entitlements are not inferred.",
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
