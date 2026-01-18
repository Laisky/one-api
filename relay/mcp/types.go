package mcp

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ToolDescriptor describes a tool returned by MCP servers.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// UnmarshalJSON decodes MCP tool descriptors while supporting multiple schema field names.
func (t *ToolDescriptor) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("mcp tool descriptor is nil")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool descriptor")
	}
	if name, ok := raw["name"].(string); ok {
		t.Name = name
	}
	if description, ok := raw["description"].(string); ok {
		t.Description = description
	}
	schema := raw["input_schema"]
	if schema == nil {
		schema = raw["inputSchema"]
	}
	if schemaMap, ok := schema.(map[string]any); ok {
		t.InputSchema = schemaMap
		return nil
	}
	if schema != nil {
		encoded, err := json.Marshal(schema)
		if err != nil {
			return errors.Wrap(err, "marshal mcp tool schema")
		}
		var parsed map[string]any
		if err := json.Unmarshal(encoded, &parsed); err != nil {
			return errors.Wrap(err, "decode mcp tool schema")
		}
		t.InputSchema = parsed
	}
	return nil
}

// CallToolResult represents a MCP tool call response.
type CallToolResult struct {
	Content any             `json:"content"`
	IsError bool            `json:"is_error,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

// UnmarshalJSON parses the MCP tool call result while keeping the raw payload.
func (c *CallToolResult) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("mcp tool result is nil")
	}
	c.Raw = append(c.Raw[:0], data...)
	var parsed struct {
		Content any  `json:"content"`
		IsError bool `json:"is_error,omitempty"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool result")
	}
	c.Content = parsed.Content
	c.IsError = parsed.IsError
	return nil
}
