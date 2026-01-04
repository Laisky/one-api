package openai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/model"
)

// ResponseAPITool represents the tool format for Response API requests
// This differs from the ChatCompletion tool format where function properties are nested
// Supports both function tools and MCP tools
type ResponseAPITool struct {
	Type        string          `json:"type"`                  // Required: "function", "web_search", "mcp", etc.
	Name        string          `json:"name,omitempty"`        // Legacy: function name when function block absent
	Description string          `json:"description,omitempty"` // Legacy: function description when function block absent
	Parameters  map[string]any  `json:"parameters,omitempty"`  // Legacy: function parameters when function block absent
	Function    *model.Function `json:"function,omitempty"`    // Modern function definition (preferred for Response API)

	// Web-search specific configuration
	SearchContextSize *string                 `json:"search_context_size,omitempty"`
	Filters           *model.WebSearchFilters `json:"filters,omitempty"`
	UserLocation      *model.UserLocation     `json:"user_location,omitempty"`

	// MCP-specific fields (for MCP tools)
	ServerLabel     string            `json:"server_label,omitempty"`
	ServerUrl       string            `json:"server_url,omitempty"`
	RequireApproval any               `json:"require_approval,omitempty"`
	AllowedTools    []string          `json:"allowed_tools,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
}

func (t ResponseAPITool) MarshalJSON() ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(t.Type)) {
	case "function":
		fn := sanitizeFunctionForRequest(t)
		payload := map[string]any{"type": "function"}
		if fn != nil {
			if name := strings.TrimSpace(fn.Name); name != "" {
				payload["name"] = name
			}
			if desc := strings.TrimSpace(fn.Description); desc != "" {
				payload["description"] = desc
			}
			if params, ok := fn.Parameters.(map[string]any); ok && len(params) > 0 {
				payload["parameters"] = params
			}
		}
		if fn != nil {
			payload["function"] = fn
		}
		return json.Marshal(payload)
	case "web_search":
		payload := map[string]any{"type": t.Type}
		if t.SearchContextSize != nil {
			payload["search_context_size"] = t.SearchContextSize
		}
		if t.Filters != nil {
			payload["filters"] = t.Filters
		}
		if t.UserLocation != nil {
			payload["user_location"] = t.UserLocation
		}
		return json.Marshal(payload)
	case "mcp":
		payload := map[string]any{"type": t.Type}
		if t.ServerLabel != "" {
			payload["server_label"] = t.ServerLabel
		}
		if t.ServerUrl != "" {
			payload["server_url"] = t.ServerUrl
		}
		if t.RequireApproval != nil {
			payload["require_approval"] = t.RequireApproval
		}
		if len(t.AllowedTools) > 0 {
			payload["allowed_tools"] = t.AllowedTools
		}
		if len(t.Headers) > 0 {
			payload["headers"] = t.Headers
		}
		return json.Marshal(payload)
	default:
		type alias ResponseAPITool
		return json.Marshal(alias(t))
	}
}

func (t *ResponseAPITool) UnmarshalJSON(data []byte) error {
	type rawTool struct {
		Type              string                  `json:"type"`
		Name              string                  `json:"name,omitempty"`
		Description       string                  `json:"description,omitempty"`
		Parameters        map[string]any          `json:"parameters,omitempty"`
		Function          json.RawMessage         `json:"function,omitempty"`
		SearchContextSize *string                 `json:"search_context_size,omitempty"`
		Filters           *model.WebSearchFilters `json:"filters,omitempty"`
		UserLocation      *model.UserLocation     `json:"user_location,omitempty"`
		ServerLabel       string                  `json:"server_label,omitempty"`
		ServerUrl         string                  `json:"server_url,omitempty"`
		RequireApproval   any                     `json:"require_approval,omitempty"`
		AllowedTools      []string                `json:"allowed_tools,omitempty"`
		Headers           map[string]string       `json:"headers,omitempty"`
	}

	var raw rawTool
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal response api tool")
	}

	t.Type = raw.Type
	t.Name = raw.Name
	t.Description = raw.Description
	t.Parameters = raw.Parameters
	t.SearchContextSize = raw.SearchContextSize
	t.Filters = raw.Filters
	t.UserLocation = raw.UserLocation
	t.ServerLabel = raw.ServerLabel
	t.ServerUrl = raw.ServerUrl
	t.RequireApproval = raw.RequireApproval
	t.AllowedTools = raw.AllowedTools
	t.Headers = raw.Headers
	t.Function = nil

	if len(raw.Function) > 0 {
		var fn model.Function
		if err := json.Unmarshal(raw.Function, &fn); err != nil {
			return errors.Wrap(err, "unmarshal response api tool.function")
		}
		t.Function = sanitizeDecodedFunction(&fn)
	} else if raw.Type == "function" && (raw.Name != "" || raw.Description != "" || raw.Parameters != nil) {
		t.Function = &model.Function{
			Name:        raw.Name,
			Description: raw.Description,
			Parameters:  raw.Parameters,
		}
	}

	// Keep legacy fields in sync when function block is present
	if t.Function != nil {
		if t.Function.Name != "" {
			t.Name = t.Function.Name
		}
		if t.Function.Description != "" {
			t.Description = t.Function.Description
		}
		if params, ok := t.Function.Parameters.(map[string]any); ok {
			t.Parameters = params
		}
	}

	return nil
}

func sanitizeFunctionForRequest(tool ResponseAPITool) *model.Function {
	fn := tool.Function
	if fn == nil && (tool.Name != "" || tool.Description != "" || tool.Parameters != nil) {
		fn = &model.Function{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
	}
	if fn == nil {
		return nil
	}
	clone := *fn
	clone.Arguments = nil
	return &clone
}

func sanitizeResponseAPIFunctionParameters(params any) any {
	return sanitizeResponseAPIFunctionParametersWithDepth(params, 0)
}

func sanitizeResponseAPIFunctionParametersWithDepth(params any, depth int) any {
	switch v := params.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for key, raw := range v {
			lowerKey := strings.ToLower(key)
			if key == "$schema" || lowerKey == "additionalproperties" {
				continue
			}
			if depth == 0 && (lowerKey == "description" || lowerKey == "strict") {
				continue
			}
			cleaned[key] = sanitizeResponseAPIFunctionParametersWithDepth(raw, depth+1)
		}
		if len(cleaned) == 0 {
			return map[string]any{}
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(v))
		for _, item := range v {
			cleaned = append(cleaned, sanitizeResponseAPIFunctionParametersWithDepth(item, depth+1))
		}
		return cleaned
	default:
		return params
	}
}

func sanitizeResponseAPIJSONSchema(schema any) any {
	return sanitizeResponseAPIFunctionParameters(schema)
}

func sanitizeDecodedFunction(fn *model.Function) *model.Function {
	if fn == nil {
		return nil
	}
	// No special handling required today; keep hook for future sanitation.
	return fn
}
