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
// and Luna (as well as GPT-6.1 Terra), in addition to the GPT-6 effort ladder.
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

// gpt6CyberLongContextTier is GPT-6 Cyber's full-request tier above 272K input
// tokens. OpenAI publishes its input, output and cache-read prices but no
// long-context cache-write price, so the field is deliberately omitted and
// modelBillingPolicies marks such cache writes unpriced instead of letting the
// tier resolver settle the inherited short-context rate.
// https://developers.openai.com/api/docs/pricing.md
var gpt6CyberLongContextTier = adaptor.ModelRatioTier{
	Ratio:               40 * ratio.MilliTokensUsd,
	CompletionRatio:     150.0 / 40.0,
	CachedInputRatio:    4 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

// gpt6ModelRatios captures Standard pricing and metadata for the GPT-6 family.
// Sol and Luna were released on 2026-09-22; Terra was released 2026-10-26.
// Only published model IDs are listed;
// account provisioning remains the channel administrator's responsibility.
//
// Sources verified through 2026-11-22:
//   - https://developers.openai.com/api/docs/models/gpt-6.2-astra
//   - https://developers.openai.com/api/docs/models/gpt-6.2-sol
//   - https://developers.openai.com/api/docs/guides/deployment-checklist
//   - https://developers.openai.com/api/docs/models/gpt-6-astra
//   - https://developers.openai.com/api/docs/models/gpt-6-sol
//   - https://developers.openai.com/api/docs/models/gpt-6-luna
//   - https://developers.openai.com/api/docs/models/gpt-6.1-terra
//   - https://developers.openai.com/api/docs/pricing
//   - https://developers.openai.com/api/docs/guides/latest-model
//
// Cyber source verified 2026-11-16: https://developers.openai.com/api/docs/models/gpt-6-cyber
var gpt6ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-6.2-astra": {
		Ratio:             8.0 * ratio.MilliTokensUsd,
		CompletionRatio:   40.0 / 8.0,
		CachedInputRatio:  0.4 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10.0 * ratio.MilliTokensUsd, // Legacy field name; OpenAI uses 30m.
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               16.0 * ratio.MilliTokensUsd,
			CompletionRatio:     60.0 / 16.0,
			CachedInputRatio:    0.8 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   20.0 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
		}},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning", "streaming", "tools", "structured_outputs", "web_search", "prompt_caching"},
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "medium",
		Description: "GPT-6.2 Astra: reasoning for complex coding and professional work. Maximum input is 922,000 tokens; tools require Responses. " +
			"Standard HTTP pricing with 30m cache writes; inputs over 272K use the full-request long-context tier. 12h caching and other service tiers are not supported by this tariff.",
	},
	"gpt-6.2-sol": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   10.0 / 2.0,
		CachedInputRatio:  0.1 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, // Legacy field name; OpenAI uses 30m.
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               4.0 * ratio.MilliTokensUsd,
			CompletionRatio:     15.0 / 4.0,
			CachedInputRatio:    0.2 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   5.0 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
		}},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning", "streaming", "tools", "structured_outputs", "web_search", "prompt_caching"},
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "medium",
		Description: "GPT-6.2 Sol: reasoning for everyday coding and agent workflows. Maximum input is 922,000 tokens; tools require Responses. " +
			"Standard HTTP pricing with 30m cache writes; inputs over 272K use the full-request long-context tier. 12h caching and other service tiers are not supported by this tariff.",
	},
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
	"gpt-6.1-sol": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   10.0 / 2.0,
		CachedInputRatio:  0.1 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               4.0 * ratio.MilliTokensUsd,
			CompletionRatio:     15.0 / 4.0,
			CachedInputRatio:    0.2 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   5.0 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
		}},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-6.1 Sol: reasoning model for coding, computer use, and professional work. Tool calling requires Responses; Chat Completions supports text without tools. Neither none nor minimal reasoning is supported.",
	},
	"gpt-6.1-terra": {
		Ratio:             1.0 * ratio.MilliTokensUsd,
		CompletionRatio:   5.0 / 1.0,
		CachedInputRatio:  0.05 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               2.0 * ratio.MilliTokensUsd,
			CompletionRatio:     7.5 / 2.0,
			CachedInputRatio:    0.1 * ratio.MilliTokensUsd,
			CacheWrite5mRatio:   2.5 * ratio.MilliTokensUsd,
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
		Description:                 "GPT-6.1 Terra: efficient visual reasoning and computer-use model. Tool calling requires Responses. Standard cache-read is 5% of input; inputs over 272K tokens incur the full-request long-context tier.",
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
	"gpt-6-cyber": {
		Ratio:             20 * ratio.MilliTokensUsd,
		CompletionRatio:   100.0 / 20.0,
		CachedInputRatio:  2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 25 * ratio.MilliTokensUsd, // Legacy field name; OpenAI uses 30m.
		Tiers:             []adaptor.ModelRatioTier{gpt6CyberLongContextTier},
		ContextLength:     1_050_000,
		MaxOutputTokens:   128_000,
		InputModalities:   []string{"text", "image"},
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{
			"reasoning", "streaming", "tools", "structured_outputs", "web_search", "prompt_caching",
		},
		Description: "GPT-6 Cyber: reasoning for authorized security research; requires upstream Daybreak Red approval. Standard pricing; inputs over 272K use the long-context tier, where OpenAI publishes no cache-write price, so implicit prompt caching is disabled there unless the channel configures a long-context cache-write tier. 12h cache retention is not supported.",
	},
}
