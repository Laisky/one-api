package anthropic

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// decodeClaudeJSON decodes complete JSON while keeping numeric tool arguments exact.
func decodeClaudeJSON(raw []byte, value any) error {
	if !json.Valid(raw) {
		return errors.New("invalid Claude JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return errors.Wrap(decoder.Decode(value), "decode Claude JSON")
}

// prepareClaudeChatControls normalizes request-local controls without mutating caller-owned data.
func prepareClaudeChatControls(c *gin.Context, request model.GeneralOpenAIRequest) (model.GeneralOpenAIRequest, error) {
	if request.MaxCompletionTokens != nil && *request.MaxCompletionTokens > 0 {
		request.MaxTokens = *request.MaxCompletionTokens
	}
	if request.Thinking != nil {
		copy := *request.Thinking
		request.Thinking = &copy
	}
	if request.Thinking == nil && request.ExtraBody["thinking"] != nil {
		raw, err := json.Marshal(request.ExtraBody["thinking"])
		if err != nil {
			return request, errors.Wrap(err, "validation failed: encode thinking extension")
		}
		if err := json.Unmarshal(raw, &request.Thinking); err != nil {
			return request, errors.Wrap(err, "validation failed: decode thinking extension")
		}
	}
	if len(request.OutputConfig) == 0 && request.ExtraBody["output_config"] != nil {
		raw, err := json.Marshal(request.ExtraBody["output_config"])
		if err != nil {
			return request, errors.Wrap(err, "validation failed: encode output_config extension")
		}
		request.OutputConfig = raw
	}
	name := CompatibilityModel(c, request.Model)
	// Native effort controls, unlike generic reasoning_effort, are forwarded to all Claude models.
	if IsClaudeAdaptiveThinkingModel(name) || name == "claude-sonnet-4-6" || name == "claude-opus-4-6" {
		effort := request.ReasoningEffort
		if effort == nil && request.Reasoning != nil {
			effort = request.Reasoning.Effort
		}
		if effort != nil {
			output := map[string]json.RawMessage{}
			if len(request.OutputConfig) > 0 {
				if err := json.Unmarshal(request.OutputConfig, &output); err != nil {
					return request, errors.Wrap(err, "validation failed: decode output_config")
				}
				if output == nil {
					return request, errors.New("validation failed: output_config must be an object")
				}
			}
			if _, explicit := output["effort"]; !explicit {
				raw, err := json.Marshal(*effort)
				if err != nil {
					return request, errors.Wrap(err, "validation failed: encode effort")
				}
				output["effort"] = raw
				request.OutputConfig, err = json.Marshal(output)
				if err != nil {
					return request, errors.Wrap(err, "validation failed: encode output_config")
				}
			}
		}
	}
	if !IsClaudeSonnet55(name) && !IsClaudeHaiku55(name) {
		return request, nil
	}
	controls := map[string]json.RawMessage{}
	if request.Thinking != nil {
		raw, err := json.Marshal(request.Thinking)
		if err != nil {
			return request, errors.Wrap(err, "validation failed: encode thinking")
		}
		controls["thinking"] = raw
	}
	if len(request.OutputConfig) > 0 {
		controls["output_config"] = request.OutputConfig
	}
	if request.ToolChoice != nil {
		raw, err := json.Marshal(request.ToolChoice)
		if err != nil {
			return request, errors.Wrap(err, "validation failed: encode tool_choice")
		}
		controls["tool_choice"] = raw
	}
	if err := NormalizeSonnet55Controls(name, controls); err != nil {
		return request, err
	}
	if err := NormalizeHaiku55Controls(name, controls); err != nil {
		return request, err
	}
	if raw, exists := controls["thinking"]; exists {
		if err := json.Unmarshal(raw, &request.Thinking); err != nil {
			return request, errors.Wrap(err, "decode normalized thinking")
		}
	}
	return request, nil
}

// convertClaudeStop maps supported portable stop forms without silently dropping stopping constraints.
func convertClaudeStop(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	if text, ok := value.(string); ok {
		return []string{text}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Wrap(err, "validation failed: encode stop")
	}
	var stops []string
	if err := json.Unmarshal(raw, &stops); err != nil {
		return nil, errors.Wrap(err, "validation failed: stop must be a string or string array")
	}
	return stops, nil
}

// convertClaudeToolChoice maps explicit portable tool policy without replacing none or forced use with auto.
func convertClaudeToolChoice(value any, hasTools bool) (any, error) {
	if value == nil {
		if !hasTools {
			return nil, nil
		}
		return map[string]any{"type": "auto"}, nil
	}
	if kind, ok := value.(string); ok {
		switch kind {
		case "auto", "none":
			return map[string]any{"type": kind}, nil
		case "required", "any":
			return map[string]any{"type": "any"}, nil
		default:
			return nil, errors.New("validation failed: unsupported tool_choice")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.Wrap(err, "validation failed: encode tool_choice")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.Wrap(err, "validation failed: invalid tool_choice")
	}
	var kind string
	if err := json.Unmarshal(fields["type"], &kind); err != nil {
		return nil, errors.Wrap(err, "validation failed: missing tool_choice.type")
	}
	switch kind {
	case "function":
		var function struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(fields["function"], &function); err != nil {
			return nil, errors.Wrap(err, "validation failed: invalid chosen function")
		}
		if function.Name == "" {
			return nil, errors.New("validation failed: chosen function has no name")
		}
		return map[string]any{"type": "tool", "name": function.Name}, nil
	case "auto", "none", "any", "tool":
		return fields, nil
	default:
		return nil, errors.New("validation failed: unsupported tool_choice.type")
	}
}
