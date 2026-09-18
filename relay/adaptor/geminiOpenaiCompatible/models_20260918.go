package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Gemini catalog sources, verified 2026-09-18:
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live
//   - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live-extended-thinking
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://ai.google.dev/gemini-api/docs/deprecations
//
// Like the existing Live entries, these are upstream catalog metadata, not an
// implementation of the Live API transport. The REST adaptor cannot relay Live
// sessions. Google's separate $1.00/1M image/video INPUT rate cannot be represented
// by ModelConfig's image-generation or video-output pricing fields; leave those
// fields unset rather than advertising an incorrect output charge.

// gemini38LiveConfig builds the documented text/audio prices and Live capabilities.
// Parameters: extendedThinking selects the configurable background-reasoning variant.
// Returns: an independently allocated configuration without Flash pricing overlays.
func gemini38LiveConfig(extendedThinking bool) adaptor.ModelConfig {
	config := adaptor.ModelConfig{
		Ratio:           0.75 * ratio.MilliTokensUsd,
		CompletionRatio: 4.50 / 0.75,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.00 / 0.75,
			CompletionRatio: 12.00 / 4.50,
		},
		ContextLength:     131_072,
		MaxOutputTokens:   65_536,
		InputModalities:   []string{"text", "image", "audio", "video"},
		OutputModalities:  []string{"text", "audio"},
		SupportedFeatures: []string{"tools", "web_search", "reasoning"},
		Description: "Gemini 3.8 Live stable voice model with automatic interleaved thinking; " +
			"thinking configuration is not supported. Catalog metadata only: requires the Live API, " +
			"not implemented by this REST adaptor. Text output uses audio transcription.",
	}
	if extendedThinking {
		config.SupportedReasoningEfforts = []string{"low", "medium", "high"}
		config.Description = "Gemini 3.8 Live Extended Thinking stable voice model with background reasoning " +
			"and non-blocking function calls. Catalog metadata only: requires the Live API, " +
			"not implemented by this REST adaptor. Text output uses audio transcription."
	}
	// Do not infer a default thinking level, integer thinking budget, caching
	// discount, or the Flash family's 2027 price transition for either Live model.
	return config
}

var geminiSeptember18LifecycleDescriptions = map[string]string{
	"gemini-3-pro-preview":           "Gemini 3 Pro preview has a published earliest shutdown date of March 9, 2026. Use gemini-3.1-pro-preview.",
	"gemini-3.1-flash-lite-preview":  "Gemini 3.1 Flash-Lite preview has a published earliest shutdown date of May 25, 2026. Use gemini-3.1-flash-lite or gemini-3.5-flash-lite.",
	"gemini-3.1-flash-image-preview": "Gemini 3.1 Flash Image preview has a published earliest shutdown date of June 25, 2026. Use gemini-3.1-flash-image.",
	"gemini-3-pro-image-preview":     "Gemini 3 Pro Image preview has a published earliest shutdown date of June 25, 2026. Use gemini-3-pro-image.",
	"gemini-2.5-flash-image":         "Gemini 2.5 Flash Image stable native image model; earliest shutdown October 2, 2026. Prefer the current stable gemini-3.1-flash-image model.",
	"gemini-2.5-flash-image-preview": "Gemini 2.5 Flash Image preview has a published earliest shutdown date of January 15, 2026. Prefer the current stable gemini-3.1-flash-image model.",
	"gemini-3.1-flash-live-preview":  "Gemini 3.1 Flash Live preview; no shutdown date is announced. Use gemini-3.8-live for new Live API workloads. Catalog metadata does not add Live transport support.",
}

// refreshGeminiSeptember18Catalog installs the September 15 Live release and
// updates lifecycle descriptions while preserving existing prices and model IDs.
// Parameters: none. Returns: none. The catalog init calls this before rebuilding
// ModelList, avoiding a second init function with a file-order dependency.
func refreshGeminiSeptember18Catalog() {
	for model, extendedThinking := range map[string]bool{
		"gemini-3.8-live":                   false,
		"gemini-3.8-live-extended-thinking": true,
	} {
		ModelRatios[model] = gemini38LiveConfig(extendedThinking)
		geminiWebSearchModels[model] = struct{}{}
	}
	for model, description := range geminiSeptember18LifecycleDescriptions {
		config, ok := ModelRatios[model]
		if !ok {
			continue
		}
		config.Description = description
		ModelRatios[model] = config
	}
}
