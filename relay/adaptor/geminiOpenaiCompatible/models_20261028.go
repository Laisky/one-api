package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

const (
	// GeminiNanoBanana21Model is the stable upstream ID used by the Gemini and Vertex adaptors.
	GeminiNanoBanana21Model = "gemini-nano-banana-2.1"

	// Google bills image output by resolution: 1,120/1,680/3,780 tokens
	// at $30 per million output-image tokens for Standard inference.
	geminiNanoBanana21Image1KUsd = 1120 * 30.0 / 1_000_000
	geminiNanoBanana21Image2KUsd = 1680 * 30.0 / 1_000_000
	geminiNanoBanana21Image4KUsd = 3780 * 30.0 / 1_000_000
)

// geminiNanoBanana21Config builds the October 2026 Nano Banana 2.1 catalog entry.
// Parameters: none. Returns: independent standard-tier model and image metadata.
// Sources:
//   - https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/nano-banana-2-1
func geminiNanoBanana21Config() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           1.50 * ratio.MilliTokensUsd,
		CompletionRatio: 7.50 / 1.50,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: geminiNanoBanana21Image1KUsd,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"2048x2048": geminiNanoBanana21Image2KUsd / geminiNanoBanana21Image1KUsd,
				"4096x4096": geminiNanoBanana21Image4KUsd / geminiNanoBanana21Image1KUsd,
			},
		},
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             []string{"text", "image", "video", "file"},
		OutputModalities:            []string{"text", "image"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: []string{"max_tokens"},
		SupportedReasoningEfforts:   []string{"minimal", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Description: "Gemini Nano Banana 2.1 stable image generation and conversational editing model. " +
			"Supports text, images, video, and PDF inputs; text and image outputs. " +
			"128K context, 32K output, 1K default render and 2K/4K resolution tiers. " +
			"The upstream supports Google Search, but this gateway does not forward built-in web_search yet. " +
			"Requests for web_search are rejected explicitly; level-based thinking remains supported. " +
			"function calling, JSON structured output, Live API, and sampling controls are not. " +
			"Developer API caching is unavailable; Vertex AI implicit-cache pricing is provider-specific. " +
			"Higher image resolution requires explicit native imageConfig and matching billing configuration.",
	}
}

// geminiOctober28LifecycleDescriptions records Google's October 22 published
// shutdown schedule. Keep historical IDs and prices intact for configured channels.
var geminiOctober28LifecycleDescriptions = map[string]string{
	"gemini-3.1-flash-tts-preview":                  "Gemini 3.1 Flash TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-tts or gemini-3.8-flash-lite-tts; migration requires the new speech_metadata protocol.",
	"gemini-2.5-flash-preview-tts":                  "Gemini 2.5 Flash TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-lite-tts; migration requires the new speech_metadata protocol.",
	"gemini-2.5-pro-preview-tts":                    "Gemini 2.5 Pro TTS preview; published earliest shutdown November 17, 2026. Use gemini-3.8-flash-tts; migration requires the new speech_metadata protocol.",
	"gemini-3.1-flash-live-preview":                 "Gemini 3.1 Flash Live preview; published earliest shutdown November 17, 2026. Use gemini-3.8-live with its dedicated native Live WebSocket API.",
	"gemini-2.5-flash-native-audio-preview-12-2025": "Gemini 2.5 Flash native-audio preview December 2025 snapshot; published earliest shutdown November 17, 2026. Use gemini-3.8-live and its native Live WebSocket transport.",
	"gemini-2.5-flash-image":                        "Gemini 2.5 Flash Image stable; published earliest shutdown March 15, 2027. Use gemini-3.1-flash-lite-image for new image workloads.",
	"gemini-omni-flash-preview":                     "Gemini Omni Flash preview video model; published shutdown October 22, 2026. Use gemini-omni-1.1-flash. Retaining this ID does not restore upstream availability.",
	"gemini-2.5-computer-use-preview-10-2025":       "Gemini 2.5 Computer Use October 2025 preview snapshot; published shutdown July 28, 2026. Use gemini-3.8-flash for new agentic workflows.",
}

// init installs the new model and refreshes lifecycle descriptions without
// removing existing IDs, reassigning legacy prices, or rewriting model aliases.
func init() {
	if _, exists := ModelRatios[GeminiNanoBanana21Model]; exists {
		panic("gemini: duplicate model definition " + GeminiNanoBanana21Model)
	}
	ModelRatios[GeminiNanoBanana21Model] = geminiNanoBanana21Config()
	for id, description := range geminiOctober28LifecycleDescriptions {
		cfg, exists := ModelRatios[id]
		if !exists {
			panic("gemini: missing legacy model for lifecycle update " + id)
		}
		cfg.Description = description
		ModelRatios[id] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
