package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Catalog sources (existing verified contracts):
// - https://developers.openai.com/api/docs/pricing
// - https://developers.openai.com/api/docs/models/gpt-5.5
// - https://developers.openai.com/api/docs/models/gpt-5.4
// - https://developers.openai.com/api/docs/models/gpt-5.2
// - https://developers.openai.com/api/docs/models/gpt-5.1
// - https://developers.openai.com/api/docs/models/gpt-5
// - https://developers.openai.com/api/docs/guides/latest-model
// - https://developers.openai.com/api/docs/models/gpt-5.6-sol
// - https://developers.openai.com/api/docs/models/chat-latest
// - https://developers.openai.com/api/docs/models/gpt-transcribe

var gpt56CanonicalReasoningEfforts = []string{"none", "low", "medium", "high", "xhigh", "max"}

var gpt56SolPromoLongContextTier = adaptor.ModelRatioTier{
	Ratio:               8.0 * ratio.MilliTokensUsd,
	CompletionRatio:     30.0 / 8.0,
	CachedInputRatio:    0.8 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   10.0 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

var gpt56CyberLongContextTier = adaptor.ModelRatioTier{
	Ratio:               25.0 * ratio.MilliTokensUsd,
	CompletionRatio:     112.5 / 25.0,
	CachedInputRatio:    2.5 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   31.25 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

var gpt56TerraLongContextTier = adaptor.ModelRatioTier{
	Ratio:               4.0 * ratio.MilliTokensUsd,
	CompletionRatio:     18.0 / 4.0,
	CachedInputRatio:    0.4 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   5.0 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

var gpt56LunaLongContextTier = adaptor.ModelRatioTier{
	Ratio:               0.4 * ratio.MilliTokensUsd,
	CompletionRatio:     1.8 / 0.4,
	CachedInputRatio:    0.04 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   0.5 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

// gpt56ModelRatios contains complete GPT-5.6 and Daybreak alias definitions.
var gpt56ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-5.6":     newGPT56SolConfig("GPT-5.6 Sol: frontier reasoning model with 1.05M context and promotional API pricing."),
	"gpt-5.6-sol": newGPT56SolConfig("GPT-5.6 Sol: frontier reasoning model with 1.05M context and promotional API pricing."),
	"gpt-5.6-terra": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             12.0 / 2.0,
		CachedInputRatio:            0.2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           2.5 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt56TerraLongContextTier},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt56CanonicalReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-5.6 Terra: balanced 5.6 reasoning tier with 1.05M context (long-context surcharge >272K input).",
	},
	"gpt-5.6-luna": {
		Ratio:                       0.2 * ratio.MilliTokensUsd,
		CompletionRatio:             1.2 / 0.2,
		CachedInputRatio:            0.02 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           0.25 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt56LunaLongContextTier},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt56CanonicalReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-5.6 Luna: cost-efficient 5.6 reasoning tier with 1.05M context (long-context surcharge >272K input).",
	},
	"gpt-5.6-cyber":            newGPT56CyberConfig("GPT-5.6 Cyber: specialized cybersecurity reasoning model with separately provisioned access."),
	"gpt-daybreak-red-latest":  newGPT56CyberConfig("GPT Daybreak Red: alias for gpt-5.6-cyber; separate approval and provisioning required."),
	"gpt-daybreak-blue-latest": newGPT56SolConfig("GPT Daybreak Blue: alias for gpt-5.6-sol; separate approval and provisioning required."),
}

// newGPT56SolConfig returns independently owned Sol defaults for description.
// It preserves the verified promotional tariff and its long-input tier.
func newGPT56SolConfig(description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                       4.0 * ratio.MilliTokensUsd,
		CompletionRatio:             20.0 / 4.0,
		CachedInputRatio:            0.4 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           5.0 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt56SolPromoLongContextTier},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt56CanonicalReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 description,
	}
}

// newGPT56CyberConfig returns Cyber defaults with the supplied description.
// Each alias owns its slices and the specialized long-input write tariff.
func newGPT56CyberConfig(description string) adaptor.ModelConfig {
	config := newGPT56SolConfig(description)
	config.Ratio = 12.5 * ratio.MilliTokensUsd
	config.CompletionRatio = 75.0 / 12.5
	config.CachedInputRatio = 1.25 * ratio.MilliTokensUsd
	config.CacheWrite5mRatio = 15.625 * ratio.MilliTokensUsd
	config.Tiers = []adaptor.ModelRatioTier{gpt56CyberLongContextTier}
	config.ContextLength = 400_000
	return config
}
