package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"strings"
)

// gemini4ModelRatios contains complete gemini4 catalog entries.
var gemini4ModelRatios = map[string]adaptor.ModelConfig{
	"gemini-4-argon":    gemini4ArgonConfig(),
	"gemini-4-carbon":   gemini4CarbonConfig(),
	"gemini-4-hydrogen": gemini4HydrogenConfig(),
}

const (
	// Gemini4CarbonModel is the stable Gemini 4 Carbon upstream model ID.
	Gemini4CarbonModel = "gemini-4-carbon"
	// Gemini4HydrogenModel is the stable Gemini 4 Hydrogen upstream model ID.
	Gemini4HydrogenModel = "gemini-4-hydrogen"
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

// IsGemini4Model reports whether model is a verified Gemini 4 text-generation
// model, matching case-insensitively without extending the contract to unknown IDs.
func IsGemini4Model(model string) bool {
	switch strings.ToLower(model) {
	case "gemini-4-argon", Gemini4CarbonModel, Gemini4HydrogenModel:
		return true
	default:
		return false
	}
}

// gemini4CarbonConfig returns independent Carbon pricing and discovery metadata.
// Carbon and Argon share the documented tariffs, December boundary and thinking
// levels. Shared discovery retains the conservative 32K non-streaming REST cap;
// a 1M long decode requires continuation, which this relay does not implement.
// Sources verified 2026-11-12:
//   - https://aistudio.google.com/docs/models/gemini-4-carbon
//   - https://aistudio.google.com/docs/pricing#gemini-4-carbon
//   - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon
func gemini4CarbonConfig() adaptor.ModelConfig {
	cfg := gemini4ArgonConfig()
	cfg.TimeWindows[0].Name = "carbon-standard-pricing-from-december-2026"
	cfg.Description = strings.Replace(cfg.Description, "Gemini 4 Argon", "Gemini 4 Carbon", 1)
	return cfg
}

// gemini4HydrogenConfig returns independent Hydrogen pricing and discovery
// metadata, including its high default and exclusive xhigh reasoning level.
// Sources verified 2026-11-12:
//   - https://aistudio.google.com/docs/latest-model
//   - https://aistudio.google.com/docs/pricing#gemini-4-hydrogen
//   - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen
func gemini4HydrogenConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                       1.50 * ratio.MilliTokensUsd,
		CompletionRatio:             7.50 / 1.50,
		CachedInputRatio:            0.075 * ratio.MilliTokensUsd,
		ContextLength:               1_048_576,
		MaxOutputTokens:             65_536,
		InputModalities:             []string{"text", "image", "audio", "video", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"},
		SupportedSamplingParameters: []string{"max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh"},
		DefaultReasoningEffort:      "high",
		Description: "Gemini 4 Hydrogen stable reasoning model. Shared 1,048,576-token input/thinking/output context; " +
			"65,536-token synchronous output limit for streaming and non-streaming. Caching is implicit only. " +
			"Long decode requires continuation and is not advertised by this relay. " +
			"Text output only; no image generation, audio generation, or Live API.",
	}
}
