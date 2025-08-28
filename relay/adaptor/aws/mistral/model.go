package aws

// Request is the request to AWS Mistral Large
//
// Based on AWS Bedrock Mistral Large documentation
type Request struct {
	Messages    []Message   `json:"messages"`
	Tools       []Tool      `json:"tools,omitempty"`
	ToolChoice  interface{} `json:"tool_choice,omitempty"` // "auto"|"any"|"none" or object
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature *float64    `json:"temperature,omitempty"`
	TopP        *float64    `json:"top_p,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	Role       string     `json:"role"`                   // "system"|"user"|"assistant"|"tool"
	Content    string     `json:"content,omitempty"`      // Message content
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // For assistant messages with tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"` // For tool messages
}

// ToolCall represents a tool call in assistant messages
type ToolCall struct {
	ID       string   `json:"id"`
	Function Function `json:"function"`
}

// Function represents the function details in a tool call
type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// Tool represents a tool definition
type Tool struct {
	Type     string   `json:"type"` // "function"
	Function ToolSpec `json:"function"`
}

// ToolSpec represents the specification of a tool function
type ToolSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON schema
}

// Response is the response from AWS Mistral Large
//
// Based on AWS Bedrock Mistral Large documentation
type Response struct {
	Choices []Choice `json:"choices"`
}

// Choice represents a single choice in the response
type Choice struct {
	Index      int             `json:"index"`
	Message    ResponseMessage `json:"message"`
	StopReason string          `json:"stop_reason"` // "stop"|"length"|"tool_calls"
}

// ResponseMessage represents the message in the response
type ResponseMessage struct {
	Role      string     `json:"role"`                 // "assistant"
	Content   string     `json:"content,omitempty"`    // Response content
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // Tool calls if stop_reason is "tool_calls"
}

// StreamResponse represents a streaming response chunk
type StreamResponse struct {
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice represents a streaming choice
type StreamChoice struct {
	Index      int                   `json:"index"`
	Delta      StreamResponseMessage `json:"delta"`
	StopReason string                `json:"stop_reason,omitempty"`
}

// StreamResponseMessage represents the delta message in streaming response
type StreamResponseMessage struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Converse API structures for streaming (similar to Nova)
type MistralConverseStreamResponse struct {
	MessageStart      *MistralMessageStart      `json:"messageStart,omitempty"`
	ContentBlockDelta *MistralContentBlockDelta `json:"contentBlockDelta,omitempty"`
	MessageStop       *MistralMessageStop       `json:"messageStop,omitempty"`
	Metadata          *MistralStreamMetadata    `json:"metadata,omitempty"`
}

type MistralMessageStart struct {
	Role string `json:"role"`
}

type MistralContentBlockDelta struct {
	Delta MistralContentDelta `json:"delta"`
}

type MistralContentDelta struct {
	Text string `json:"text"`
}

type MistralMessageStop struct {
	StopReason string `json:"stopReason"`
}

type MistralStreamMetadata struct {
	Usage MistralUsage `json:"usage"`
}

type MistralUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// Converse API request structures
type MistralConverseMessage struct {
	Role    string                        `json:"role"`
	Content []MistralConverseContentBlock `json:"content"`
}

type MistralConverseContentBlock struct {
	Text string `json:"text"`
}

type MistralConverseSystemMessage struct {
	Text string `json:"text"`
}

type MistralConverseInferenceConfig struct {
	MaxTokens     int      `json:"maxTokens"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}
