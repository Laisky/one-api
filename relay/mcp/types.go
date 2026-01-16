package mcp

// ToolDescriptor describes a tool returned by MCP servers.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// CallToolResult represents a MCP tool call response.
type CallToolResult struct {
	Content any  `json:"content"`
	IsError bool `json:"is_error,omitempty"`
}
