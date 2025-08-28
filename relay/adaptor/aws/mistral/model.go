package aws

// Request represents a chat completion request to AWS Bedrock Mistral Large.
//
// This structure defines all the parameters needed to send a chat completion
// request to the Mistral Large model via AWS Bedrock. It supports both
// simple text conversations and advanced tool calling workflows.
//
// Based on AWS Bedrock Mistral Large documentation.
type Request struct {
	// Messages contains the conversation history including system, user, assistant, and tool messages.
	// This field is required and must contain at least one message.
	Messages []Message `json:"messages"`

	// Tools defines the available functions that the model can call during the conversation.
	// Optional field that enables function calling capabilities.
	Tools []Tool `json:"tools,omitempty"`

	// ToolChoice controls how the model should use the available tools.
	// Can be "auto" (model decides), "any" (must use a tool), "none" (no tools),
	// or a specific tool choice object. Optional field.
	ToolChoice interface{} `json:"tool_choice,omitempty"`

	// MaxTokens specifies the maximum number of tokens to generate in the response.
	// Optional field that helps control response length and API costs.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls the randomness of the model's responses.
	// Range: 0.0 to 1.0, where 0.0 is deterministic and 1.0 is most random.
	// Optional field, uses model default if not specified.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, limiting the cumulative probability of token choices.
	// Range: 0.0 to 1.0, where lower values make responses more focused.
	// Optional field, uses model default if not specified.
	TopP *float64 `json:"top_p,omitempty"`
}

// Message represents a single message in the conversation history.
//
// Messages form the core of the chat completion request, containing the back-and-forth
// conversation between different participants. Each message has a specific role that
// determines its purpose and expected content format.
type Message struct {
	// Role identifies the sender of the message.
	// Valid values: "system" (instructions), "user" (human input),
	// "assistant" (model response), "tool" (function result).
	Role string `json:"role"`

	// Content contains the text content of the message.
	// Required for system, user, and assistant messages (when not using tool calls).
	// Optional for tool messages where tool_call_id is used instead.
	Content string `json:"content,omitempty"`

	// ToolCalls contains function calls made by the assistant.
	// Only used in assistant messages when the model decides to call functions.
	// Each tool call includes an ID and function details.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID references the ID of the tool call this message responds to.
	// Only used in tool messages to provide the result of a function execution.
	// Must match the ID from a previous assistant's tool call.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call made by the assistant during conversation.
//
// When the model decides to use a tool, it generates a ToolCall with a unique ID
// and function details. The calling application should execute the function and
// return the result in a subsequent tool message.
type ToolCall struct {
	// ID is a unique identifier for this specific tool call.
	// Used to match tool responses with their corresponding calls.
	ID string `json:"id"`

	// Function contains the details of the function to be called,
	// including the function name and JSON-encoded arguments.
	Function Function `json:"function"`
}

// Function represents the specific function details within a tool call.
//
// This structure contains the function name and its arguments in JSON format,
// providing all the information needed to execute the requested function.
type Function struct {
	// Name is the identifier of the function to be called.
	// Must match one of the function names defined in the Tools array.
	Name string `json:"name"`

	// Arguments contains the function parameters encoded as a JSON string.
	// The structure of this JSON should match the parameters schema
	// defined in the corresponding ToolSpec.
	Arguments string `json:"arguments"`
}

// Tool represents a function definition available to the model.
//
// Tools enable the model to call external functions during conversation,
// extending its capabilities beyond text generation. Each tool defines
// a function that can be invoked with specific parameters.
type Tool struct {
	// Type specifies the kind of tool being defined.
	// Currently only "function" is supported by the Mistral API.
	Type string `json:"type"`

	// Function contains the detailed specification of the callable function,
	// including its name, description, and parameter schema.
	Function ToolSpec `json:"function"`
}

// ToolSpec defines the specification and schema for a callable function.
//
// This structure provides all the metadata the model needs to understand
// how and when to call a function, including its purpose and expected parameters.
type ToolSpec struct {
	// Name is the unique identifier for this function.
	// Used by the model when generating ToolCall instances.
	Name string `json:"name"`

	// Description explains what this function does and when to use it.
	// Helps the model make informed decisions about tool usage.
	Description string `json:"description"`

	// Parameters defines the JSON schema for the function's input parameters.
	// Should be a valid JSON Schema object describing expected arguments.
	Parameters interface{} `json:"parameters"`
}

// Response represents the complete response from AWS Bedrock Mistral Large.
//
// This structure contains the model's response to a chat completion request,
// including generated text and/or tool calls. The response follows the standard
// OpenAI-compatible format with choices array for potential multiple responses.
//
// Based on AWS Bedrock Mistral Large documentation.
type Response struct {
	// Choices contains the possible response options generated by the model.
	// Typically contains a single choice, but the array format maintains
	// compatibility with OpenAI's API structure.
	Choices []Choice `json:"choices"`
}

// Choice represents a single response option from the model.
//
// Each choice contains the generated content along with metadata about
// why the generation stopped. This structure allows for multiple response
// alternatives, though Mistral typically returns only one choice.
type Choice struct {
	// Index identifies the position of this choice in the choices array.
	// Typically 0 for the first (and usually only) choice.
	Index int `json:"index"`

	// Message contains the actual response content from the assistant.
	// This includes either text content or tool calls, depending on the model's decision.
	Message ResponseMessage `json:"message"`

	// StopReason indicates why the model stopped generating tokens.
	// Valid values: "stop" (natural completion), "length" (max tokens reached),
	// "tool_calls" (model wants to call functions).
	StopReason string `json:"stop_reason"`
}

// ResponseMessage represents the assistant's message in the response.
//
// This structure contains the model's generated content, which can be either
// text content for a regular response or tool calls when the model decides
// to invoke functions. The role is always "assistant" for response messages.
type ResponseMessage struct {
	// Role identifies the message sender, always "assistant" for responses.
	// This maintains consistency with the conversation message format.
	Role string `json:"role"`

	// Content contains the generated text response from the model.
	// Present when the model generates a text response, empty when making tool calls.
	Content string `json:"content,omitempty"`

	// ToolCalls contains function calls made by the assistant.
	// Present when stop_reason is "tool_calls", indicating the model wants
	// to execute functions before continuing the conversation.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// StreamResponse represents a single chunk in a streaming response.
//
// When using streaming mode, the model sends multiple StreamResponse chunks
// instead of a single complete Response. Each chunk contains incremental
// updates (deltas) that should be accumulated to build the complete response.
type StreamResponse struct {
	// Choices contains the streaming choice updates for this chunk.
	// Typically contains a single choice with delta information
	// representing the incremental content since the last chunk.
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice represents a single streaming choice with incremental updates.
//
// Each streaming choice contains a delta (incremental change) rather than
// the complete content. Clients should accumulate these deltas to build
// the full response content progressively.
type StreamChoice struct {
	// Index identifies the position of this choice in the choices array.
	// Consistent across all streaming chunks for the same choice.
	Index int `json:"index"`

	// Delta contains the incremental content update for this chunk.
	// This represents new content added since the previous chunk,
	// not the complete accumulated content.
	Delta StreamResponseMessage `json:"delta"`

	// StopReason indicates why streaming stopped, present only in the final chunk.
	// Valid values: "stop" (natural completion), "length" (max tokens reached),
	// "tool_calls" (model wants to call functions). Empty for intermediate chunks.
	StopReason string `json:"stop_reason,omitempty"`
}

// StreamResponseMessage represents incremental content in a streaming response.
//
// This structure contains delta (incremental) updates rather than complete content.
// Fields are populated only when they have new content to add. Clients should
// accumulate these deltas to build the complete assistant message.
type StreamResponseMessage struct {
	// Role is set only in the first streaming chunk to establish the message role.
	// Typically "assistant" for response messages, empty in subsequent chunks.
	Role string `json:"role,omitempty"`

	// Content contains new text content added in this streaming chunk.
	// Should be appended to previously received content to build the full response.
	Content string `json:"content,omitempty"`

	// ToolCalls contains incremental tool call information.
	// Present when the model begins or continues building function calls.
	// May contain partial tool call data that continues across chunks.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// MistralConverseStreamResponse represents a streaming response chunk from AWS Bedrock Converse API.
//
// This structure is used for AWS Bedrock's Converse API streaming mode, which provides
// a different streaming format compared to the standard chat completions API. It follows
// a pattern similar to Amazon Nova, with distinct events for message lifecycle management.
type MistralConverseStreamResponse struct {
	// MessageStart signals the beginning of a new assistant message.
	// Present in the first chunk of a streaming response to establish the message role.
	MessageStart *MistralMessageStart `json:"messageStart,omitempty"`

	// ContentBlockDelta contains incremental content updates during message generation.
	// Present in intermediate chunks that deliver the actual response content progressively.
	ContentBlockDelta *MistralContentBlockDelta `json:"contentBlockDelta,omitempty"`

	// MessageStop signals the end of message generation with completion reason.
	// Present in the final chunk to indicate why generation stopped.
	MessageStop *MistralMessageStop `json:"messageStop,omitempty"`

	// Metadata provides usage statistics and additional information about the response.
	// May be present in various chunks, typically in the final chunk with complete usage data.
	Metadata *MistralStreamMetadata `json:"metadata,omitempty"`
}

// MistralMessageStart indicates the beginning of a streaming assistant message.
//
// This event is sent as the first chunk in a Converse API streaming response
// to establish the role of the message being generated.
type MistralMessageStart struct {
	// Role identifies the message sender, typically "assistant" for model responses.
	// Consistent with the conversation message format used throughout the API.
	Role string `json:"role"`
}

// MistralContentBlockDelta contains incremental content updates in Converse API streaming.
//
// This structure wraps the actual delta content that should be accumulated
// to build the complete response text progressively.
type MistralContentBlockDelta struct {
	// Delta contains the actual incremental content update.
	// Should be appended to previously received content to build the full response.
	Delta MistralContentDelta `json:"delta"`
}

// MistralContentDelta represents the actual incremental text content in a streaming chunk.
//
// This is the core content structure within Converse API streaming responses,
// containing the new text that should be added to the growing response.
type MistralContentDelta struct {
	// Text contains the new text content added in this streaming chunk.
	// Should be concatenated with previous chunks to build the complete response.
	Text string `json:"text"`
}

// MistralMessageStop indicates the completion of a streaming message generation.
//
// This event is sent as the final chunk in a Converse API streaming response
// to signal that message generation has completed and provide the reason for stopping.
type MistralMessageStop struct {
	// StopReason indicates why message generation stopped.
	// Valid values include completion reasons like "stop", "length", or "tool_calls".
	StopReason string `json:"stopReason"`
}

// MistralStreamMetadata contains usage statistics and metadata for streaming responses.
//
// This structure provides token consumption information and other metadata
// about the streaming response, typically included in the final chunks.
type MistralStreamMetadata struct {
	// Usage contains detailed token consumption statistics for the request.
	// Includes input, output, and total token counts for billing and monitoring.
	Usage MistralUsage `json:"usage"`
}

// MistralUsage provides detailed token consumption statistics.
//
// This structure tracks the number of tokens consumed during request processing,
// essential for billing, quota management, and performance monitoring.
type MistralUsage struct {
	// InputTokens represents the number of tokens in the input (request).
	// Includes all tokens from messages, system prompts, and tool definitions.
	InputTokens int `json:"inputTokens"`

	// OutputTokens represents the number of tokens generated in the response.
	// Includes all tokens in the assistant's response text and tool calls.
	OutputTokens int `json:"outputTokens"`

	// TotalTokens is the sum of InputTokens and OutputTokens.
	// Provides a convenient total for billing and quota calculations.
	TotalTokens int `json:"totalTokens"`
}

// MistralConverseMessage represents a message in AWS Bedrock Converse API format.
//
// The Converse API uses a different message structure compared to the standard
// chat completions API, with content organized into blocks for enhanced flexibility
// and potential multimodal support.
type MistralConverseMessage struct {
	// Role identifies the sender of the message.
	// Valid values: "user" (human input), "assistant" (model response).
	// System messages are handled separately via MistralConverseSystemMessage.
	Role string `json:"role"`

	// Content contains the message content organized into structured blocks.
	// Currently supports text blocks, designed to accommodate future multimodal content.
	Content []MistralConverseContentBlock `json:"content"`
}

// MistralConverseContentBlock represents a content block within a Converse API message.
//
// This structure provides a flexible content format that can accommodate different
// types of content. Currently focuses on text content but designed for extensibility
// to support multimodal content in future API versions.
type MistralConverseContentBlock struct {
	// Text contains the text content for this content block.
	// This is the primary content type for current Mistral implementations.
	Text string `json:"text"`
}

// MistralConverseSystemMessage represents system instructions in Converse API format.
//
// System messages provide instructions and context to the model about how to behave
// during the conversation. In the Converse API, these are handled separately from
// regular user and assistant messages.
type MistralConverseSystemMessage struct {
	// Text contains the system instruction or prompt.
	// Should provide clear guidance about the model's role, behavior, and constraints.
	Text string `json:"text"`
}

// MistralConverseInferenceConfig specifies generation parameters for Converse API requests.
//
// This structure contains all the configurable parameters that control how the model
// generates responses, including length limits, randomness controls, and stopping conditions.
type MistralConverseInferenceConfig struct {
	// MaxTokens specifies the maximum number of tokens to generate in the response.
	// Required field that helps control response length and API costs.
	MaxTokens int `json:"maxTokens"`

	// Temperature controls the randomness of the model's responses.
	// Range: 0.0 to 1.0, where 0.0 is deterministic and 1.0 is most random.
	// Optional field, uses model default if not specified.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, limiting the cumulative probability of token choices.
	// Range: 0.0 to 1.0, where lower values make responses more focused.
	// Optional field, uses model default if not specified.
	TopP *float64 `json:"topP,omitempty"`

	// StopSequences contains custom strings that will stop generation when encountered.
	// Optional field that allows fine-grained control over response termination.
	StopSequences []string `json:"stopSequences,omitempty"`
}
