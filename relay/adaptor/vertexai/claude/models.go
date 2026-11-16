package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Catalog sources (existing verified contracts):
// - https://cloud.google.com/vertex-ai/generative-ai/pricing
// - https://platform.claude.com/docs/en/models/overview
// - https://platform.claude.com/docs/en/about-claude/pricing
// - https://platform.claude.com/docs/en/models/fable-5-2/overview
// - https://platform.claude.com/docs/en/models/mythos-5-2/overview
// - https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
// - https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
// - https://platform.claude.com/docs/en/models/opus-5-5/overview
// - https://platform.claude.com/docs/en/models/sonnet-5-5/overview

// ModelRatios contains complete Vertex Claude defaults and reference metadata.
var ModelRatios = map[string]adaptor.ModelConfig{
	"claude-3-haiku@20240307":       {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.025 * ratio.MilliTokensUsd, CacheWrite5mRatio: 0.3125 * ratio.MilliTokensUsd, CacheWrite1hRatio: 0.5 * ratio.MilliTokensUsd},
	"claude-3-5-haiku@20241022":     {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},
	"claude-haiku-4-5@20251001":     {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd},
	"claude-3-sonnet@20240229":      {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-5-sonnet@20240620":    {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-5-sonnet-v2@20241022": {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-7-sonnet@20250219":    {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4@20250514":      {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-5@20250929":    {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-6":             {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd, CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
	},
	"claude-3-opus@20240229":   {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4@20250514":   {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-1@20250805": {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-5@20251101": {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-6":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-7":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-8":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-5":            {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-fable-5":           {Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5, CachedInputRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd},
	"claude-fable-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
	},
	"claude-mythos-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
	},
	"claude-haiku-5-5": {
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
	},
	"claude-sonnet-5-5": {
		Ratio:             2 * ratio.MilliTokensUsd,
		CompletionRatio:   5,
		CachedInputRatio:  0.1 * ratio.MilliTokensUsd,
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
		Description:                 "Claude Sonnet 5.5 on Vertex AI with 1M-token context and 128K output. Adaptive thinking defaults to high effort. Forced tool choice is not supported. Global standard input/output pricing is $2/$10 per million tokens; cache reads $0.10, 5-minute writes $2.50, and 1-hour writes $4. Regional and multi-region premiums are not included.",
	},
	"claude-fable-5-2":  vertexClaudeReferenceConfig("claude-fable-5-2", "Claude Fable 5.2 on Vertex AI with 1M context, 128K output and always-on adaptive thinking. Requires explicit operator-verified Google endpoint token and cache tariffs; reference metadata is not a verified Google price."),
	"claude-mythos-5-2": vertexClaudeReferenceConfig("claude-mythos-5-2", "Claude Mythos 5.2 on Vertex AI for verified organizations only. Requires explicit operator-verified Google endpoint token and cache tariffs; reference metadata is not a verified Google price. Entitlement is separate."),
	"claude-opus-5-5":   vertexClaudeReferenceConfig("claude-opus-5-5", "Claude Opus 5.5 on Vertex AI with 1M-token context, 128K output, and always-on adaptive thinking (default effort: medium). Global list input/output pricing is $4/$20 per million tokens; cache reads are $0.20 per million tokens. Regional premiums are not included."),
}

// ModelList derives discovery after the complete Claude catalog is assembled.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// vertexClaudeReferenceConfig returns native Claude reference limits and prices
// for name with Vertex's feature profile and description. It does not establish
// a Google tariff or advertise Anthropic-only hosted tools.
func vertexClaudeReferenceConfig(name, description string) adaptor.ModelConfig {
	config := anthropic.ModelRatios[name]
	config.SupportedFeatures = []string{"tools", "reasoning"}
	config.Description = description
	return config
}
