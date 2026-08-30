package mcp

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ToolDescriptor describes a tool returned by MCP servers.
type ToolDescriptor struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  map[string]any   `json:"inputSchema,omitempty"`
	OutputSchema map[string]any   `json:"outputSchema,omitempty"`
	Annotations  map[string]any   `json:"annotations,omitempty"`
	Icons        []map[string]any `json:"icons,omitempty"`
	Meta         map[string]any   `json:"_meta,omitempty"`
}

// UnmarshalJSON decodes MCP tool descriptors while supporting legacy schema field names.
func (t *ToolDescriptor) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("mcp tool descriptor is nil")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool descriptor")
	}
	t.Name, _ = raw["name"].(string)
	t.Title, _ = raw["title"].(string)
	t.Description, _ = raw["description"].(string)
	t.InputSchema = decodeSchemaMap(raw, "inputSchema", "input_schema")
	t.OutputSchema = decodeSchemaMap(raw, "outputSchema", "output_schema")
	t.Annotations, _ = raw["annotations"].(map[string]any)
	if icons, ok := raw["icons"].([]any); ok {
		t.Icons = make([]map[string]any, 0, len(icons))
		for _, icon := range icons {
			if object, ok := icon.(map[string]any); ok {
				t.Icons = append(t.Icons, object)
			}
		}
	}
	t.Meta, _ = raw["_meta"].(map[string]any)
	return nil
}

// ListToolsResult represents a modern tools/list response.
type ListToolsResult struct {
	ResultType string           `json:"resultType,omitempty"`
	Tools      []ToolDescriptor `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
	TTLMS      int64            `json:"ttlMs,omitempty"`
	CacheScope string           `json:"cacheScope,omitempty"`
	Meta       map[string]any   `json:"_meta,omitempty"`
}

// CallToolRequestOptions carries MCP 2026-07-28 multi-round-trip retry fields.
type CallToolRequestOptions struct {
	InputResponses map[string]any `json:"inputResponses,omitempty"`
	RequestState   string         `json:"requestState,omitempty"`
}

// CallToolResult represents an MCP tool call response across modern and legacy protocol versions.
type CallToolResult struct {
	ResultType        string          `json:"resultType,omitempty"`
	Content           any             `json:"content"`
	StructuredContent any             `json:"structured_content,omitempty"`
	IsError           bool            `json:"is_error,omitempty"`
	InputRequests     map[string]any  `json:"input_requests,omitempty"`
	RequestState      string          `json:"request_state,omitempty"`
	Meta              map[string]any  `json:"_meta,omitempty"`
	Raw               json.RawMessage `json:"-"`
}

// UnmarshalJSON parses MCP tool call results while accepting legacy snake_case aliases.
func (c *CallToolResult) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("mcp tool result is nil")
	}
	c.Raw = append(c.Raw[:0], data...)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool result")
	}
	if encoded := raw["resultType"]; len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.ResultType); err != nil {
			return errors.Wrap(err, "decode mcp result type")
		}
	}
	if encoded := raw["content"]; len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.Content); err != nil {
			return errors.Wrap(err, "decode mcp result content")
		}
	}
	if encoded := firstRawMessage(raw, "structuredContent", "structured_content"); len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.StructuredContent); err != nil {
			return errors.Wrap(err, "decode mcp structured content")
		}
	}
	if encoded := firstRawMessage(raw, "isError", "is_error"); len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.IsError); err != nil {
			return errors.Wrap(err, "decode mcp result error flag")
		}
	}
	if encoded := firstRawMessage(raw, "inputRequests", "input_requests"); len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.InputRequests); err != nil {
			return errors.Wrap(err, "decode mcp input requests")
		}
	}
	if encoded := firstRawMessage(raw, "requestState", "request_state"); len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.RequestState); err != nil {
			return errors.Wrap(err, "decode mcp request state")
		}
	}
	if encoded := raw["_meta"]; len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &c.Meta); err != nil {
			return errors.Wrap(err, "decode mcp result metadata")
		}
	}
	return nil
}

// NormalizeCallToolResult fills protocol-required defaults without changing tool payloads.
func NormalizeCallToolResult(result *CallToolResult) *CallToolResult {
	if result == nil {
		return &CallToolResult{ResultType: ResultTypeComplete}
	}
	if result.ResultType == "" {
		result.ResultType = ResultTypeComplete
	}
	return result
}

// decodeSchemaMap extracts a JSON object using modern and legacy field names.
func decodeSchemaMap(raw map[string]any, names ...string) map[string]any {
	for _, name := range names {
		value := raw[name]
		if value == nil {
			continue
		}
		if schema, ok := value.(map[string]any); ok {
			return schema
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err == nil {
			return schema
		}
	}
	return nil
}

// firstRawMessage returns the first present alias from a JSON object.
func firstRawMessage(raw map[string]json.RawMessage, names ...string) json.RawMessage {
	for _, name := range names {
		if encoded := raw[name]; len(encoded) != 0 {
			return encoded
		}
	}
	return nil
}
