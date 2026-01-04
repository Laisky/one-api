package openai

// ResponseAPIPrompt represents the prompt template configuration for Response API requests
type ResponseAPIPrompt struct {
	Id        string         `json:"id"`                  // Required: Unique identifier of the prompt template
	Version   *string        `json:"version,omitempty"`   // Optional: Specific version of the prompt (defaults to "current")
	Variables map[string]any `json:"variables,omitempty"` // Optional: Map of values to substitute in for variables in the prompt
}

// ResponseAPIRequiredAction represents the required action block in Response API responses
type ResponseAPIRequiredAction struct {
	Type              string                        `json:"type"`
	SubmitToolOutputs *ResponseAPISubmitToolOutputs `json:"submit_tool_outputs,omitempty"`
}

// ResponseAPISubmitToolOutputs contains the tool calls that must be fulfilled by the client
type ResponseAPISubmitToolOutputs struct {
	ToolCalls []ResponseAPIToolCall `json:"tool_calls,omitempty"`
}

// ResponseAPIToolCall represents a single tool call the model wants to execute
type ResponseAPIToolCall struct {
	Id       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function *ResponseAPIFunctionCall `json:"function,omitempty"`
}

// ResponseAPIFunctionCall captures the function invocation details in a tool call
type ResponseAPIFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// WebSearchCallAction captures metadata about a single web search invocation emitted by the OpenAI Responses API.
type WebSearchCallAction struct {
	Type    string                `json:"type,omitempty"`
	Query   string                `json:"query,omitempty"`
	Domains []string              `json:"domains,omitempty"`
	Sources []WebSearchCallSource `json:"sources,omitempty"`
}

// WebSearchCallSource represents an individual source returned by the web search tool.
type WebSearchCallSource struct {
	Url   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

// ResponseTextConfig represents the text configuration for Response API
type ResponseTextConfig struct {
	Format    *ResponseTextFormat `json:"format,omitempty"`    // Optional: Format configuration for structured outputs
	Verbosity *string             `json:"verbosity,omitempty"` // Optional: Verbosity level (low, medium, high)
}

// ResponseTextFormat represents the format configuration for Response API structured outputs
type ResponseTextFormat struct {
	Type        string         `json:"type"`                  // Required: Format type (e.g., "text", "json_schema")
	Name        string         `json:"name,omitempty"`        // Optional: Schema name for json_schema type
	Description string         `json:"description,omitempty"` // Optional: Schema description
	Schema      map[string]any `json:"schema,omitempty"`      // Optional: JSON schema definition
	Strict      *bool          `json:"strict,omitempty"`      // Optional: Whether to use strict mode
}

// MCPApprovalResponseInput represents the input structure for MCP approval responses
// Used when responding to mcp_approval_request output items to approve or deny MCP tool calls
type MCPApprovalResponseInput struct {
	Type              string `json:"type"`                // Required: Always "mcp_approval_response"
	Approve           bool   `json:"approve"`             // Required: Whether to approve the MCP tool call
	ApprovalRequestId string `json:"approval_request_id"` // Required: ID of the approval request being responded to
}
