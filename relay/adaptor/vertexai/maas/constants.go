// Package maas relays the documented Vertex managed open-model Chat Completions API.
package maas

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains verified standard online tariffs and endpoint-specific limits.
// Sources retrieved 2026-11-12:
// https://cloud.google.com/model-garden/partner-models/pricing
// https://cloud.google.com/model-garden/pricing
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/zaiorg/glm-52
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/google/gemma-4-26b-a4b-it
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/llama/llama4-maverick
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/llama/llama4-scout
var ModelRatios = map[string]adaptor.ModelConfig{
	"zai-org/glm-5.2-maas": {
		Ratio: 1.40 * ratio.MilliTokensUsd, CompletionRatio: 4.40 / 1.40, CachedInputRatio: .14 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 64000,
		InputModalities: []string{"text"}, OutputModalities: []string{"text"},
		SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
		Description:       "GLM 5.2 on Vertex MaaS (Preview, global). Minimum availability November 30, 2026. Thinking defaults on; use native chat_template_kwargs.enable_thinking.",
	},
	"google/gemma-4-26b-a4b-it-maas": {
		Ratio: .15 * ratio.MilliTokensUsd, CompletionRatio: .60 / .15, CachedInputRatio: .015 * ratio.MilliTokensUsd,
		ContextLength: 262144, MaxOutputTokens: 128000,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures: []string{"tools", "structured_outputs"},
		Description:       "Gemma 4 26B A4B IT on Vertex MaaS (Experimental, global). Text/image input; audio/video unsupported. Thinking is not advertised because the model card and thinking guide disagree.",
	},
	"meta/llama-4-maverick-17b-128e-instruct-maas": {
		Ratio: .35 * ratio.MilliTokensUsd, CompletionRatio: 1.15 / .35,
		ContextLength: 524288, MaxOutputTokens: 8192,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures: []string{"tools", "structured_outputs"},
		Description:       "Llama 4 Maverick on Vertex MaaS (GA, us-east5), fixed FP8, at most three input images. No Llama Guard or reasoning control advertised.",
	},
	"meta/llama-4-scout-17b-16e-instruct-maas": {
		Ratio: .25 * ratio.MilliTokensUsd, CompletionRatio: .70 / .25,
		ContextLength: 1310720, MaxOutputTokens: 8192,
		InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
		SupportedFeatures: []string{"tools", "structured_outputs"},
		Description:       "Llama 4 Scout on Vertex MaaS (GA, us-east5), fixed FP8, at most three input images. No Llama Guard or reasoning control advertised.",
	},
}

// ModelList enumerates only models with documented managed API IDs and tariffs.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// EndpointConfig returns the documented host/location for an exact managed model ID.
// The boolean excludes unknown publishers and self-deployed Model Garden variants.
func EndpointConfig(name string) (host, region string, ok bool) {
	switch name {
	case "zai-org/glm-5.2-maas", "google/gemma-4-26b-a4b-it-maas":
		return "aiplatform.googleapis.com", "global", true
	case "meta/llama-4-maverick-17b-128e-instruct-maas", "meta/llama-4-scout-17b-16e-instruct-maas":
		return "us-east5-aiplatform.googleapis.com", "us-east5", true
	default:
		return "", "", false
	}
}
