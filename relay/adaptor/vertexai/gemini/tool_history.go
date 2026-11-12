package vertexai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/model"
)

// configureToolHistory replaces textual tool results with native v1 function responses.
// Parameters: converted contains shared content lowering, source owns the caller history,
// and modelName is the effective upstream model. Returns a controlled validation error
// for ambiguous history; source messages and call IDs are never changed.
func configureToolHistory(converted *gemini.ChatRequest, source *model.GeneralOpenAIRequest, modelName string) error {
	hasHistory := false
	for _, message := range source.Messages {
		if len(message.ToolCalls) > 0 || message.Role == "tool" {
			hasHistory = true
			break
		}
	}
	if !hasHistory {
		return nil
	}
	var output []gemini.ChatContent
	var pending []model.Tool
	results := make(map[string]*gemini.FunctionResponse)
	callsByID := make(map[string]*model.Function)
	contentIndex := 0
	for _, message := range source.Messages {
		if message.Role == "system" && gemini.IsModelSupportSystemInstruction(modelName) {
			continue
		}
		if contentIndex >= len(converted.Contents) {
			return errors.New("validation failed: invalid Vertex content alignment")
		}
		content := converted.Contents[contentIndex]
		contentIndex++
		if message.Role == "tool" {
			if len(pending) == 0 || message.ToolCallId == "" {
				return errors.New("validation failed: Vertex tool result has no matching function call")
			}
			matched := callsByID[message.ToolCallId]
			if matched == nil || results[message.ToolCallId] != nil {
				return errors.New("validation failed: unknown or duplicate Vertex tool result")
			}
			if message.Name != nil && *message.Name != "" && *message.Name != matched.Name {
				return errors.New("validation failed: Vertex tool result name does not match its function call")
			}
			result, err := vertexToolResult(message)
			if err != nil {
				return errors.Wrap(err, "convert Vertex function result")
			}
			results[message.ToolCallId] = &gemini.FunctionResponse{Name: matched.Name, Response: result}
			if len(results) == len(pending) {
				parts := make([]gemini.Part, 0, len(pending))
				for _, call := range pending {
					parts = append(parts, gemini.Part{FunctionResponse: results[call.Id]})
				}
				output = append(output, gemini.ChatContent{Role: "user", Parts: parts})
				pending = nil
				callsByID = make(map[string]*model.Function)
				results = make(map[string]*gemini.FunctionResponse)
			}
			continue
		}
		if len(pending) > 0 {
			return errors.New("validation failed: Vertex function calls require all results before the next turn")
		}
		if len(message.ToolCalls) > 0 {
			if message.Role != "assistant" {
				return errors.New("validation failed: Vertex function calls require an assistant message")
			}
			pending = message.ToolCalls
			callIndex := 0
			for i := range content.Parts {
				if content.Parts[i].FunctionCall == nil {
					continue
				}
				if callIndex >= len(pending) {
					return errors.New("validation failed: invalid Vertex function call alignment")
				}
				call := pending[callIndex]
				if call.Id == "" || callsByID[call.Id] != nil || call.Function == nil || call.Function.Name == "" {
					return errors.New("validation failed: Vertex function calls require unique IDs and function names")
				}
				callsByID[call.Id] = call.Function
				raw, err := call.Function.ArgumentsJSON()
				if err != nil {
					return errors.Wrap(err, "validation failed: invalid Vertex function arguments")
				}
				if !json.Valid([]byte(raw)) {
					return errors.New("validation failed: Vertex function arguments must be complete JSON")
				}
				var args map[string]any
				decoder := json.NewDecoder(strings.NewReader(raw))
				decoder.UseNumber()
				if err := decoder.Decode(&args); err != nil || args == nil {
					return errors.New("validation failed: Vertex function arguments must be a JSON object")
				}
				// Preserve complete argument objects and number literals without the shared float conversion.
				content.Parts[i].FunctionCall.Arguments = args
				callIndex++
			}
			if callIndex != len(pending) {
				return errors.New("validation failed: invalid Vertex function call alignment")
			}
		}
		output = append(output, content)
		if message.Role == "system" {
			if contentIndex >= len(converted.Contents) {
				return errors.New("validation failed: invalid Vertex system content alignment")
			}
			output = append(output, converted.Contents[contentIndex])
			contentIndex++
		}
	}
	if len(pending) > 0 {
		return errors.New("validation failed: Vertex function calls are missing tool results")
	}
	if contentIndex != len(converted.Contents) {
		return errors.New("validation failed: invalid Vertex content alignment")
	}
	converted.Contents = output
	return nil
}

// vertexToolResult maps a text result to a protobuf Struct without changing object keys.
// Parameters: message is the correlated tool result. Returns the original JSON object,
// or an output wrapper for text/scalar/array/null values, and an error for media that
// this text-result path cannot represent without discarding information.
func vertexToolResult(message model.Message) (map[string]any, error) {
	if len(message.ToolResultParts) > 0 {
		for _, part := range message.ToolResultParts {
			if part.Type != model.ContentTypeText {
				return nil, errors.New("validation failed: Vertex multimodal tool results require native response parts")
			}
		}
	}
	parts, err := message.ParseContentChecked()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed: invalid Vertex tool result content")
	}
	for _, part := range parts {
		if part.Type != model.ContentTypeText {
			return nil, errors.New("validation failed: Vertex multimodal tool results require native response parts")
		}
	}
	text := message.StringContent()
	var value any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if json.Valid([]byte(text)) {
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.Wrap(err, "validation failed: invalid Vertex tool result JSON")
		}
		if message.ToolResultIsError {
			return map[string]any{"error": value}, nil
		}
		if object, ok := value.(map[string]any); ok {
			return object, nil
		}
	} else {
		value = text
	}
	if message.ToolResultIsError {
		return map[string]any{"error": value}, nil
	}
	return map[string]any{"output": value}, nil
}
