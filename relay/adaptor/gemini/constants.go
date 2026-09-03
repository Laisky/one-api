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
//
// Source: https://ai.google.dev/gemini-api/docs/models
var ModelsSupportSystemInstruction = []string{
	"gemini-2.5-flash", "gemini-2.5-flash-preview",
	"gemini-2.5-flash-lite", "gemini-2.5-flash-lite-preview",
	"gemini-2.5-flash-native-audio",
	"gemini-2.5-pro", "gemini-2.5-pro-preview",
	"gemini-2.5-computer-use-preview",
	"gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-3-pro-image-preview",
	"gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools",
	"gemini-3.1-flash-image-preview", "gemini-3.1-flash-lite", "gemini-3.1-flash-lite-preview",
	"gemini-3.5-flash", "gemini-3.5-flash-lite",
	"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash",
	"gemini-robotics-er-1.6-preview",
}

// IsModelSupportSystemInstruction reports whether model accepts system instructions.
// Parameters: model is the upstream Gemini model ID. Returns: true when system instructions are supported.
func IsModelSupportSystemInstruction(model string) bool {
	return slices.Contains(ModelsSupportSystemInstruction, model)
}
