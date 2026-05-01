package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// ModelRatios contains all supported OpenAI models and their pricing/configuration metadata.
// The map is assembled from family-specific submaps (constants_<family>.go) so the
// per-family files stay focused and readable. Model list is derived from the keys of
// this map, eliminating redundancy.
//
// Pricing sources:
//   - https://platform.openai.com/docs/pricing
//   - https://platform.openai.com/docs/models
//   - https://developers.openai.com/api/docs/pricing (realtime/audio)
var ModelRatios = mergeModelRatios(
	gpt35ModelRatios,
	gpt4ModelRatios,
	gpt4oModelRatios,
	realtimeModelRatios,
	gpt45ModelRatios,
	gpt41ModelRatios,
	gpt5ModelRatios,
	oSeriesModelRatios,
	specializedModelRatios,
	embeddingModelRatios,
	audioModelRatios,
	imageModelRatios,
	videoModelRatios,
)

// ModelList derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// OpenAIToolingDefaults enumerates OpenAI's built-in tool whitelist and pricing (retrieved 2026-03-05).
// Source: https://r.jina.ai/https://platform.openai.com/docs/pricing#built-in-tools
var OpenAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"code_interpreter":                 {UsdPerCall: 0.03},   // $0.03 per default-tier container session (20-minute billing begins 2026-03-31)
		"file_search":                      {UsdPerCall: 0.0025}, // $2.50 per 1K tool calls
		"web_search":                       {UsdPerCall: 0.01},   // $10 per 1K tool calls
		"web_search_preview_reasoning":     {UsdPerCall: 0.01},   // Preview tier for reasoning models, $10 per 1K tool calls
		"web_search_preview_non_reasoning": {UsdPerCall: 0.025},  // Preview tier for non-reasoning models, $25 per 1K tool calls
	},
}

// mergeModelRatios concatenates per-family pricing maps into a single ModelRatios map.
// It returns a fresh map and panics if duplicate keys are detected so misconfiguration
// surfaces at process start rather than silently overwriting entries.
func mergeModelRatios(maps ...map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	out := make(map[string]adaptor.ModelConfig, total)
	for _, m := range maps {
		for name, cfg := range m {
			if _, dup := out[name]; dup {
				panic("openai.mergeModelRatios: duplicate model entry: " + name)
			}
			out[name] = cfg
		}
	}
	return out
}

// standardSamplingParameters returns a fresh slice of OpenAI-compatible sampling
// parameters supported by standard (non-reasoning) chat-completions models.
// A new slice is returned on every call to keep callers from mutating shared state.
func standardSamplingParameters() []string {
	return []string{
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"stop",
		"seed",
		"max_tokens",
		"logprobs",
		"logit_bias",
	}
}

// reasoningSamplingParameters returns the constrained sampling-parameter set
// supported by OpenAI's reasoning models (o-series, gpt-5 family). Reasoning
// models reject temperature, top_p, frequency_penalty, and presence_penalty.
func reasoningSamplingParameters() []string {
	return []string{"seed", "max_tokens"}
}
