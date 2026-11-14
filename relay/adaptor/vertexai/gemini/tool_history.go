package vertexai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/model"
)

// pendingToolCall is one caller-identified function call bound to its native functionCall Part.
// A batch stores these in restored native Part order, which is also the functionResponse order.
type pendingToolCall struct {
	id   string
	name string
}

// toolResultBatch correlates the results of one assistant turn's calls by caller ID.
// Every identity lives in calls; results are keyed by the same ID, never by array position.
type toolResultBatch struct {
	calls   []pendingToolCall
	names   map[string]string
	results map[string]map[string]any
}

// newToolResultBatch returns a batch awaiting results for calls in native order.
// Parameters: calls were produced by bindNativeFunctionCalls. Returns the empty batch.
func newToolResultBatch(calls []pendingToolCall) *toolResultBatch {
	batch := &toolResultBatch{calls: calls, names: make(map[string]string, len(calls)), results: make(map[string]map[string]any, len(calls))}
	for _, call := range calls {
		batch.names[call.id] = call.name
	}
	return batch
}

// accept records one tool message as the result of its correlated call.
// Parameters: message is a role=tool message. Returns a validation error for an unknown,
// duplicate, misnamed or unrepresentable result.
func (b *toolResultBatch) accept(message model.Message) error {
	if message.ToolCallId == "" {
		return errors.New("validation failed: Vertex tool result has no matching function call")
	}
	name, known := b.names[message.ToolCallId]
	if !known || b.results[message.ToolCallId] != nil {
		return errors.New("validation failed: unknown or duplicate Vertex tool result")
	}
	if message.Name != nil && *message.Name != "" && *message.Name != name {
		return errors.New("validation failed: Vertex tool result name does not match its function call")
	}
	result, err := vertexToolResult(message)
	if err != nil {
		return errors.Wrap(err, "convert Vertex function result")
	}
	b.results[message.ToolCallId] = result
	return nil
}

// complete reports whether every call in the batch has a result.
func (b *toolResultBatch) complete() bool {
	return len(b.results) == len(b.calls)
}

// content returns one user turn of functionResponse Parts in restored native call order.
// It must only be called on a complete batch.
func (b *toolResultBatch) content() gemini.ChatContent {
	parts := make([]gemini.Part, 0, len(b.calls))
	for _, call := range b.calls {
		parts = append(parts, gemini.Part{FunctionResponse: &gemini.FunctionResponse{Name: call.name, Response: b.results[call.id]}})
	}
	return gemini.ChatContent{Role: "user", Parts: parts}
}

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
	var batch *toolResultBatch
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
			if batch == nil {
				return errors.New("validation failed: Vertex tool result has no matching function call")
			}
			if err := batch.accept(message); err != nil {
				return errors.Wrap(err, "correlate Vertex tool result")
			}
			if batch.complete() {
				output = append(output, batch.content())
				batch = nil
			}
			continue
		}
		if batch != nil {
			return errors.New("validation failed: Vertex function calls require all results before the next turn")
		}
		if len(message.ToolCalls) > 0 {
			calls, err := bindNativeFunctionCalls(&content, message)
			if err != nil {
				return errors.Wrap(err, "bind Vertex function calls")
			}
			batch = newToolResultBatch(calls)
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
	if batch != nil {
		return errors.New("validation failed: Vertex function calls are missing tool results")
	}
	if contentIndex != len(converted.Contents) {
		return errors.New("validation failed: invalid Vertex content alignment")
	}
	converted.Contents = output
	return nil
}

// bindNativeFunctionCalls binds each native functionCall Part to the caller call it was generated from.
// Parameters: content is the converted model turn (its Parts receive exact argument objects) and
// message is the caller's assistant message. Returns the calls in restored native Part order, so
// arguments, signatures and later results all follow the same identity rather than array position.
func bindNativeFunctionCalls(content *gemini.ChatContent, message model.Message) ([]pendingToolCall, error) {
	if message.Role != "assistant" {
		return nil, errors.New("validation failed: Vertex function calls require an assistant message")
	}
	seenIDs := make(map[string]bool, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		if call.Id == "" || seenIDs[call.Id] || call.Function == nil || call.Function.Name == "" {
			return nil, errors.New("validation failed: Vertex function calls require unique IDs and function names")
		}
		seenIDs[call.Id] = true
	}
	calls := make([]pendingToolCall, 0, len(message.ToolCalls))
	seenSources := make(map[int]bool, len(message.ToolCalls))
	for i := range content.Parts {
		part := &content.Parts[i]
		if part.FunctionCall == nil {
			continue
		}
		sourceIndex := part.SourceToolCallIndex
		if sourceIndex == nil || *sourceIndex < 0 || *sourceIndex >= len(message.ToolCalls) || seenSources[*sourceIndex] {
			return nil, errors.New("validation failed: invalid Vertex function call alignment")
		}
		seenSources[*sourceIndex] = true
		call := message.ToolCalls[*sourceIndex]
		if part.FunctionCall.FunctionName != call.Function.Name {
			return nil, errors.New("validation failed: Vertex function call source does not match its native part")
		}
		args, err := vertexFunctionArguments(call.Function)
		if err != nil {
			return nil, err
		}
		// Preserve complete argument objects and number literals without the shared float conversion.
		part.FunctionCall.Arguments = args
		calls = append(calls, pendingToolCall{id: call.Id, name: call.Function.Name})
	}
	if len(calls) != len(message.ToolCalls) {
		return nil, errors.New("validation failed: invalid Vertex function call alignment")
	}
	return calls, nil
}

// vertexFunctionArguments decodes one call's arguments as a complete JSON object.
// Parameters: function is the caller's call. Returns the object with number literals kept as
// json.Number, or a validation error for incomplete, non-object or undecodable arguments.
func vertexFunctionArguments(function *model.Function) (map[string]any, error) {
	raw, err := function.ArgumentsJSON()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed: invalid Vertex function arguments")
	}
	if !json.Valid([]byte(raw)) {
		return nil, errors.New("validation failed: Vertex function arguments must be complete JSON")
	}
	var args map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&args); err != nil || args == nil {
		return nil, errors.New("validation failed: Vertex function arguments must be a JSON object")
	}
	return args, nil
}

// vertexToolResult maps a text result to a protobuf Struct without changing object keys.
// Parameters: message is the correlated tool result. Returns the original JSON object,
// or an output wrapper for text/scalar/array/null values, and an error for media that
// this text-result path cannot represent without discarding information.
func vertexToolResult(message model.Message) (map[string]any, error) {
	// ToolResultParts is the typed media carrier of Responses and native Claude lowering,
	// whose Content holds only text; any media there must fail closed, not be flattened.
	for _, part := range message.ToolResultParts {
		if part.Type != model.ContentTypeText {
			return nil, errors.New("validation failed: Vertex multimodal tool results require native response parts")
		}
	}
	parts, err := message.ParseContentChecked()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed: invalid Vertex tool result content")
	}
	// Build the result from the validated parts themselves, so every content shape the
	// checked parser accepts (including typed []model.MessageContent) keeps its text.
	var builder strings.Builder
	for _, part := range parts {
		if part.Type != model.ContentTypeText || part.Text == nil {
			return nil, errors.New("validation failed: Vertex multimodal tool results require native response parts")
		}
		builder.WriteString(*part.Text)
	}
	text := builder.String()
	var value any
	if json.Valid([]byte(text)) {
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
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
