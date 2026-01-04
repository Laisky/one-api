package openai

import "strings"

// IsModelsOnlySupportedByChatCompletionAPI determines if a model only supports ChatCompletion API
// and should not be converted to Response API format.
// Currently returns false for all models (allowing conversion), but can be implemented later
// to return true for specific models that only support ChatCompletion API.
func IsModelsOnlySupportedByChatCompletionAPI(actualModel string) bool {
	switch {
	case strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-search-"),
		strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-audio-"):
		return true
	default:
		return false
	}
}
