package aws

import (
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// Request represents a chat completion request to AWS Bedrock Qwen models.
//
// This structure defines all the parameters needed to send a chat completion
// request to the Qwen model family via AWS Bedrock. Supports both Qwen3 general
// models (qwen3-235b, qwen3-32b) for complex reasoning and conversation, and
// Qwen3 Coder models (qwen3-coder-30b, qwen3-coder-480b) specialized for code
// generation and technical analysis. All models feature advanced reasoning
// capabilities with configurable reasoning effort control.
//
// Based on AWS Bedrock Qwen model documentation.
type Request struct {
	// Messages contains the conversation history including system, user, and assistant messages.
	// This field is required and must contain at least one message.
	// Qwen models process these messages to generate high-quality responses across all tasks,
	// from complex reasoning to code generation. Supports tool calls and tool results for
	// advanced workflows including code execution, API integration, and automated reasoning.
	Messages []Message `json:"messages"`

	// MaxTokens specifies the maximum number of tokens to generate in the response.
	// Optional field that helps control response length and API costs.
	// Qwen models use this to limit generation while maintaining response quality across
	// all task types, including complex reasoning, conversation, and code generation.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls the randomness of the model's responses.
	// Range: 0.0 to 1.0, where 0.0 is deterministic and 1.0 is most random.
	// Optional field, uses model default if not specified.
	// Lower temperatures (0.2-0.4) recommended for code generation and reasoning tasks,
	// higher values (0.7-0.9) suitable for creative content and conversation.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, limiting the cumulative probability of token choices.
	// Range: 0.0 to 1.0, where lower values make responses more focused.
	// Optional field, uses model default if not specified.
	// Optimized for Qwen's response quality across reasoning, conversation, and code tasks.
	TopP *float64 `json:"top_p,omitempty"`

	// Stop contains custom strings that will stop generation when encountered.
	// Optional field that allows fine-grained control over response termination.
	// Useful for controlling when Qwen models stop generating in specific contexts,
	// including reasoning chains, code blocks, or conversation patterns.
	Stop []string `json:"stop,omitempty"`

	// ReasoningEffort controls the reasoning capabilities for Qwen models.
	// Optional field that enables enhanced reasoning display for Qwen models.
	// Valid values: "low", "medium", "high" - higher values show more detailed reasoning content.
	// When present, it's converted to additional-model-request-fields for AWS Bedrock.
	// Setting to "high" enables full reasoning content visibility in responses.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`

	// Tools contains the available tool definitions for the model to use.
	// Optional field that enables function calling capabilities.
	// When provided, Qwen models can intelligently invoke these tools for various tasks
	// including code execution, API integration, data processing, and automated workflows.
	Tools []QwenTool `json:"tools,omitempty"`

	// ToolChoice controls how the model decides to use tools.
	// Can be "auto" (model decides), "any" (must use a tool), or specify a particular tool.
	// Optional field that provides fine-grained control over tool invocation behavior.
	ToolChoice any `json:"tool_choice,omitempty"`
}

// Message represents a single message in the conversation history.
//
// Messages form the core of the chat completion request, containing the back-and-forth
// conversation between different participants. Each message has a specific role that
// determines its purpose and expected content format for Qwen model processing.
// Supports basic conversation, complex reasoning, code generation, and advanced tool calling.
type Message struct {
	// Role identifies the sender of the message.
	// Valid values: "system" (instructions to guide model behavior),
	// "user" (human input), "assistant" (model response).
	// Qwen models use role information to maintain conversation context and ensure
	// appropriate response generation across reasoning, conversation, and code tasks.
	Role string `json:"role"`

	// Content contains the text content of the message.
	// Required for system and user messages. For assistant messages, this contains
	// the model's response with Qwen's high-quality reasoning, conversation, or code generation.
	// Supports diverse content types including natural language, reasoning chains, and code.
	Content string `json:"content,omitempty"`

	// ToolCalls contains tool invocations made by the assistant.
	// Present in assistant messages when the model decides to call tools.
	// Each tool call includes function name, arguments, and unique identifier.
	ToolCalls []QwenToolCall `json:"tool_calls,omitempty"`

	// ToolCallID identifies which tool call this message responds to.
	// Present in user messages that contain tool execution results.
	// Links tool results back to the original assistant tool call.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// Response represents the complete response from AWS Bedrock Qwen models.
//
// This structure contains the model's response to a chat completion request,
// including generated text with Qwen's high-quality reasoning, conversation, or code.
// Qwen responses feature contextually appropriate content across all capabilities:
// complex reasoning, natural conversation, technical analysis, and code generation.
//
// Based on AWS Bedrock Qwen model documentation.
type Response struct {
	// Choices contains the possible response options generated by the model.
	// Typically contains a single choice, but the array format maintains
	// compatibility with OpenAI's API structure. Each choice represents
	// high-quality content from Qwen across reasoning, conversation, and code tasks.
	Choices []Choice `json:"choices"`
}

// Choice represents a single response option from the Qwen model.
//
// Each choice contains the generated content along with metadata about
// why the generation stopped. Qwen choices feature high-quality content
// across all capabilities: complex reasoning, natural conversation,
// technical analysis, code generation, and problem-solving.
type Choice struct {
	// Index identifies the position of this choice in the choices array.
	// Typically 0 for the first (and usually only) choice.
	Index int `json:"index"`

	// Message contains the actual response content from the assistant.
	// This includes Qwen's high-quality reasoning, conversation, or code generation
	// with comprehensive language support and advanced capabilities.
	Message ResponseMessage `json:"message"`

	// StopReason indicates why the model stopped generating tokens.
	// Valid values: "stop" (natural completion), "length" (max tokens reached),
	// "end_turn" (model decided to end), "max_tokens" (token limit reached).
	StopReason string `json:"stop_reason"`

	// FinishReason indicates why the model stopped generating tokens (OpenAI format).
	// Valid values: "stop" (natural completion), "length" (max tokens reached),
	// "tool_calls" (model invoked tools), "content_filter" (safety filtering).
	FinishReason string `json:"finish_reason"`
}

// ResponseMessage represents the assistant's message in the response.
//
// This structure contains the model's generated content with Qwen's high-quality
// capabilities across reasoning, conversation, and code generation. The response
// includes advanced language understanding, technical analysis, and problem-solving.
// The role is always "assistant" for response messages.
type ResponseMessage struct {
	// Role identifies the message sender, always "assistant" for responses.
	// This maintains consistency with the conversation message format
	// and indicates Qwen generated content.
	Role string `json:"role"`

	// Content contains the generated text response from the model.
	// This is Qwen's high-quality response across reasoning, conversation, or code tasks,
	// with advanced language understanding and comprehensive problem-solving applied.
	Content string `json:"content,omitempty"`
}

// StreamResponse represents a streaming response chunk from AWS Bedrock Qwen models.
//
// This structure contains the streaming response data when using server-sent events
// for real-time chat completion. Each chunk represents incremental content delivery
// from Qwen models, allowing for progressive response rendering with high-quality
// content maintained throughout the stream across reasoning, conversation, and code.
//
// Based on AWS Bedrock Qwen streaming documentation.
type StreamResponse struct {
	// Choices contains the streaming response options generated by the model.
	// Similar to non-streaming responses, this typically contains a single choice
	// but maintains array format for API compatibility. Each choice represents
	// incremental high-quality content from Qwen across reasoning, conversation, and code.
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice represents a single streaming response option from the Qwen model.
//
// Each streaming choice contains incremental content (delta) along with metadata
// about the current state of generation. Qwen streaming choices deliver high-quality
// content progressively, maintaining reasoning coherence, conversation context,
// or code accuracy throughout the stream.
type StreamChoice struct {
	// Index identifies the position of this choice in the choices array.
	// Typically 0 for the first (and usually only) streaming choice.
	Index int `json:"index"`

	// Delta contains the incremental content generated by the model.
	// This represents the new tokens added in this streaming chunk,
	// featuring Qwen's high-quality content with progressive delivery
	// across reasoning, conversation, and code generation.
	Delta StreamResponseMessage `json:"delta"`

	// StopReason indicates why the model stopped generating tokens, if applicable.
	// Valid values: "stop" (natural completion), "length" (max tokens reached),
	// "end_turn" (model decided to end), "max_tokens" (token limit reached).
	// Empty during active streaming, populated only when generation completes.
	StopReason string `json:"stop_reason,omitempty"`
}

// StreamResponseMessage represents the incremental delta message in a streaming response.
//
// This structure contains the progressive content generated by Qwen models
// during streaming responses. Each delta represents new tokens added to the
// conversation, maintaining high-quality content with advanced capabilities
// applied progressively across reasoning, conversation, and code generation.
type StreamResponseMessage struct {
	// Role identifies the message sender for the delta content.
	// Typically "assistant" for streaming responses, indicating
	// Qwen generated content.
	Role string `json:"role,omitempty"`

	// Content contains the incremental text generated in this streaming chunk.
	// This represents new tokens from Qwen's high-quality response with
	// progressive delivery of reasoning, conversation, or code content.
	Content string `json:"content,omitempty"`
}

// QwenConverseStreamResponse represents a streaming response from AWS Bedrock Qwen Converse API.
//
// This structure handles streaming responses from Qwen models through
// the AWS Bedrock Converse API. It provides different event types during the
// streaming process, including message initiation, content deltas, completion,
// and usage metadata. Each streaming event delivers high-quality content
// across reasoning, conversation, and code generation tasks.
//
// Based on AWS Bedrock Converse API streaming documentation.
type QwenConverseStreamResponse struct {
	// MessageStart indicates the beginning of a new message generation.
	// Contains role information for the assistant response from Qwen models.
	// Present only at the start of streaming to establish message context.
	MessageStart *QwenMessageStart `json:"messageStart,omitempty"`

	// ContentBlockDelta contains incremental content generated during streaming.
	// Provides progressive text delivery from Qwen with high-quality content
	// across reasoning, conversation, and code tasks throughout the stream.
	ContentBlockDelta *QwenContentBlockDelta `json:"contentBlockDelta,omitempty"`

	// MessageStop indicates the completion of message generation.
	// Contains information about why generation stopped, providing insight into
	// Qwen's completion logic across reasoning, conversation, and code generation.
	MessageStop *QwenMessageStop `json:"messageStop,omitempty"`

	// Metadata provides usage information and statistics for the streaming session.
	// Includes token counts for cost tracking and performance monitoring of
	// Qwen generation across all task types.
	Metadata *QwenStreamMetadata `json:"metadata,omitempty"`
}

// QwenMessageStart represents the initiation event for streaming message generation.
//
// This structure signals the beginning of a new response from Qwen3 Coder
// through the AWS Bedrock Converse API. It establishes the role context for
// the streaming content that follows, ensuring proper conversation flow and
// code-focused response quality from the start.
type QwenMessageStart struct {
	// Role identifies the message sender, typically "assistant" for Qwen3 Coder responses.
	// This establishes the context for code-focused content generation
	// and maintains conversation consistency throughout the streaming process.
	Role string `json:"role"`
}

// QwenContentBlockDelta represents incremental content delivery during streaming.
//
// This structure contains the progressive content generated by Qwen3 Coder
// during streaming responses through the AWS Bedrock Converse API. Each delta
// provides new text tokens with code-focused quality, maintaining technical
// accuracy and programming context throughout content delivery.
type QwenContentBlockDelta struct {
	// Delta contains the incremental text content generated in this streaming chunk.
	// Features Qwen3 Coder's code-focused generation with progressive delivery,
	// advanced technical understanding, and multi-language programming support.
	Delta QwenContentDelta `json:"delta"`
}

// QwenContentDelta represents the actual text content in a streaming delta.
//
// This structure contains the incremental text tokens generated by Qwen3 Coder
// during streaming responses. Each delta represents new content added to the
// conversation with code-focused quality, featuring advanced multi-language
// programming support, technical accuracy, and reasoning applied progressively.
type QwenContentDelta struct {
	// Text contains the incremental text generated in this streaming chunk.
	// This represents new tokens from Qwen3 Coder's code-focused response
	// with progressive delivery of technical content, programming accuracy,
	// and multi-language code support.
	Text string `json:"text,omitempty"`

	// ReasoningContent contains the reasoning process content from Qwen.
	// This feature shows the model's internal thought process and reasoning
	// steps before arriving at the final answer. Similar to DeepSeek's reasoning
	// capability, Qwen models can provide transparency into their decision-making.
	ReasoningContent *QwenReasoningContent `json:"reasoningContent,omitempty"`
}

// QwenReasoningContent represents the reasoning process content from Qwen models.
//
// This structure captures Qwen's internal reasoning process, allowing users
// to understand how the model arrived at its conclusions. This is particularly
// useful for complex problem-solving, code generation decisions, and multi-step
// technical reasoning tasks.
type QwenReasoningContent struct {
	// ReasoningText contains the model's internal reasoning process.
	// This shows the step-by-step thought process that Qwen used
	// to analyze the problem and arrive at its final answer or code solution.
	ReasoningText string `json:"reasoningText"`
}

// QwenMessageStop represents the completion event for streaming message generation.
//
// This structure signals the end of content generation from Qwen3 Coder
// through the AWS Bedrock Converse API. It provides information about why
// generation completed, offering insight into the model's decision-making
// process and technical reasoning for code generation completion.
type QwenMessageStop struct {
	// StopReason indicates why the model stopped generating tokens.
	// Valid values include "end_turn" (natural completion), "max_tokens" (token limit),
	// "stop_sequence" (custom stop sequence encountered), and other Qwen-specific
	// reasons including code generation completion logic.
	StopReason string `json:"stopReason"`
}

// QwenStreamMetadata represents usage statistics and metadata for streaming responses.
//
// This structure provides comprehensive usage information for Qwen3 Coder
// streaming sessions through the AWS Bedrock Converse API. It includes token
// consumption data essential for cost tracking, performance monitoring, and
// usage analytics of Qwen's advanced code generation capabilities.
type QwenStreamMetadata struct {
	// Usage contains detailed token consumption statistics for the streaming session.
	// Provides input, output, and total token counts for accurate cost calculation
	// and performance monitoring of Qwen3 Coder code generation and technical analysis.
	Usage QwenUsage `json:"usage"`
}

// QwenUsage represents detailed token usage statistics for Qwen requests.
//
// This structure provides comprehensive token consumption data for both streaming
// and non-streaming requests to Qwen3 Coder models through AWS Bedrock.
// Token tracking is essential for cost management, performance optimization,
// and usage analytics of Qwen's advanced code generation capabilities.
type QwenUsage struct {
	// InputTokens represents the number of tokens consumed from the input prompt.
	// Includes all tokens from system messages, user messages, and conversation history
	// processed by Qwen3 Coder for technical understanding and code generation.
	InputTokens int `json:"inputTokens"`

	// OutputTokens represents the number of tokens generated in the response.
	// Includes all tokens produced by Qwen3 Coder in the assistant response
	// with code-focused quality and technical accuracy applied.
	OutputTokens int `json:"outputTokens"`

	// TotalTokens represents the sum of input and output tokens for the request.
	// Provides the complete token consumption for cost calculation and usage
	// monitoring of Qwen3 Coder code generation and technical analysis sessions.
	TotalTokens int `json:"totalTokens"`
}

// QwenConverseMessage represents a message in the AWS Bedrock Converse API format.
//
// This structure defines individual messages within conversations for Qwen3 Coder
// models through the AWS Bedrock Converse API. Each message contains role information
// and content blocks, supporting rich text communication with code-focused quality,
// multi-language programming support, and advanced technical understanding.
//
// Based on AWS Bedrock Converse API message format documentation.
type QwenConverseMessage struct {
	// Role identifies the sender of the message in the conversation.
	// Valid values: "user" (human input), "assistant" (Qwen3 Coder response),
	// "system" (instructions). Maintains conversation context for code-focused
	// dialogue flow and technical multi-turn interactions.
	Role string `json:"role"`

	// Content contains the message content as an array of content blocks.
	// Supports structured content delivery for Qwen3 Coder processing,
	// enabling rich text communication with advanced technical understanding
	// and code-focused quality.
	Content []QwenConverseContentBlock `json:"content"`
}

// QwenConverseContentBlock represents a content block within a Converse API message.
//
// This structure contains individual content elements that make up a message
// for Qwen3 Coder models through the AWS Bedrock Converse API. Content blocks
// enable structured text delivery with code-focused processing, supporting
// advanced multi-language programming capabilities and technical understanding.
// Can contain either text content or reasoning content blocks.
type QwenConverseContentBlock struct {
	// Text contains the textual content of this block.
	// Supports rich text input for Qwen3 Coder processing with code-focused
	// quality, advanced technical understanding, multi-language programming support,
	// and accurate code generation capabilities.
	Text string `json:"text,omitempty"`

	// ReasoningContent contains the reasoning process content from Qwen.
	// This field captures the model's internal thought process and reasoning steps,
	// providing transparency into how the model arrived at its code solutions or answers.
	ReasoningContent *QwenReasoningContent `json:"reasoningContent,omitempty"`
}

// QwenConverseSystemMessage represents system instructions for the Converse API.
//
// This structure defines system-level instructions that guide Qwen3 Coder's
// behavior and response generation through the AWS Bedrock Converse API. System
// messages establish context, tone, and operational parameters for code-focused
// conversations with advanced programming capabilities and technical accuracy.
type QwenConverseSystemMessage struct {
	// Text contains the system instruction content.
	// Provides behavioral guidance for Qwen3 Coder code generation and technical analysis,
	// establishing context, tone, programming style, and response characteristics
	// for accurate, multi-language code generation and problem-solving.
	Text string `json:"text"`
}

// QwenConverseInferenceConfig represents inference parameters for the Converse API.
//
// This structure defines generation parameters that control Qwen3 Coder's
// response characteristics through the AWS Bedrock Converse API. These parameters
// enable fine-tuned control over code-focused generation, balancing creativity,
// technical accuracy, and programming quality for optimal code generation.
//
// Based on AWS Bedrock Converse API inference configuration documentation.
type QwenConverseInferenceConfig struct {
	// MaxTokens specifies the maximum number of tokens to generate in the response.
	// Controls response length and API costs while maintaining Qwen3 Coder's
	// code-focused quality and technical accuracy in generation completion.
	MaxTokens int `json:"maxTokens"`

	// Temperature controls the randomness of Qwen3 Coder's responses.
	// Range: 0.0 to 1.0, where 0.0 is deterministic and 1.0 is most random.
	// Optional field that balances creativity with consistency in code generation.
	// Lower values recommended for accurate code generation.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, limiting cumulative probability of token choices.
	// Range: 0.0 to 1.0, where lower values make responses more focused.
	// Optional field optimized for Qwen's code generation quality and technical accuracy.
	TopP *float64 `json:"topP,omitempty"`

	// StopSequences contains custom strings that will stop generation when encountered.
	// Optional field enabling fine-grained control over Qwen3 Coder response
	// termination for code generation management and technical dialogue flow control.
	StopSequences []string `json:"stopSequences,omitempty"`
}

// QwenConverseResponse represents the complete response from AWS Bedrock Converse API.
//
// This structure contains Qwen3 Coder's response to a conversation request
// through the AWS Bedrock Converse API. It includes the generated message with
// code-focused quality, completion metadata, and usage statistics for
// comprehensive code generation management and cost tracking.
//
// Based on AWS Bedrock Converse API response format documentation.
type QwenConverseResponse struct {
	// Message contains the assistant's response with role and content information.
	// Features Qwen3 Coder's code-focused quality with advanced technical
	// understanding, multi-language programming support, accurate code generation,
	// and problem-solving applied to the generated content.
	Message struct {
		// Role identifies the message sender, typically "assistant" for Qwen3 Coder.
		// Maintains conversation context and indicates code-focused generated content
		// with advanced programming capabilities and technical accuracy applied.
		Role string `json:"role"`

		// Content contains the response content as structured content blocks.
		// Delivers Qwen3 Coder's code-focused response with technical accuracy,
		// programming quality, and multi-language code support.
		Content []QwenConverseContentBlock `json:"content"`
	} `json:"message"`

	// StopReason indicates why Qwen3 Coder stopped generating tokens.
	// Valid values include "end_turn" (natural completion), "max_tokens" (limit reached),
	// "stop_sequence" (custom sequence encountered), and code generation completion reasons.
	StopReason string `json:"stopReason"`

	// Usage contains detailed token consumption statistics for the request.
	// Provides input, output, and total token counts for cost management
	// and performance monitoring of Qwen3 Coder code generation sessions.
	Usage QwenUsage `json:"usage"`
}

// QwenBedrockResponse represents the complete response from AWS Bedrock Qwen models.
//
// This structure provides a comprehensive response format that matches the official
// AWS Bedrock Converse API structure for Qwen3 Coder models. It includes
// response metadata, generated content choices with code-focused quality,
// and detailed usage statistics for code generation management and cost tracking.
//
// Based on AWS Bedrock official Converse API response format documentation.
type QwenBedrockResponse struct {
	// ID provides a unique identifier for this specific response.
	// Used for request tracking, logging, and code generation session management
	// with Qwen3 Coder models through AWS Bedrock infrastructure.
	ID string `json:"id"`

	// Model identifies the specific Qwen3 Coder model used for generation.
	// Optional field that indicates which code-focused model variant processed
	// the request, useful for model performance tracking and programming analytics.
	Model string `json:"model,omitempty"`

	// Object specifies the response type, typically "chat.completion".
	// Maintains compatibility with standard chat completion APIs while providing
	// Qwen3 Coder's code-focused capabilities through AWS Bedrock.
	Object string `json:"object"`

	// Created represents the Unix timestamp when the response was generated.
	// Provides timing information for code generation analytics, performance
	// monitoring, and audit trails of Qwen3 Coder interactions.
	Created int64 `json:"created"`

	// Choices contains the response options generated by Qwen3 Coder.
	// Typically includes a single choice featuring code-focused quality
	// with advanced technical understanding, multi-language support, and programming accuracy.
	Choices []QwenBedrockChoice `json:"choices"`

	// Usage contains detailed token consumption statistics for cost management.
	// Provides comprehensive input, output, and total token counts for
	// cost tracking and performance monitoring of Qwen3 Coder code generation.
	Usage relaymodel.Usage `json:"usage"`
}

// QwenBedrockChoice represents a single response choice from AWS Bedrock Qwen.
//
// This structure contains individual response options generated by Qwen3 Coder
// models through AWS Bedrock. Each choice includes the generated message content
// with code-focused quality, completion metadata, and contextual information
// about the generation process and technical reasoning.
type QwenBedrockChoice struct {
	// Index identifies the position of this choice in the choices array.
	// Typically 0 for the primary (and usually only) choice, maintaining
	// compatibility with multi-choice response formats while delivering
	// Qwen3 Coder's focused code-generation quality.
	Index int `json:"index"`

	// Message contains the generated response content from Qwen3 Coder.
	// Features code-focused quality with advanced technical understanding,
	// accurate code generation, multi-language programming support, and
	// comprehensive problem-solving applied throughout the generation process.
	Message QwenBedrockMessage `json:"message"`

	// FinishReason indicates why Qwen3 Coder stopped generating content.
	// Valid values: "stop" (natural completion), "length" (token limit reached),
	// "tool_calls" (tool invocation), and other code generation-specific completion
	// reasons that provide insight into the generation termination process.
	FinishReason string `json:"finish_reason"`
}

// QwenBedrockMessage represents the message content in AWS Bedrock response format.
//
// This structure contains the actual conversation content generated by Qwen3 Coder
// through AWS Bedrock, formatted as structured content blocks. It maintains role
// information and delivers code-focused quality with advanced technical understanding,
// accurate code generation, and comprehensive programming support.
type QwenBedrockMessage struct {
	// Role identifies the message sender, typically "assistant" for Qwen3 Coder.
	// Maintains conversation context and indicates code-focused generated content
	// with advanced programming capabilities, multi-language support, and technical
	// accuracy applied throughout the response generation process.
	Role string `json:"role"`

	// Content contains the response as an array of structured content blocks.
	// Delivers Qwen3 Coder's code-focused content with accurate code generation,
	// technical appropriateness, advanced multi-language programming support,
	// and comprehensive problem-solving applied to ensure high-quality responses.
	Content []QwenBedrockContentBlock `json:"content"`
}

// QwenBedrockContentBlock represents individual content elements in AWS Bedrock format.
//
// This structure contains specific content elements that compose the response message
// from Qwen3 Coder models through AWS Bedrock. Each content block represents
// a portion of the code-focused response with advanced technical understanding,
// accurate code generation, and comprehensive programming support applied.
// Can contain either text content or reasoning content blocks.
type QwenBedrockContentBlock struct {
	// Text contains the textual content of this block.
	// Represents a segment of Qwen3 Coder's code-focused response with accurate
	// code generation, technical appropriateness, multi-language programming support,
	// and problem-solving ensuring high-quality technical content.
	Text *string `json:"text,omitempty"`

	// ReasoningContent contains the reasoning process content from Qwen.
	// This field captures the model's internal thought process and reasoning steps,
	// providing transparency into how the model arrived at its code solutions or answers.
	ReasoningContent *QwenReasoningContent `json:"reasoningContent,omitempty"`
}

// QwenBedrockStreamChoice represents individual streaming choices in AWS Bedrock format.
//
// This structure contains incremental content delivery from Qwen3 Coder models
// during streaming responses through AWS Bedrock. Each streaming choice provides
// progressive content with code-focused quality, maintaining accurate code generation
// and technical appropriateness throughout the stream.
type QwenBedrockStreamChoice struct {
	// Index identifies the position of this streaming choice in the choices array.
	// Typically 0 for the primary streaming choice, ensuring focused delivery
	// of Qwen3 Coder's code-focused content.
	Index int `json:"index"`

	// Delta contains the incremental content generated in this streaming chunk.
	// Provides progressive delivery of Qwen3 Coder's code-focused response
	// with accurate code generation, technical understanding, and programming
	// support applied to each incremental content piece.
	Delta QwenBedrockStreamMessage `json:"delta"`

	// FinishReason indicates completion status when streaming ends, if applicable.
	// Provides insight into why Qwen3 Coder completed generation, including
	// natural completion, token limits, or code generation completion logic.
	FinishReason *string `json:"finish_reason,omitempty"`
}

// QwenBedrockStreamMessage represents incremental message content during streaming.
//
// This structure contains the progressive content generated by Qwen3 Coder
// during streaming responses through AWS Bedrock. Each streaming message represents
// new content added to the conversation with code-focused quality, featuring
// accurate code generation, technical understanding, and programming support.
type QwenBedrockStreamMessage struct {
	// Role identifies the sender for streaming content, typically "assistant".
	// Maintains conversation context during progressive delivery and indicates
	// code-focused content generation from Qwen3 Coder with advanced
	// programming capabilities and technical accuracy applied.
	Role string `json:"role,omitempty"`

	// Content contains incremental content blocks delivered in this streaming chunk.
	// Provides progressive delivery of Qwen3 Coder's code-focused response with
	// accurate code generation, technical appropriateness, and comprehensive
	// programming support applied to streaming content.
	Content []QwenBedrockContentBlock `json:"content,omitempty"`
}

// QwenTool represents a tool definition for Qwen's advanced function calling capabilities.
//
// This structure defines individual tools that can be made available to Qwen
// models through AWS Bedrock's Converse API. Each tool represents a function that the model
// can intelligently decide to invoke during conversation, enabling diverse workflows
// including automation, code execution, API integration, data processing, and reasoning tasks
// with Qwen's advanced decision-making and understanding capabilities.
//
// Based on AWS Bedrock Converse API tool specification format.
type QwenTool struct {
	// Type specifies the tool category, typically "function" for callable functions.
	// This field categorizes the tool for Qwen's intelligent tool selection
	// process, ensuring appropriate invocation across reasoning, conversation, and code contexts
	// while maintaining compatibility with AWS Bedrock's tool calling infrastructure.
	Type string `json:"type"`

	// Function contains the detailed specification of the callable function.
	// Provides Qwen with comprehensive function metadata including name,
	// description, and parameter schema for intelligent tool selection and invocation
	// across diverse workflows including automation, reasoning, and code tasks.
	Function QwenToolSpec `json:"function"`
}

// QwenToolSpec represents the specification of a tool function for Qwen integration.
//
// This structure provides comprehensive metadata about a callable function that Qwen
// can intelligently invoke through AWS Bedrock's Converse API. The specification enables
// the model to understand function capabilities, parameter requirements, and appropriate
// usage contexts for tool calling across reasoning, conversation, and code tasks with
// advanced decision-making applied throughout the invocation process.
type QwenToolSpec struct {
	// Name identifies the unique function name for tool invocation.
	// Used by Qwen for precise tool selection and invocation across
	// reasoning, conversation, and code contexts, ensuring accurate function identification
	// and maintaining compatibility with AWS Bedrock's tool calling mechanisms.
	Name string `json:"name"`

	// Description provides human-readable explanation of the function's purpose and behavior.
	// Enables Qwen's advanced reasoning capabilities to intelligently decide
	// when and how to invoke the tool within conversation context, supporting diverse
	// automation workflows with contextual appropriateness and accuracy.
	Description string `json:"description"`

	// Parameters defines the JSON schema for function input parameters.
	// Provides Qwen with parameter structure, types, and constraints
	// for generating appropriate function calls with proper validation and type safety
	// across tool calling workflows through AWS Bedrock integration.
	Parameters any `json:"parameters"`
}

// QwenToolCall represents a tool invocation made by Qwen3 Coder during conversation.
//
// This structure contains the details of a function call that Qwen3 Coder has decided
// to make during conversation processing through AWS Bedrock. Each tool call represents
// an intelligent decision by the model to invoke external functionality, featuring
// code-focused reasoning, technical appropriateness, and programming accuracy
// applied to the tool selection and parameter generation process.
type QwenToolCall struct {
	// ID provides a unique identifier for this specific tool call invocation.
	// Used for tracking and correlating tool calls with their corresponding results
	// in code-focused workflows, enabling proper response handling and
	// maintaining conversation context throughout the tool calling process.
	ID string `json:"id"`

	// Type specifies the tool call category, typically "function" for function invocations.
	// Indicates the nature of the tool call for proper processing and response handling
	// within Qwen3 Coder's code-focused workflows and AWS Bedrock
	// tool calling infrastructure integration.
	Type string `json:"type"`

	// Function contains the specific function invocation details and generated parameters.
	// Includes the function name and arguments generated by Qwen3 Coder's advanced
	// reasoning capabilities, ensuring appropriate parameter values and maintaining
	// code-focused accuracy and technical safety throughout the tool invocation process.
	Function QwenToolFunction `json:"function"`
}

// QwenToolFunction represents the function details in a Qwen3 Coder tool call.
//
// This structure contains the specific function invocation information generated by
// Qwen3 Coder during intelligent tool calling through AWS Bedrock. It includes
// the function identifier and generated arguments, demonstrating the model's
// code-focused reasoning capabilities in parameter generation and technical
// appropriateness for automated workflow integration and programming tool calling.
type QwenToolFunction struct {
	// Name identifies the specific function to be invoked by the tool call.
	// Corresponds to a function defined in the available tools list, enabling
	// precise function selection by Qwen3 Coder's intelligent reasoning
	// within code-focused contexts and AWS Bedrock integration.
	Name string `json:"name"`

	// Arguments contains the JSON-encoded function parameters generated by the model.
	// Represents Qwen3 Coder's intelligent parameter generation based on
	// conversation context, function schema, and code-focused reasoning
	// capabilities, ensuring appropriate values for successful tool invocation.
	Arguments string `json:"arguments"`
}

// QwenResponseMessage represents an OpenAI-compatible message with comprehensive tool calling support.
//
// This structure provides OpenAI API compatibility for Qwen3 Coder responses processed
// through AWS Bedrock, enabling seamless integration with existing OpenAI-based applications
// while preserving Qwen's code-focused quality and advanced tool calling capabilities.
// The message format maintains full compatibility with OpenAI's chat completion API
// while delivering Qwen3 Coder's multi-language programming support and technical accuracy.
//
// Based on OpenAI Chat Completions API message format with Qwen3 Coder enhancements.
type QwenResponseMessage struct {
	// Role identifies the message sender, typically "assistant" for Qwen3 Coder responses.
	// Maintains OpenAI API compatibility while indicating code-focused content generation
	// from Qwen3 Coder with advanced programming capabilities, technical understanding,
	// and comprehensive accuracy applied throughout the response generation.
	Role string `json:"role"`

	// Content contains the generated text response from Qwen3 Coder.
	// Delivers code-focused content with OpenAI API compatibility, featuring
	// Qwen's advanced multi-language programming support, technical accuracy,
	// and problem-solving while maintaining the expected OpenAI response structure.
	Content string `json:"content,omitempty"`

	// ToolCalls contains tool invocations made by Qwen3 Coder in OpenAI-compatible format.
	// Provides seamless OpenAI API compatibility for tool calling functionality while
	// preserving Qwen3 Coder's intelligent tool selection and parameter generation
	// capabilities within code-focused workflows.
	ToolCalls []QwenToolCallResponse `json:"tool_calls,omitempty"`

	// ReasoningContent contains the model's internal reasoning process.
	// This field provides transparency into Qwen's thought process and decision-making,
	// showing the step-by-step reasoning that led to the final answer. Compatible
	// with OpenAI's reasoning content format for deepseek-reasoner and similar models.
	ReasoningContent *string `json:"reasoning_content,omitempty"`
}

// QwenToolCallResponse represents a tool call in OpenAI-compatible format for seamless integration.
//
// This structure converts Qwen3 Coder's intelligent tool calling decisions into
// OpenAI-compatible format, enabling existing OpenAI-based applications to seamlessly
// integrate with Qwen's code-focused tool calling capabilities through AWS Bedrock.
// Maintains full compatibility with OpenAI's tool calling API while preserving Qwen's
// advanced reasoning and technical appropriateness in tool selection and invocation.
type QwenToolCallResponse struct {
	// ID provides the unique identifier for this tool call, maintaining OpenAI compatibility.
	// Enables proper tool call tracking and correlation with results in existing
	// OpenAI-based applications while preserving Qwen3 Coder's code-focused
	// tool calling workflow management and conversation context handling.
	ID string `json:"id"`

	// Type specifies the tool call category in OpenAI-compatible format, typically "function".
	// Maintains compatibility with OpenAI's tool calling API structure while indicating
	// Qwen3 Coder's intelligent function invocation within code-focused workflows
	// and AWS Bedrock integration capabilities.
	Type string `json:"type"`

	// Function contains the specific function details generated by Qwen3 Coder.
	// Provides OpenAI-compatible function invocation information while preserving
	// Qwen's code-focused parameter generation, technical reasoning, and
	// intelligent tool selection capabilities for seamless application integration.
	Function QwenToolFunction `json:"function"`
}

// QwenResponseChoice represents an OpenAI-compatible choice with comprehensive tool calling support.
//
// This structure provides individual response options from Qwen3 Coder in OpenAI-compatible
// format, enabling seamless integration with existing applications while preserving code-focused
// quality. Each choice represents Qwen's advanced reasoning capabilities, technical
// understanding, and programming accuracy delivered through AWS Bedrock with full OpenAI API
// compatibility for tool calling and standard code generation workflows.
type QwenResponseChoice struct {
	// Index identifies the position of this choice in the choices array for OpenAI compatibility.
	// Typically 0 for the primary choice, maintaining OpenAI API structure while delivering
	// Qwen3 Coder's focused code-generation quality and intelligent tool calling
	// capabilities through AWS Bedrock integration.
	Index int `json:"index"`

	// Message contains the response content and tool calls from Qwen3 Coder.
	// Delivers code-focused content in OpenAI-compatible format, featuring
	// Qwen's advanced multi-language programming support, technical accuracy,
	// intelligent tool calling, and comprehensive problem-solving capabilities.
	Message QwenResponseMessage `json:"message"`

	// FinishReason indicates why Qwen3 Coder stopped generating content in OpenAI format.
	// Provides completion status information compatible with OpenAI API expectations
	// while reflecting Qwen's code-focused generation logic, programming accuracy,
	// and intelligent conversation completion decisions.
	FinishReason string `json:"finish_reason"`
}

// QwenResponse represents the complete OpenAI-compatible response with Qwen3 Coder capabilities.
//
// This structure provides full OpenAI Chat Completions API compatibility for Qwen3 Coder
// responses processed through AWS Bedrock, enabling seamless integration with existing
// applications while preserving all of Qwen's code-focused features including advanced
// tool calling, multi-language programming support, technical understanding, and comprehensive
// programming accuracy. The response maintains OpenAI format while delivering superior code quality.
//
// Based on OpenAI Chat Completions API response format with Qwen3 Coder enhancements.
type QwenResponse struct {
	// ID provides a unique identifier for this response in OpenAI-compatible format.
	// Enables proper response tracking and correlation in existing applications while
	// maintaining code-focused session management and audit capabilities
	// for Qwen3 Coder interactions through AWS Bedrock infrastructure.
	ID string `json:"id"`

	// Object specifies the response type, maintaining OpenAI API compatibility.
	// Typically "chat.completion" to indicate chat completion response format
	// while delivering Qwen3 Coder's code-focused capabilities and advanced
	// tool calling features through AWS Bedrock integration.
	Object string `json:"object"`

	// Created represents the Unix timestamp when the response was generated for OpenAI compatibility.
	// Provides timing information in standard OpenAI format while enabling
	// code generation analytics, performance monitoring, and audit trails for
	// Qwen3 Coder interactions and tool calling workflows.
	Created int64 `json:"created"`

	// Model identifies the specific Qwen model used, maintaining OpenAI API compatibility.
	// Indicates which Qwen3 Coder variant processed the request while preserving
	// OpenAI format expectations for model identification in code generation
	// applications and programming analytics workflows.
	Model string `json:"model"`

	// Choices contains the response options from Qwen3 Coder in OpenAI-compatible format.
	// Delivers code-focused choices with advanced tool calling support,
	// multi-language programming capabilities, technical understanding, and accuracy
	// while maintaining full compatibility with existing OpenAI-based applications.
	Choices []QwenResponseChoice `json:"choices"`

	// Usage contains detailed token consumption statistics in OpenAI-compatible format (prompt_tokens, completion_tokens, total_tokens).
	// Matches the project's unified Usage struct to ensure consistent billing, logging, and client compatibility.
	Usage relaymodel.Usage `json:"usage"`
}
