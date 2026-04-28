package gemini

import (
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

var claudeStructuredIntentKeywords = []string{"json", "structured", "schema", "fields"}

// claudeStructuredResponseFormat detects Claude structured-output requests that should use Gemini's native responseSchema.
// Parameters: request is the incoming Claude Messages payload.
// Returns: a promoted OpenAI-style response_format or nil when the request should remain tool-based.
func claudeStructuredResponseFormat(request *model.ClaudeRequest) *model.ResponseFormat {
	if request == nil || len(request.Tools) != 1 || claudeMessagesContainToolUsage(request.Messages) {
		return nil
	}

	tool := request.Tools[0]
	choiceName, hasChoice := claudeToolChoiceName(request.ToolChoice)
	if !hasChoice || !strings.EqualFold(strings.TrimSpace(choiceName), strings.TrimSpace(tool.Name)) {
		return nil
	}

	schemaMap, ok := tool.InputSchema.(map[string]any)
	if !ok || len(schemaMap) == 0 || !claudeSchemaLooksStructured(schemaMap) {
		return nil
	}

	if !claudeRequestHasStructuredIntent(request, tool.Description) {
		return nil
	}

	strict := true
	return &model.ResponseFormat{
		Type: "json_schema",
		JsonSchema: &model.JSONSchema{
			Name:        tool.Name,
			Description: strings.TrimSpace(tool.Description),
			Schema:      schemaMap,
			Strict:      &strict,
		},
	}
}

// claudeMessagesContainToolUsage reports whether the Claude message history already contains tool use or tool result blocks.
// Parameters: messages is the Claude Messages history.
// Returns: true when the request is a real tool flow that should not be promoted to responseSchema.
func claudeMessagesContainToolUsage(messages []model.ClaudeMessage) bool {
	for _, msg := range messages {
		contentBlocks, ok := msg.Content.([]any)
		if !ok {
			continue
		}
		for _, entry := range contentBlocks {
			block, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := block["type"].(string)
			switch strings.ToLower(strings.TrimSpace(typeStr)) {
			case "tool_use", "tool_result", "server_tool_use", "tool_search_tool_result":
				return true
			}
		}
	}

	return false
}

// claudeToolChoiceName extracts the selected tool name from a Claude tool_choice payload.
// Parameters: toolChoice is the Claude tool_choice field, which may be a string or a structured object.
// Returns: the selected tool name and whether a usable name was found.
func claudeToolChoiceName(toolChoice any) (string, bool) {
	switch value := toolChoice.(type) {
	case string:
		name := strings.TrimSpace(value)
		if name == "" {
			return "", false
		}
		return name, true
	case map[string]any:
		if name, ok := value["name"].(string); ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name), true
		}
		if fn, ok := value["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
				return strings.TrimSpace(name), true
			}
		}
	}

	return "", false
}

// claudeSchemaLooksStructured verifies that the schema carries the explicit structured-output signal used by the harness.
// Parameters: schema is the tool input schema associated with the Claude request.
// Returns: true when the schema sets additionalProperties to false.
func claudeSchemaLooksStructured(schema map[string]any) bool {
	if schema == nil {
		return false
	}

	additionalProperties, exists := schema["additionalProperties"]
	if !exists {
		return false
	}

	strictObject, ok := additionalProperties.(bool)
	return ok && !strictObject
}

// claudeRequestHasStructuredIntent checks for JSON/structured-output cues in the request prompt or tool description.
// Parameters: request is the Claude request and toolDescription is the selected tool description.
// Returns: true when the user intent clearly requests structured JSON output.
func claudeRequestHasStructuredIntent(request *model.ClaudeRequest, toolDescription string) bool {
	if claudeTextContainsStructuredIntent(toolDescription) {
		return true
	}
	if request == nil {
		return false
	}
	if request.System != nil && claudeTextContainsStructuredIntent(claudeContentText(request.System)) {
		return true
	}
	for _, msg := range request.Messages {
		if claudeTextContainsStructuredIntent(claudeContentText(msg.Content)) {
			return true
		}
	}

	return false
}

// claudeTextContainsStructuredIntent reports whether text contains structured-output intent keywords.
// Parameters: text is the text blob extracted from the Claude request.
// Returns: true when any structured-output keyword is present.
func claudeTextContainsStructuredIntent(text string) bool {
	lowerText := strings.ToLower(strings.TrimSpace(text))
	if lowerText == "" {
		return false
	}

	for _, keyword := range claudeStructuredIntentKeywords {
		if strings.Contains(lowerText, keyword) {
			return true
		}
	}

	return false
}

// claudeContentText flattens Claude content payloads into a single text blob for intent detection.
// Parameters: content is a Claude content field that may contain strings, blocks, maps, or arrays.
// Returns: a newline-joined text representation of the payload.
func claudeContentText(content any) string {
	parts := make([]string, 0, 4)
	collectClaudeContentText(content, &parts)
	return strings.Join(parts, "\n")
}

// collectClaudeContentText recursively gathers textual fields from Claude content payloads.
// Parameters: content is the current JSON-like node to inspect and parts accumulates extracted text fragments.
// Returns: no values; parts is updated in place.
func collectClaudeContentText(content any, parts *[]string) {
	switch value := content.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			*parts = append(*parts, trimmed)
		}
	case []any:
		for _, item := range value {
			collectClaudeContentText(item, parts)
		}
	case map[string]any:
		if text, ok := value["text"].(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				*parts = append(*parts, trimmed)
			}
		}
		for _, nested := range value {
			collectClaudeContentText(nested, parts)
		}
	}
}
