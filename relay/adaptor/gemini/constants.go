package gemini

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
)

// ModelRatios uses the shared Gemini pricing from geminiOpenaiCompatible.
var ModelRatios = geminiOpenaiCompatible.ModelRatios

// ModelList is derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// GeminiToolingDefaults reuses the Gemini OpenAI-compatible tooling defaults sourced from Google pricing, verified 2026-09-02.
var GeminiToolingDefaults = geminiOpenaiCompatible.GeminiToolingDefaults()

// ModelsSupportSystemInstruction lists models that accept system instructions.
// Keep stable IDs and dated snapshots aligned with the shared model catalog.
//
// Sources: https://ai.google.dev/gemini-api/docs/models (verified 2026-09-18)
// and https://aistudio.google.com/docs/models (Gemini 4 verified 2026-11-12).
// and https://ai.google.dev/gemini-api/docs/latest-model (Argon verified 2026-10-13).
// Vertex additions verified 2026-11-13 against the System instructions capability:
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-argon
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/3-8-flash-cyber
// Cyber variants retain the base capability except the differences listed on their model pages.
var ModelsSupportSystemInstruction = []string{
	"gemini-2.5-flash", "gemini-2.5-flash-preview",
	"gemini-2.5-flash-lite", "gemini-2.5-flash-lite-preview",
	"gemini-2.5-flash-native-audio",
	"gemini-2.5-pro", "gemini-2.5-pro-preview",
	"gemini-2.5-computer-use-preview", "gemini-2.5-computer-use-preview-10-2025",
	"gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-3-pro-image-preview",
	"gemini-3-pro-image",
	"gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools",
	"gemini-3.1-flash-image-preview", "gemini-3.1-flash-lite", "gemini-3.1-flash-lite-preview",
	"gemini-3.1-flash-image", "gemini-3.1-flash-lite-image",
	geminiOpenaiCompatible.GeminiNanoBanana21Model,
	"gemini-3.5-flash", "gemini-3.5-flash-lite",
	"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash",
	"gemini-4-argon",
	geminiOpenaiCompatible.Gemini4CarbonModel, geminiOpenaiCompatible.Gemini4HydrogenModel,
	"gemini-4-carbon-cyber-permissive", "gemini-4-argon-cyber-permissive",
	"gemini-3.8-flash-cyber",
	"gemini-robotics-er-1.6-preview", "gemini-robotics-er-2-preview",
}

// IsModelSupportSystemInstruction reports whether model accepts system instructions.
// Parameters: model is the upstream Gemini model ID. Returns: true when system instructions are supported.
func IsModelSupportSystemInstruction(model string) bool {
	return geminiOpenaiCompatible.IsGemini4Model(model) ||
		geminiOpenaiCompatible.IsGeminiNanoBanana21Model(model) ||
		slices.Contains(ModelsSupportSystemInstruction, model)
}
