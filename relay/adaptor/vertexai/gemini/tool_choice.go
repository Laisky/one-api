package vertexai

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
)

// configureToolChoice maps Chat, Responses, Claude and legacy choices into Vertex v1 modes.
// Parameters: converted is the native request, source supplies choices and declarations,
// and modelName is the effective model. Returns an error for malformed or unavailable
// selections instead of weakening an explicit caller constraint to automatic selection.
func configureToolChoice(converted *gemini.ChatRequest, source *model.GeneralOpenAIRequest, modelName string) error {
	if geminiOpenaiCompatible.IsGeminiNanoBanana21Model(modelName) {
		return nil
	}
	choice := source.ToolChoice
	legacy := false
	if choice == nil {
		choice = source.FunctionCall
		legacy = true
	}
	if choice == nil {
		return nil
	}
	var mode, name string
	switch value := choice.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "auto":
			mode = "AUTO"
		case "none":
			mode = "NONE"
		case "required":
			mode = "ANY"
		default:
			return errors.New("validation failed: unsupported Vertex tool choice")
		}
	case map[string]any:
		kind, _ := value["type"].(string)
		if legacy && kind == "" {
			name, _ = value["name"].(string)
			mode = "ANY"
		} else {
			switch kind {
			case "auto":
				mode = "AUTO"
			case "none":
				mode = "NONE"
			case "any", "required":
				mode = "ANY"
			case "function", "tool":
				name, _ = value["name"].(string)
				if function, ok := value["function"].(map[string]any); ok {
					name, _ = function["name"].(string)
				}
				if strings.TrimSpace(name) == "" {
					return errors.New("validation failed: Vertex named tool choice requires a name")
				}
				mode = "ANY"
			default:
				return errors.New("validation failed: unsupported Vertex tool choice")
			}
		}
	default:
		return errors.New("validation failed: malformed Vertex tool choice")
	}
	if mode == "ANY" {
		names := make(map[string]bool)
		if len(source.Tools) > 0 {
			for _, tool := range source.Tools {
				if tool.Function != nil {
					names[tool.Function.Name] = true
				}
			}
		} else {
			for _, function := range source.Functions {
				names[function.Name] = true
			}
		}
		if len(names) == 0 || (name != "" && !names[name]) {
			return errors.New("validation failed: Vertex tool choice has no matching function declaration")
		}
		if legacy {
			if _, ok := choice.(map[string]any); ok && name == "" {
				return errors.New("validation failed: legacy Vertex function choice requires a name")
			}
		}
	}
	config := gemini.FunctionCallingConfig{Mode: mode}
	if name != "" {
		config.AllowedFunctionNames = []string{name}
	}
	converted.ToolConfig = &gemini.ToolConfig{FunctionCallingConfig: config}
	return nil
}
