package openai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// responseCustomToolInputArgument is the single string argument that carries a
// custom (freeform) tool's raw input through function-only upstreams.
const responseCustomToolInputArgument = "input"

// responseCustomToolGatewayNote reconciles freeform tool descriptions ("do not
// wrap the input in JSON") with the function-call envelope fallback must use.
const responseCustomToolGatewayNote = "This tool takes freeform text. Pass the complete raw text, unchanged and without Markdown fences, as the string argument `input`; the gateway delivers that string to the tool verbatim."

// responseFunctionToolToChat converts a Responses function definition into a
// Chat function tool with the established fallback parameter sanitation.
// Parameters: tool is a function definition. Returns nil for an empty definition.
func responseFunctionToolToChat(tool ResponseAPITool) *model.Tool {
	fn := sanitizeFunctionForRequest(tool)
	if fn == nil {
		return nil
	}
	fn.Strict = nil
	params := map[string]any{}
	if fn.Parameters != nil {
		sanitized := sanitizeResponseAPIFunctionParameters(fn.Parameters)
		if parsed, ok := sanitized.(map[string]any); ok && parsed != nil {
			params = parsed
		}
	}
	if typeName, ok := params["type"].(string); !ok || strings.TrimSpace(typeName) == "" {
		params["type"] = "object"
	}
	fn.Parameters = params
	return &model.Tool{Type: "function", Function: fn}
}

// responseCustomToolToChat converts a Responses custom (freeform or grammar)
// tool into a Chat function tool whose only argument is the raw input string.
// The grammar or text format is preserved in the description so the upstream
// model can produce conforming input. Returns nil for an unnamed definition.
func responseCustomToolToChat(tool ResponseAPITool) *model.Tool {
	name := tool.Name
	description := tool.Description
	if tool.Function != nil {
		if tool.Function.Name != "" {
			name = tool.Function.Name
		}
		if tool.Function.Description != "" {
			description = tool.Function.Description
		}
	}
	if strings.TrimSpace(name) == "" {
		return nil
	}
	parts := []string{strings.TrimSpace(description), responseCustomToolGatewayNote}
	if format := describeResponseCustomToolFormat(tool.Format); format != "" {
		parts = append(parts, format)
	}
	return &model.Tool{
		Type: "function",
		Function: &model.Function{
			Name:        name,
			Description: strings.TrimSpace(strings.Join(parts, "\n\n")),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					responseCustomToolInputArgument: map[string]any{
						"type":        "string",
						"description": "The complete raw input for the tool, in the format the tool description requires.",
					},
				},
				"required": []any{responseCustomToolInputArgument},
			},
		},
	}
}

// describeResponseCustomToolFormat renders a custom tool format for the model.
// Parameters: raw is the custom tool "format" object. Returns "" for text or
// missing formats, otherwise a grammar description including its definition.
func describeResponseCustomToolFormat(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var format struct {
		Type       string `json:"type"`
		Syntax     string `json:"syntax"`
		Definition string `json:"definition"`
	}
	if err := json.Unmarshal(raw, &format); err != nil {
		return ""
	}
	if !strings.EqualFold(format.Type, "grammar") || strings.TrimSpace(format.Definition) == "" {
		return ""
	}
	syntax := strings.TrimSpace(format.Syntax)
	if syntax == "" {
		syntax = "formal"
	}
	return "The input must match this " + syntax + " grammar exactly:\n" + format.Definition
}

// responseSearchToolToChat converts a client-executed tool_search definition
// into a Chat function tool whose arguments are forwarded to the client as the
// tool_search_call arguments object.
func responseSearchToolToChat(tool ResponseAPITool) *model.Tool {
	description := tool.Description
	if strings.TrimSpace(description) == "" {
		description = "Search the client's deferred tool catalog. Matching tools become callable on the next turn."
	}
	params := map[string]any{}
	if tool.Parameters != nil {
		if parsed, ok := sanitizeResponseAPIFunctionParameters(tool.Parameters).(map[string]any); ok && parsed != nil {
			params = parsed
		}
	}
	if len(params) == 0 {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
			"required":   []any{"query"},
		}
	}
	if typeName, ok := params["type"].(string); !ok || strings.TrimSpace(typeName) == "" {
		params["type"] = "object"
	}
	return &model.Tool{
		Type:     "function",
		Function: &model.Function{Name: "tool_search", Description: description, Parameters: params},
	}
}
