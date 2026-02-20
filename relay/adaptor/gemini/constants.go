package gemini

import (
	"slices"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
)

// ModelRatios uses the shared Gemini pricing from geminiOpenaiCompatible
var ModelRatios = geminiOpenaiCompatible.ModelRatios

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// GeminiToolingDefaults reuses the Gemini OpenAI-compatible tooling defaults sourced from Google pricing (retrieved 2025-11-12).
var GeminiToolingDefaults = geminiOpenaiCompatible.GeminiToolingDefaults()

// ModelsSupportSystemInstruction is the list of models that support system instruction.
//
// https://cloud.google.com/vertex-ai/generative-ai/docs/learn/prompts/system-instructions
var ModelsSupportSystemInstruction = []string{
	"gemini-2.0-flash", "gemini-2.0-flash-exp",
	"gemini-2.0-flash-thinking-exp-01-21",
	"gemini-2.0-flash-lite",
	"gemini-2.0-pro-exp-02-05",
	"gemini-2.5-flash", "gemini-2.5-flash-preview",
	"gemini-2.5-flash-lite", "gemini-2.5-flash-lite-preview",
	"gemini-2.5-flash-native-audio",
	"gemini-2.5-pro", "gemini-2.5-pro-preview",
	"gemini-2.5-computer-use-preview",
	"gemini-3-pro-preview",
	"gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools",
}

// IsModelSupportSystemInstruction check if the model support system instruction.
//
// Because the main version of Go is 1.20, slice.Contains cannot be used
func IsModelSupportSystemInstruction(model string) bool {
	return slices.Contains(ModelsSupportSystemInstruction, model)
}
