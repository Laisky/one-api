package gemini

import (
	"fmt"
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
	"strings"
)

// convertToolChoiceToConfig maps a client tool choice into a native tool configuration.
// Parameters: toolChoice is the client-supplied selection.
// Returns: the native tool configuration or nil when none is needed.
func convertToolChoiceToConfig(toolChoice any) *ToolConfig {
	switch choice := toolChoice.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "none":
			return &ToolConfig{FunctionCallingConfig: FunctionCallingConfig{Mode: "NONE"}}
		default:
			return nil
		}
	case map[string]any:
		choiceType := strings.ToLower(strings.TrimSpace(fmt.Sprint(choice["type"])))
		var allowed []string
		switch choiceType {
		case "function":
			if fn, ok := choice["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
					allowed = append(allowed, strings.TrimSpace(name))
				}
			}
		case "tool":
			if name, ok := choice["name"].(string); ok && strings.TrimSpace(name) != "" {
				allowed = append(allowed, strings.TrimSpace(name))
			}
		}
		if len(allowed) > 0 {
			return &ToolConfig{FunctionCallingConfig: FunctionCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: allowed,
			}}
		}
	}
	return nil
}

// ConvertEmbeddingRequest converts an OpenAI-compatible embedding request to Gemini's BatchEmbeddingRequest format.
// Parameters: request is the OpenAI-compatible embeddings request.
// Returns: the Gemini batch embedding request or an error when the input cannot be normalized into Gemini contents.
func ConvertEmbeddingRequest(request model.GeneralOpenAIRequest) (*BatchEmbeddingRequest, error) {
	contents, _, err := BuildEmbeddingContents(request.Input)
	if err != nil {
		return nil, errors.Wrap(err, "build gemini embedding contents")
	}

	requests := make([]EmbeddingRequest, len(contents))
	model := fmt.Sprintf("models/%s", request.Model)

	for i, content := range contents {
		requests[i] = EmbeddingRequest{
			Model:   model,
			Content: content,
		}
	}

	return &BatchEmbeddingRequest{
		Requests: requests,
	}, nil
}

// resolveGeminiAPIVersion selects the Gemini API version for the requested model.
// Parameters: modelName is the target upstream model and configuredVersion is the optional channel override.
// Returns: the API version string that should be used for Gemini requests.
func resolveGeminiAPIVersion(modelName string, configuredVersion string) string {
	defaultVersion := config.GeminiVersion
	modelName = strings.ToLower(modelName)
	if geminiOpenaiCompatible.GeminiVersionAtLeast(modelName, 1.5) ||
		strings.Contains(modelName, "preview") ||
		strings.Contains(modelName, "gemma-3") {
		defaultVersion = "v1beta"
	}

	return helper.AssignOrDefault(configuredVersion, defaultVersion)
}
