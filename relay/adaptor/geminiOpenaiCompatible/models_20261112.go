package geminiOpenaiCompatible

import (
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

const (
	// Gemini4CarbonModel is the stable Gemini 4 Carbon upstream model ID.
	Gemini4CarbonModel = "gemini-4-carbon"
	// Gemini4HydrogenModel is the stable Gemini 4 Hydrogen upstream model ID.
	Gemini4HydrogenModel = "gemini-4-hydrogen"
)

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

// init registers the November 2026 chat models and refreshes Developer API
// lifecycle descriptions. It takes no parameters and returns nothing. Historical
// IDs and tariffs remain available; redirects are upstream behavior, not local
// model mappings or a claim that Vertex follows the Developer API schedule.
func init() {
	for name, cfg := range map[string]adaptor.ModelConfig{
		Gemini4CarbonModel:   gemini4CarbonConfig(),
		Gemini4HydrogenModel: gemini4HydrogenConfig(),
	} {
		if _, exists := ModelRatios[name]; exists {
			panic("gemini: duplicate model definition " + name)
		}
		ModelRatios[name] = cfg
		geminiWebSearchModels[name] = struct{}{}
	}
	// https://aistudio.google.com/docs/changelog and /docs/deprecations,
	// verified 2026-11-12. Use replacement IDs to adopt their wire contract and
	// tariffs explicitly; these notes must not silently reprice legacy channels.
	for name, description := range map[string]string{
		"gemini-2.5-pro": "Gemini 2.5 Pro is deprecated in the Gemini Developer API. Effective December 1, 2026, " +
			"Google routes requests to gemini-3.8-flash with that model's schema and pricing. " +
			"Use the replacement ID explicitly; this legacy entry retains historical pricing for configured channels.",
		"gemini-2.5-flash": "Gemini 2.5 Flash is deprecated in the Gemini Developer API. Effective December 1, 2026, " +
			"Google routes requests to gemini-3.5-flash-lite with that model's schema and pricing. " +
			"Use the replacement ID explicitly; this legacy entry retains historical pricing for configured channels.",
		"gemini-embedding-2-preview": "Gemini Embedding 2 preview is deprecated. Since November 6, 2026, " +
			"Gemini Developer API requests route to gemini-embedding-2 with the same schema, embedding space and pricing. " +
			"Existing vectors do not require reindexing; use the stable ID for new workloads.",
		"gemini-omni-1.1-flash": "Gemini Omni 1.1 Flash video generation model. Effective December 3, 2026, " +
			"Gemini Developer API requests route to gemini-omni-1.2-flash. " +
			"The replacement needs a separate video protocol integration; this legacy entry retains historical pricing.",
	} {
		cfg, exists := ModelRatios[name]
		if !exists {
			panic("gemini: missing legacy model for lifecycle update " + name)
		}
		cfg.Description = description
		ModelRatios[name] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
