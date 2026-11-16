package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// videoModelRatios contains complete video catalog entries.
var videoModelRatios = map[string]adaptor.ModelConfig{
	"gemini-omni-flash-preview": geminiOmniConfig(
		"Gemini Omni Flash preview video model; published shutdown October 22, 2026. Use gemini-omni-1.1-flash. Retaining this ID does not restore upstream availability.",
	),
	"gemini-omni-1.1-flash": geminiOmniConfig(
		"Gemini Omni 1.1 Flash video generation model. Effective December 3, 2026, Gemini Developer API requests route to gemini-omni-1.2-flash. The replacement needs a separate video protocol integration; this legacy entry retains historical pricing.",
	),
}

// geminiOmniConfig builds the shared Gemini Omni Flash pricing and
// video metadata. Parameters: description summarizes lifecycle status. Returns: a model configuration.
func geminiOmniConfig(description string) adaptor.ModelConfig {
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
