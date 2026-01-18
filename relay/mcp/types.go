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
