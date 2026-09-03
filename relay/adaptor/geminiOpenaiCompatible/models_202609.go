package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

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

// geminiSeptember2026FlashConfig builds Gemini 3.6+ Flash metadata and the documented
// promotional price transition. Parameters: description is the model summary and
// reasoningEfforts lists accepted thinking levels. Returns: a complete model configuration.
func geminiSeptember2026FlashConfig(description string, reasoningEfforts []string) adaptor.ModelConfig {
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
		SupportedSamplingParameters: geminiSamplingChat,
		SupportedReasoningEfforts:   append([]string(nil), reasoningEfforts...),
		DefaultReasoningEffort:      "medium",
		MaxReasoningTokens:          gemini3LevelMaxThinkingBudget,
		Description:                 description,
	}
}

// geminiSeptember2026TranscribeConfig builds the pricing and modality metadata for
// a Gemini 3.5 transcription endpoint. Parameters: inputUsd and outputUsd are paid-tier
// prices per million tokens, and description summarizes the endpoint. Returns: a model configuration.
func geminiSeptember2026TranscribeConfig(inputUsd float64, outputUsd float64, description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           inputUsd * ratio.MilliTokensUsd,
		CompletionRatio: outputUsd / inputUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           1,
			PromptTokensPerSecond: 25,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: geminiOutputText,
		Description:      description,
	}
}

// geminiSeptember2026OmniConfig builds the shared Gemini Omni Flash pricing and
// video metadata. Parameters: description summarizes lifecycle status. Returns: a model configuration.
func geminiSeptember2026OmniConfig(description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           1.50 * ratio.MilliTokensUsd,
		CompletionRatio: 9.00 / 1.50,
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   geminiOmniVideoOutputUsd * geminiOmniVideoTokensPerSecond / 1_000_000,
			BaseResolution: "1280x720",
		},
		ContextLength:    gemini1MContext,
		InputModalities:  []string{"text", "image", "video"},
		OutputModalities: []string{"video"},
		Description:      description,
	}
}

// geminiSeptember2026RoboticsConfig builds Gemini Robotics ER 2 metadata.
// Parameters: streaming selects the Live API endpoint. Returns: a model configuration.
func geminiSeptember2026RoboticsConfig(streaming bool) adaptor.ModelConfig {
	config := adaptor.ModelConfig{
		Ratio:            2.00 * ratio.MilliTokensUsd,
		CompletionRatio:  10.00 / 2.00,
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
	config.CachedInputRatio = 0.20 * ratio.MilliTokensUsd
	return config
}

var geminiSeptember2026ModelRatios = map[string]adaptor.ModelConfig{
	"gemini-3.8-flash": geminiSeptember2026FlashConfig(
		"Gemini 3.8 Flash stable model for long-horizon software engineering, autonomous agents, and complex enterprise workflows.",
		gemini37PlusReasoningEfforts,
	),
	"gemini-3.7-flash": geminiSeptember2026FlashConfig(
		"Gemini 3.7 Flash stable model for coding, agentic tool use, and reliable multi-step execution.",
		gemini37PlusReasoningEfforts,
	),
	"gemini-3.6-flash": geminiSeptember2026FlashConfig(
		"Gemini 3.6 Flash stable multimodal reasoning model optimized for rapid agentic and coding loops.",
		gemini3FlashReasoningEfforts,
	),
	"gemini-3.5-transcribe": geminiSeptember2026TranscribeConfig(
		2.00,
		12.00,
		"Gemini 3.5 Transcribe stable audio-to-text model with language detection, diarization, word timestamps, smart transcription, and vocabulary biasing.",
	),
	"gemini-3.5-transcribe-live": geminiSeptember2026TranscribeConfig(
		3.50,
		21.00,
		"Gemini 3.5 Transcribe Live stable low-latency streaming audio-to-text model over the Live API.",
	),
	"gemini-omni-1.1-flash": geminiSeptember2026OmniConfig(
		"Gemini Omni 1.1 Flash stable video generation and editing model with native video output.",
	),
	"gemini-omni-flash-preview": geminiSeptember2026OmniConfig(
		"Gemini Omni Flash preview video model; scheduled shutdown September 30, 2026. Use gemini-omni-1.1-flash.",
	),
	"gemini-robotics-er-2-preview":           geminiSeptember2026RoboticsConfig(false),
	"gemini-robotics-er-2-streaming-preview": geminiSeptember2026RoboticsConfig(true),
}

var geminiSeptember2026LifecycleDescriptions = map[string]string{
	"gemini-embedding-001":           "Gemini Embedding text model; scheduled shutdown May 14, 2028. Use gemini-embedding-2 for new workloads.",
	"gemini-3.1-flash-lite":          "Gemini 3.1 Flash-Lite stable cost-efficient multimodal model; scheduled shutdown May 7, 2027. Use gemini-3.5-flash-lite.",
	"gemini-3-flash-preview":         "Gemini 3 Flash preview multimodal reasoning model. Google recommends gemini-3.6-flash for stable production workloads; no shutdown date is announced.",
	"gemini-2.5-pro":                 "Gemini 2.5 Pro stable multimodal reasoning model with a 1M-token context window.",
	"gemini-2.5-flash":               "Gemini 2.5 Flash stable multimodal reasoning model optimized for latency and cost.",
	"gemini-2.5-flash-lite":          "Gemini 2.5 Flash-Lite stable cost-efficient multimodal model.",
	"gemini-2.0-flash":               "Gemini 2.0 Flash was shut down June 1, 2026. Use gemini-3.6-flash.",
	"gemini-2.0-flash-lite":          "Gemini 2.0 Flash-Lite was shut down June 1, 2026. Use gemini-3.1-flash-lite.",
	"gemini-robotics-er-1.6-preview": "Gemini Robotics ER 1.6 preview was shut down August 31, 2026. Use gemini-robotics-er-2-preview or gemini-robotics-er-2-streaming-preview.",
}

// refreshGeminiSeptember2026LifecycleMetadata corrects stale lifecycle descriptions
// without changing the pricing or capability metadata already attached to existing entries.
// Parameters: none. Returns: none.
func refreshGeminiSeptember2026LifecycleMetadata() {
	for model, description := range geminiSeptember2026LifecycleDescriptions {
		config, ok := ModelRatios[model]
		if !ok {
			continue
		}
		config.Description = description
		ModelRatios[model] = config
	}
}

// init installs the September 2026 Gemini catalog, refreshes lifecycle metadata,
// and rebuilds the derived model list. Parameters: none. Returns: none.
func init() {
	for model, config := range geminiSeptember2026ModelRatios {
		ModelRatios[model] = config
	}
	for _, model := range []string{
		"gemini-3.8-flash",
		"gemini-3.7-flash",
		"gemini-robotics-er-2-preview",
		"gemini-robotics-er-2-streaming-preview",
	} {
		geminiWebSearchModels[model] = struct{}{}
	}
	refreshGeminiSeptember2026LifecycleMetadata()
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
