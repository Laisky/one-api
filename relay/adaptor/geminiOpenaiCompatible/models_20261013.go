package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gemini4ArgonConfig returns independently owned Gemini 4 Argon defaults. It
// accepts no parameters and returns standard API pricing and conservative REST metadata.
// Sources verified 2026-10-14:
//   - https://ai.google.dev/gemini-api/docs/models/gemini-4-argon
//   - https://ai.google.dev/gemini-api/docs/generate-content/latest-model
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
//
// Vertex's shared defaults describe the global endpoint's effective net price;
// its introductory discount is delivered as credits, not a lower gross invoice.
// Non-global endpoints and account-specific discounts require channel overrides.
func gemini4ArgonConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:            2.00 * ratio.MilliTokensUsd,
		CompletionRatio:  10.00 / 2.00,
		CachedInputRatio: 0.10 * ratio.MilliTokensUsd,
		TimeWindows: []adaptor.TimeWindow{
			{
				Name:     "argon-standard-pricing-from-december-2026",
				TimeZone: "UTC",
				DateFrom: "2026-12-01",
				Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
				Overlay: adaptor.ModelConfig{
					Ratio:            4.00 * ratio.MilliTokensUsd,
					CompletionRatio:  20.00 / 4.00,
					CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
				},
			},
		},
		// Input, thinking, and output share one context. generateContent stops
		// at 32K without streaming and 64K with streaming. This common field has
		// no transport dimension, so publish the conservative non-streaming cap,
		// not the Interactions API's uniform 64K synchronous cap. This metadata
		// does not clamp explicit streaming requests or implement continuation.
		ContextLength:   1_048_576,
		MaxOutputTokens: 32_768,
		InputModalities: []string{"text", "image", "audio", "video", "file"},
		OutputModalities: []string{
			"text",
		},
		SupportedFeatures: []string{
			"tools", "json_mode", "structured_outputs", "web_search", "reasoning",
		},
		// Temperature, top_p, and top_k are ignored by this model. Do not copy
		// older models' integer thinking-budget ceiling into level-based thinking.
		SupportedSamplingParameters: []string{"max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Description: "Gemini 4 Argon stable reasoning model. Shared 1,048,576-token input/thinking/output context; " +
			"32,768-token non-streaming and 65,536-token streaming generateContent hop limits. " +
			"Discovery uses the conservative non-streaming limit. Caching is implicit only. " +
			"Long decode requires continuation and is not advertised by this relay. " +
			"Text output only; no image generation, audio generation, or Live API.",
	}
}

// init registers Argon after the existing catalogue refreshes. It accepts no
// parameters and returns no values. Duplicate IDs fail instead of replacing an
// existing definition; historical model configurations remain unchanged.
func init() {
	const modelName = "gemini-4-argon"
	if _, exists := ModelRatios[modelName]; exists {
		panic("gemini: duplicate model definition " + modelName)
	}
	ModelRatios[modelName] = gemini4ArgonConfig()
	geminiWebSearchModels[modelName] = struct{}{}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
