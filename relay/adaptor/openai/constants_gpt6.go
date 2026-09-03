package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt6AstraReasoningEfforts is the complete reasoning-effort ladder accepted by
// GPT-6 Astra. Unlike GPT-5.6, Astra does not support "none" or its legacy
// "minimal" alias.
var gpt6AstraReasoningEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// gpt6AstraLongContextTier represents the surcharge applied when a GPT-6 Astra
// request contains more than 272K input tokens. OpenAI bills the full request at
// 2x input and cache rates and 1.5x output in this tier.
var gpt6AstraLongContextTier = adaptor.ModelRatioTier{
	Ratio:               20.0 * ratio.MilliTokensUsd,
	CompletionRatio:     75.0 / 20.0,
	CachedInputRatio:    2.0 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   25.0 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

// gpt6ModelRatios captures pricing and metadata for the GPT-6 family.
//
// Sources verified 2026-09-03:
//   - https://developers.openai.com/api/docs/models/gpt-6-astra
//   - https://developers.openai.com/api/docs/guides/latest-model
var gpt6ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-6-astra": {
		Ratio:                       10.0 * ratio.MilliTokensUsd,
		CompletionRatio:             50.0 / 10.0,
		CachedInputRatio:            1.0 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           12.5 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt6AstraLongContextTier},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   gpt6AstraReasoningEfforts,
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-6 Astra: flagship reasoning model for complex end-to-end work with cache-write and long-context billing.",
	},
}
