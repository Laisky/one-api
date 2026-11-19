package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// roboticsModelRatios contains complete robotics catalog entries.
var roboticsModelRatios = map[string]adaptor.ModelConfig{
	"gemini-robotics-er-1.5-preview": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5 / 0.30,
		Audio:                       geminiRoboticsER15Pricing.Audio,
		ContextLength:               1_000_000,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode", "structured_outputs", "reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini25ReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini25FlashMaxThinkingBudget,
		Description:                 "Gemini Robotics-ER 1.5 preview tier for embodied reasoning workloads. (RETIRED 2026-04-30; use gemini-robotics-er-1.6-preview)",
	},
	"gemini-robotics-er-1.6-preview": {
		Ratio:                       1.00 * ratio.MilliTokensUsd,
		CompletionRatio:             5.00 / 1.00,
		Audio:                       geminiRoboticsER16Pricing.Audio,
		ContextLength:               1_000_000,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedFeatures:           append([]string(nil), []string{"tools", "json_mode", "structured_outputs", "reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingChat...),
		SupportedReasoningEfforts:   append([]string(nil), gemini25ReasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini25FlashMaxThinkingBudget,
		Description:                 "Gemini Robotics ER 1.6 preview was shut down August 31, 2026. Use gemini-robotics-er-2-preview or gemini-robotics-er-2-streaming-preview.",
	},
	"gemini-robotics-er-2-preview":           geminiRoboticsConfig(false),
	"gemini-robotics-er-2-streaming-preview": geminiRoboticsConfig(true),
}

// geminiRoboticsConfig builds Gemini Robotics ER 2 metadata.
// Prices verified 2026-09-30: https://ai.google.dev/gemini-api/docs/pricing.
// Parameters: streaming selects the Live API endpoint. Returns: a model configuration.
func geminiRoboticsConfig(streaming bool) adaptor.ModelConfig {
	config := adaptor.ModelConfig{
		Ratio:            1.00 * ratio.MilliTokensUsd,
		CompletionRatio:  5.00 / 1.00,
		TimeWindows:      []adaptor.TimeWindow{geminiStandardPriceWindow(2.00, 10.00, 0)},
		ContextLength:    131_072,
		MaxOutputTokens:  gemini3FlashMaxOutput,
		InputModalities:  geminiInputMultimodal,
		OutputModalities: geminiOutputText,
		SupportedFeatures: []string{
			"tools", "json_mode", "structured_outputs", "web_search", "reasoning",
		},
		SupportedSamplingParameters: geminiSamplingChat,
		SupportedReasoningEfforts:   gemini37PlusReasoningEfforts,
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 "Gemini Robotics ER 2 preview for embodied reasoning, video progress understanding, spatial reasoning, and multi-robot orchestration.",
	}
	if streaming {
		config.SupportedFeatures = []string{"tools", "web_search", "reasoning"}
		config.Description = "Gemini Robotics ER 2 Streaming preview for low-latency bidirectional audio and video robotics agents over the Live API."
		return config
	}
	// The REST Robotics ER 2 model defaults to high and also supports minimal;
	// the distinct Live-only streaming preview retains its separate settings.
	// Source: https://ai.google.dev/gemini-api/docs/thinking (2026-11-01).
	config.SupportedReasoningEfforts = append([]string(nil), gemini3FlashReasoningEfforts...)
	config.DefaultReasoningEffort = "high"
	config.CachedInputRatio = 0.10 * ratio.MilliTokensUsd
	config.TimeWindows[0].Overlay.CachedInputRatio = 0.20 * ratio.MilliTokensUsd
	return config
}
