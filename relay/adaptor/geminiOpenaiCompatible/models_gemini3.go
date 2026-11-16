package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gemini3ModelRatios contains complete gemini3 catalog entries.
var gemini3ModelRatios = map[string]adaptor.ModelConfig{
	"gemini-3.1-pro-preview": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  12.0 / 2.0,
		CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CachedInputRatio:    0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3ProReasoningEfforts...),
		DefaultReasoningEffort:      "high",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3.1 Pro preview multimodal reasoning tier with 1M context.",
	},
	"gemini-3.1-pro-preview-customtools": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  12.0 / 2.0,
		CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CachedInputRatio:    0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3ProReasoningEfforts...),
		DefaultReasoningEffort:      "high",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3.1 Pro preview tier configured for custom tool definitions.",
	},
	"gemini-3.1-flash-lite": {
		Ratio:                       0.25 * ratio.MilliTokensUsd,
		CompletionRatio:             1.50 / 0.25,
		CachedInputRatio:            0.025 * ratio.MilliTokensUsd,
		Audio:                       gemini31FlashLitePricing.Audio,
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3FlashLiteReasoningEfforts...),
		DefaultReasoningEffort:      "minimal",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3.1 Flash-Lite stable cost-efficient multimodal model; scheduled shutdown May 7, 2027. Use gemini-3.5-flash-lite.",
	},
	"gemini-3.1-flash-lite-preview": {
		Ratio:                       0.25 * ratio.MilliTokensUsd,
		CompletionRatio:             1.50 / 0.25,
		CachedInputRatio:            0.025 * ratio.MilliTokensUsd,
		Audio:                       gemini31FlashLitePricing.Audio,
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3FlashLiteReasoningEfforts...),
		DefaultReasoningEffort:      "minimal",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3.1 Flash-Lite preview was shut down on May 25, 2026. Use gemini-3.1-flash-lite or gemini-3.5-flash-lite.",
	},
	"gemini-3-pro-preview": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  12.0 / 2.0,
		CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CachedInputRatio:    0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), []string{"low", "high"}...),
		DefaultReasoningEffort:      "high",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3 Pro preview was shut down on March 9, 2026; the identifier now points to gemini-3.1-pro-preview. Use gemini-3.1-pro-preview.",
	},
	"gemini-3-flash-preview": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             3.00 / 0.50,
		CachedInputRatio:            0.05 * ratio.MilliTokensUsd,
		Audio:                       gemini3FlashPricing.Audio,
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3FlashReasoningEfforts...),
		DefaultReasoningEffort:      "high",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3 Flash preview multimodal reasoning model. Google recommends gemini-3.6-flash for stable production workloads; no shutdown date is announced.",
	},
	"gemini-3.5-flash": {
		Ratio:                       1.50 * ratio.MilliTokensUsd,
		CompletionRatio:             9.00 / 1.50,
		CachedInputRatio:            0.15 * ratio.MilliTokensUsd,
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3FlashReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Deprecated in the Gemini Developer API on 2026-10-08. Google automatically routes requests for this ID to gemini-3.6-flash. Use the replacement ID and its current pricing explicitly; this entry retains historical pricing for existing configurations, not a verified current tariff for redirected requests.",
	},
	"gemini-3.6-flash": geminiFlashConfig(
		"Gemini 3.6 Flash stable multimodal reasoning model optimized for rapid agentic and coding loops.",
		gemini3FlashReasoningEfforts,
	),
	"gemini-3.5-flash-lite": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             2.50 / 0.30,
		CachedInputRatio:            0.03 * ratio.MilliTokensUsd,
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), geminiFeatures25Plus...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini3FlashLiteReasoningEfforts...),
		DefaultReasoningEffort:      "minimal",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini 3.5 Flash-Lite multimodal tier listed in Google Cloud Agent Platform pricing.",
	},
	"gemini-3.8-flash": geminiFlashConfig(
		"Gemini 3.8 Flash stable model for long-horizon software engineering, autonomous agents, and complex enterprise workflows.",
		gemini37PlusReasoningEfforts,
	),
	"gemini-3.7-flash": geminiFlashConfig(
		"Deprecated in the Gemini Developer API on 2026-10-08. Google automatically routes requests for this ID to gemini-3.8-flash. Use the replacement ID and its current pricing explicitly; retaining this legacy configuration does not promise a separately served model.",
		gemini37PlusReasoningEfforts,
	),
}

const (
	geminiFlashPromotionalInputUsd  = 0.75
	geminiFlashPromotionalOutputUsd = 3.75
	geminiFlashPromotionalCacheUsd  = 0.075
	geminiFlashStandardInputUsd     = 1.50
	geminiFlashStandardOutputUsd    = 7.50
	geminiFlashStandardCacheUsd     = 0.15
	geminiOmniVideoTokensPerSecond  = 5_792
	geminiOmniVideoOutputUsd        = 17.50
)

var gemini37PlusReasoningEfforts = []string{"low", "medium", "high"}

// geminiFlashConfig builds Gemini 3.6+ Flash metadata and the documented
// promotional price transition. Parameters: description is the model summary and
// reasoningEfforts lists accepted thinking levels. Returns: a complete model configuration.
// Sampling guidance: https://ai.google.dev/gemini-api/docs/latest-model (2026-09-18).
func geminiFlashConfig(description string, reasoningEfforts []string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:            geminiFlashPromotionalInputUsd * ratio.MilliTokensUsd,
		CompletionRatio:  geminiFlashPromotionalOutputUsd / geminiFlashPromotionalInputUsd,
		CachedInputRatio: geminiFlashPromotionalCacheUsd * ratio.MilliTokensUsd,
		TimeWindows: []adaptor.TimeWindow{
			{
				Name:     "standard-pricing-from-2027",
				TimeZone: "UTC",
				DateFrom: "2027-01-01",
				Ranges: []adaptor.ClockRange{
					{Start: "00:00", End: "00:00"},
				},
				Overlay: adaptor.ModelConfig{
					Ratio:            geminiFlashStandardInputUsd * ratio.MilliTokensUsd,
					CompletionRatio:  geminiFlashStandardOutputUsd / geminiFlashStandardInputUsd,
					CachedInputRatio: geminiFlashStandardCacheUsd * ratio.MilliTokensUsd,
				},
			},
		},
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             geminiInputMultimodal,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: []string{"stop", "max_tokens"},
		SupportedReasoningEfforts:   append([]string(nil), reasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 description,
	}
}
