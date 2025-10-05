// Package aws provides AWS Bedrock adapter implementation for Qwen language models.
//
// This package implements the complete integration layer between the One API system
// and AWS Bedrock's Qwen model family. It provides production-ready adapters that
// handle request conversion, response processing, streaming support, and advanced
// tool calling capabilities for all Qwen models available through AWS Bedrock.
//
// # Supported Models
//
// The package supports the complete Qwen model family on AWS Bedrock:
//
// Qwen3 General Models:
//
//   - qwen3-235b: 235B parameter flagship model for complex reasoning tasks
//     Pricing: $0.22 input / $0.88 output per 1M tokens
//     Best for: Advanced reasoning, complex problem-solving, multi-step analysis
//
//   - qwen3-32b: 32B parameter efficient model for balanced performance
//     Pricing: $0.15 input / $0.6 output per 1M tokens
//     Best for: General conversation, content generation, quick responses
//
// Qwen3 Coder Models (Specialized for Programming):
//
//   - qwen3-coder-30b: 30B parameter code-focused model for efficient development
//     Pricing: $0.15 input / $0.6 output per 1M tokens
//     Best for: Code generation, debugging, technical documentation
//
//   - qwen3-coder-480b: 480B parameter advanced coding model
//     Pricing: $0.22 input / $1.8 output per 1M tokens
//     Best for: Complex algorithms, architecture design, code review
//
// # Key Features
//
// Request Processing:
//   - Full OpenAI API compatibility for seamless integration
//   - Automatic request format conversion to AWS Bedrock specifications
//   - Support for all standard parameters (temperature, top_p, max_tokens, stop)
//   - Reasoning effort control (low, medium, high) for enhanced reasoning visibility
//   - Multi-message conversation history with role management
//   - System message support for instruction-following
//
// Response Handling:
//   - OpenAI-compatible response format for easy client integration
//   - Accurate token usage tracking (prompt_tokens, completion_tokens, total_tokens)
//   - Proper stop reason mapping (stop, length, tool_calls, content_filter)
//   - Reasoning content support for transparent thought process visibility
//   - Error handling with detailed HTTP status codes
//
// Streaming Support:
//   - Real-time Server-Sent Events (SSE) for progressive response delivery
//   - Low-latency incremental token streaming
//   - Tool call streaming with argument accumulation
//   - Reasoning content streaming with progressive thought process delivery
//   - Usage statistics delivered at stream completion
//   - Proper stream finalization with [DONE] marker
//
// Reasoning Content Support:
//   - Transparent access to model's internal reasoning process
//   - Unified streaming of reasoning and final answer content
//   - ReasoningContent field in streaming deltas for real-time thought process
//   - Reasoning content blocks in non-streaming responses
//   - Compatible with OpenAI reasoning content format
//   - Useful for understanding complex problem-solving and code generation decisions
//
// Reasoning Effort Control:
//   - reasoning_effort parameter to control reasoning display level
//   - Valid values: "low", "medium", "high"
//   - "high" enables full reasoning content visibility in responses
//   - Converted to AWS Bedrock's reasoning_config in additional-model-request-fields
//   - Compatible with DeepSeek-style reasoning effort control
//   - Essential for models with advanced reasoning capabilities
//
// Advanced Tool Calling:
//   - Full function calling support via AWS Bedrock Converse API
//   - Tool definition with JSON schema parameter validation
//   - Multiple tool invocation modes:
//   - "auto": Model decides when to use tools
//   - "any": Model must invoke at least one tool
//   - Specific tool: Force invocation of named function
//   - Tool result submission with proper correlation
//   - Multi-turn tool calling workflows
//   - Streaming tool calls with incremental argument delivery
//   - Error handling for tool invocation failures
//
// # Architecture
//
// The package follows a clean adapter pattern with clear separation of concerns:
//
//	Request Flow:
//	  Client Request (OpenAI format)
//	    → Adaptor.ConvertRequest()
//	    → ConvertRequest() [format translation]
//	    → ConvertMessages() [message transformation]
//	    → AWS Bedrock Converse API
//
//	Response Flow:
//	  AWS Bedrock Response
//	    → Adaptor.DoResponse()
//	    → Handler() or StreamHandler()
//	    → convertConverseResponseToQwen()
//	    → Client Response (OpenAI format)
//
// # Usage Examples
//
// Basic Chat Completion (Non-Streaming):
//
//	POST /v1/chat/completions
//	{
//	  "model": "qwen3-32b",
//	  "messages": [
//	    {"role": "system", "content": "You are a helpful assistant."},
//	    {"role": "user", "content": "Explain quantum computing."}
//	  ],
//	  "temperature": 0.7,
//	  "max_tokens": 2000
//	}
//
// Code Generation with Streaming:
//
//	POST /v1/chat/completions
//	{
//	  "model": "qwen3-coder-480b",
//	  "messages": [
//	    {"role": "user", "content": "Write a binary search implementation in Python."}
//	  ],
//	  "stream": true,
//	  "temperature": 0.3
//	}
//
// Tool Calling for Code Execution:
//
//	POST /v1/chat/completions
//	{
//	  "model": "qwen3-coder-30b",
//	  "messages": [
//	    {"role": "user", "content": "Calculate the factorial of 10."}
//	  ],
//	  "tools": [
//	    {
//	      "type": "function",
//	      "function": {
//	        "name": "calculate",
//	        "description": "Execute mathematical calculations",
//	        "parameters": {
//	          "type": "object",
//	          "properties": {
//	            "expression": {"type": "string"}
//	          },
//	          "required": ["expression"]
//	        }
//	      }
//	    }
//	  ],
//	  "tool_choice": "auto"
//	}
//
// # Implementation Details
//
// Model ID Mapping:
//
// The package maintains a mapping between user-friendly model names and AWS Bedrock
// model identifiers. This abstraction allows for consistent naming across the API
// while maintaining compatibility with AWS Bedrock's versioned model system.
//
// Token Usage Tracking:
//
// Token usage is accurately tracked using AWS Bedrock's Converse API, which provides
// authoritative token counts directly from the model. This ensures billing accuracy
// and eliminates estimation errors.
//
// Tool Calling Protocol:
//
// Tool calling follows the AWS Bedrock Converse API protocol:
//  1. Client sends request with tool definitions
//  2. Model analyzes context and decides to invoke tools
//  3. Response contains tool_calls with function names and arguments
//  4. Client executes tools and submits results as user messages with tool_call_id
//  5. Model processes tool results and generates final response
//
// Streaming Implementation:
//
// Streaming uses AWS Bedrock's event stream protocol:
//   - MessageStart: Signals response beginning with role
//   - ContentBlockStart: Announces new content block (text or tool_use)
//   - ContentBlockDelta: Delivers incremental content (text or tool arguments)
//   - ContentBlockStop: Signals content block completion
//   - MessageStop: Indicates response completion with stop reason
//   - Metadata: Provides usage statistics at stream end
//
// # Performance Characteristics
//
// Model Performance (Approximate):
//
//	qwen3-235b:
//	  - Latency: ~2-4 seconds for first token
//	  - Throughput: ~50-80 tokens/second
//	  - Context: 32K tokens
//
//	qwen3-32b:
//	  - Latency: ~1-2 seconds for first token
//	  - Throughput: ~80-120 tokens/second
//	  - Context: 32K tokens
//
//	qwen3-coder-30b:
//	  - Latency: ~1-2 seconds for first token
//	  - Throughput: ~80-120 tokens/second
//	  - Context: 32K tokens (optimized for code)
//
//	qwen3-coder-480b:
//	  - Latency: ~3-5 seconds for first token
//	  - Throughput: ~40-60 tokens/second
//	  - Context: 32K tokens (optimized for code)
//
// # Error Handling
//
// The adapter implements comprehensive error handling:
//
// Common Errors:
//   - Model not found: Invalid model name in request
//   - Invalid request: Malformed parameters or missing required fields
//   - Token limit exceeded: Request exceeds model's context window
//   - Tool invocation failed: Invalid tool call format or execution error
//   - AWS API errors: Network issues, throttling, or service unavailability
//
// All errors are wrapped with context information and returned with appropriate
// HTTP status codes following OpenAI API conventions.
//
// # Best Practices
//
// Temperature Selection:
//   - Code generation: 0.2-0.4 for accuracy and consistency
//   - Creative writing: 0.7-0.9 for variety and creativity
//   - General conversation: 0.5-0.7 for balanced responses
//
// Token Management:
//   - Set max_tokens to control response length and costs
//   - Monitor prompt_tokens to stay within context limits
//   - Use streaming for long responses to improve user experience
//
// Tool Calling:
//   - Provide clear, detailed tool descriptions for better invocation
//   - Use JSON schema for strict parameter validation
//   - Handle tool errors gracefully with informative messages
//   - Consider "auto" tool_choice for flexibility
//
// Model Selection:
//   - qwen3-235b: Best for complex multi-step reasoning
//   - qwen3-32b: Best for general-purpose tasks with good cost/performance
//   - qwen3-coder-30b: Best for everyday coding tasks
//   - qwen3-coder-480b: Best for complex architectural and algorithmic work
//
// # Related Packages
//
// This package works in conjunction with:
//   - relay/adaptor/aws: Parent package providing common AWS Bedrock infrastructure
//   - relay/adaptor/aws/utils: Shared utilities for AWS adapters
//   - relay/adaptor/aws/internal/streamfinalizer: Stream finalization logic
//   - relay/model: Unified request/response models across all providers
//
// # References
//
// AWS Bedrock Documentation:
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/
//   - https://docs.aws.amazon.com/bedrock/latest/APIReference/
//
// Qwen Model Documentation:
//   - https://aws.amazon.com/bedrock/qwen/
//   - https://www.alibabacloud.com/help/en/model-studio/what-is-qwen-llm
package aws
