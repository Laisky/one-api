package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt6AstraReasoningEfforts is the complete reasoning-effort ladder accepted by
// GPT-6 Astra. Unlike GPT-5.6, Astra does not support "none" or its legacy
// "minimal" alias.
var gpt6AstraReasoningEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// gpt6SolLunaReasoningEfforts includes the non-reasoning mode supported by Sol
// and Luna, in addition to the GPT-6 reasoning-effort ladder.
var gpt6SolLunaReasoningEfforts = []string{"none", "low", "medium", "high", "xhigh", "max"}

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

// gpt6ModelRatios captures Standard pricing and metadata for the GPT-6 family.
// Sol and Luna were released on 2026-09-22. Only published model IDs are listed;
// account provisioning remains the channel administrator's responsibility.
//
// Sources verified 2026-09-22:
//   - https://developers.openai.com/api/docs/models/gpt-6-astra
//   - https://developers.openai.com/api/docs/models/gpt-6-sol
//   - https://developers.openai.com/api/docs/models/gpt-6-luna
//   - https://developers.openai.com/api/docs/pricing
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
	"gpt-6-sol": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   10.0 / 2.0,
		CachedInputRatio:  0.2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               4.0 * ratio.MilliTokensUsd,
			CompletionRatio:     15.0 / 4.0,
			CachedInputRatio:    0.4 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   5.0 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
		}},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt6SolLunaReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-6 Sol: coding and agentic reasoning model. Use Responses for reasoning with tools; Chat Completions function calling requires reasoning_effort=none.",
	},
	"gpt-6-luna": {
		Ratio:             0.1 * ratio.MilliTokensUsd,
		CompletionRatio:   0.5 / 0.1,
		CachedInputRatio:  0.01 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 0.125 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               0.2 * ratio.MilliTokensUsd,
			CompletionRatio:     0.75 / 0.2,
			CachedInputRatio:    0.02 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   0.25 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
		}},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt6SolLunaReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-6 Luna: efficient reasoning model for focused, high-volume tasks. Use Responses for reasoning with tools; Chat Completions function calling requires reasoning_effort=none.",
	},
}
