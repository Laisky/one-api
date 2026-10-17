package openai

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/model"
)

// MaxResponseNamespaceTools bounds local definitions decoded in one group.
const MaxResponseNamespaceTools = 1024

// ResponseAPITool represents the tool format for Response API requests
// This differs from the ChatCompletion tool format where function properties are nested
// Supports both function tools and MCP tools
type ResponseAPITool struct {
	Execution   string          `json:"execution,omitempty"`   // Client/server discovery discriminator.
	Type        string          `json:"type"`                  // Required: "function", "web_search", "mcp", etc.
	Name        string          `json:"name,omitempty"`        // Legacy: function name when function block absent
	Description string          `json:"description,omitempty"` // Legacy: function description when function block absent
	Parameters  map[string]any  `json:"parameters,omitempty"`  // Legacy: function parameters when function block absent
	Function    *model.Function `json:"function,omitempty"`    // Modern function definition (preferred for Response API)

	// Local namespace children and custom-tool wire metadata.
	Tools        []ResponseAPITool `json:"tools,omitempty"`
	Format       json.RawMessage   `json:"format,omitempty"`
	Strict       *bool             `json:"strict,omitempty"`
	DeferLoading *bool             `json:"defer_loading,omitempty"`

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
		if t.Strict != nil {
			payload["strict"] = t.Strict
		}
		if t.DeferLoading != nil {
			payload["defer_loading"] = t.DeferLoading
		}
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
		return marshalResponseAPIToolPayload(payload)
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
		return marshalResponseAPIToolPayload(payload)
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
		return marshalResponseAPIToolPayload(payload)
	default:
		type alias ResponseAPITool
		return marshalResponseAPIToolPayload(alias(t))
	}
}

// marshalResponseAPIToolPayload encodes one tool payload for ResponseAPITool.MarshalJSON.
//
// Parameters:
//   - payload: the tool's wire representation.
//
// Return values:
//   - []byte: the encoded payload.
//   - error: the encoding failure, wrapped with a stack; its text and errors.As identity are unchanged.
func marshalResponseAPIToolPayload(payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	return encoded, errors.WithStack(err)
}

func (t *ResponseAPITool) UnmarshalJSON(data []byte) error {
	type rawTool struct {
		Execution         string                  `json:"execution,omitempty"`
		Type              string                  `json:"type"`
		Tools             []json.RawMessage       `json:"tools,omitempty"`
		Format            json.RawMessage         `json:"format,omitempty"`
		Strict            *bool                   `json:"strict,omitempty"`
		DeferLoading      *bool                   `json:"defer_loading,omitempty"`
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

	// Every tool is its own raw/typed admission boundary, including children.
	if err := common.ValidateUnambiguousJSONRootKeys(data, rawTool{}); err != nil {
		return errors.Wrap(err, "validate response tool keys")
	}
	var raw rawTool
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal response api tool")
	}

	// Decode at most one namespace level. Inspect children before invoking this
	// decoder again, so malformed deep groups cannot consume recursive stacks.
	if len(raw.Tools) > MaxResponseNamespaceTools {
		return errors.New("response namespace tool count exceeds limit")
	}
	var children []ResponseAPITool
	if len(raw.Tools) > 0 {
		children = make([]ResponseAPITool, 0, len(raw.Tools))
	}
	for _, child := range raw.Tools {
		var shape struct {
			Type  string          `json:"type"`
			Tools json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(child, &shape); err != nil {
			return errors.Wrap(err, "decode response namespace child")
		}
		if strings.EqualFold(strings.TrimSpace(shape.Type), "namespace") || len(shape.Tools) > 0 {
			return errors.New("nested response namespace tools are not supported")
		}
		var decoded ResponseAPITool
		if err := json.Unmarshal(child, &decoded); err != nil {
			return errors.Wrap(err, "decode response namespace tool")
		}
		children = append(children, decoded)
	}
	t.Tools = children
	t.Format = raw.Format
	t.Strict = raw.Strict
	t.DeferLoading = raw.DeferLoading
	t.Execution = raw.Execution
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
		}
		// model.Function.Parameters is `any`, and omitempty does not drop an
		// interface holding a nil map — assigning raw.Parameters unconditionally put
		// `"parameters": null` inside the function block for every flattened tool
		// that carried a name but no parameters.
		if len(raw.Parameters) > 0 {
			t.Function.Parameters = raw.Parameters
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
		}
		// See ResponseAPITool.UnmarshalJSON: a nil map assigned into the `any`
		// Parameters field serializes as null rather than being omitted.
		if len(tool.Parameters) > 0 {
			fn.Parameters = tool.Parameters
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
