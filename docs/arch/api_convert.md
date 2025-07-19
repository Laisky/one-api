# API Format Conversion Architecture

## Overview

This document describes the architecture and implementation of the API format conversion system in the one-api project. The system enables automatic conversion between different API formats:

1. **OpenAI Response API Conversion**: Conversion between OpenAI's ChatCompletion API format and the newer Response API format
2. **Claude Messages API Conversion**: Conversion from Claude Messages API format to various adapter formats (OpenAI, Gemini, etc.)

This allows users to access different API capabilities through standardized interfaces.

## Problem Statement

OpenAI introduced the Response API format as a new interface that provides enhanced capabilities for certain models. However, not all models support the Response API format - some models only support the traditional ChatCompletion API. The project needs to:

1. Support transparent conversion from ChatCompletion requests to Response API format for compatible models
2. Convert Response API responses back to ChatCompletion format for client compatibility
3. Skip conversion for models that only support ChatCompletion API
4. Maintain full feature compatibility including function calling, streaming, and reasoning content

## Architecture

### High-Level Flow

```plaintext
User Request (ChatCompletion)
    ↓
[Model Support Check]
    ↓
┌─ If Model Only Supports ChatCompletion ─→ Direct Processing
│
└─ If Model Supports Response API
    ↓
[Convert to Response API]
    ↓
[Send to Upstream]
    ↓
[Response API Response]
    ↓
[Convert back to ChatCompletion]
    ↓
User Response (ChatCompletion)
```

### Key Components

#### 1. Request Conversion Pipeline

**Location**: `relay/adaptor/openai/adaptor.go`

**Entry Point**: `DoRequest()` method (lines 113-130)

```go
if relayMode == relaymode.ChatCompletions && meta.ChannelType == channeltype.OpenAI {
    // Convert to Response API format
    responseAPIRequest := ConvertChatCompletionToResponseAPI(request)

    // Store converted request in context for response detection
    c.Set(ctxkey.ConvertedRequest, responseAPIRequest)

    return responseAPIRequest, nil
}
```

**Key Condition**: Only converts when:

- Relay mode is ChatCompletion (`relaymode.ChatCompletions`)
- Channel type is OpenAI (`channeltype.OpenAI`)

#### 2. Response Conversion Pipeline

**Location**: `relay/adaptor/openai/adaptor.go`

**Entry Point**: `DoResponse()` method (lines 230-280)

**Detection Logic**:

```go
// Check if we need to convert Response API response back to ChatCompletion format
if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
    if _, ok := vi.(*ResponseAPIRequest); ok {
        // This is a Response API response that needs conversion
        err, usage = ResponseAPIHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
    } else {
        // Regular ChatCompletion request
        err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
    }
}
```

**For Streaming**:

```go
if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
    if _, ok := vi.(*ResponseAPIRequest); ok {
        // This is a Response API streaming response that needs conversion
        err, responseText, usage = ResponseAPIStreamHandler(c, resp, meta.Mode)
    } else {
        // Regular streaming response
        err, responseText, usage = StreamHandler(c, resp, meta.Mode)
    }
}
```

#### 3. Model Support Detection Function

**Current Status**: ✅ **Implemented**
**Function**: `IsModelsOnlySupportedByChatCompletionAPI(model string) bool`
**Location**: `relay/adaptor/openai/response_model.go:15`

**Current Implementation**:

```go
func IsModelsOnlySupportedByChatCompletionAPI(actualModel string) bool {
	switch {
	case strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-search-"):
		return true
	default:
		return false
	}
}
```

**Integration Points**: ✅ **Integrated**

1. **Request Processing** - `adaptor.go:117`:

```go
if relayMode == relaymode.ChatCompletions &&
   meta.ChannelType == channeltype.OpenAI &&
   !IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
    // Proceed with conversion
}
```

2. **URL Generation** - `adaptor.go:84`:

```go
if meta.Mode == relaymode.ChatCompletions &&
   meta.ChannelType == channeltype.OpenAI &&
   !IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
    responseAPIPath := "/v1/responses"
    return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
}
return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
```

**Current Behavior**:

- ✅ **Search Models**: Models containing "gpt" and "-search-" use ChatCompletion API (`/v1/chat/completions`)
- ✅ **Regular Models**: All other models use Response API (`/v1/responses`)
- ✅ **URL Consistency**: Endpoint selection matches conversion logic
- ✅ **Test Coverage**: Comprehensive tests verify both URL generation and conversion consistency

## Core Conversion Functions

### 1. Request Conversion

**Function**: `ConvertChatCompletionToResponseAPI()`
**Location**: `relay/adaptor/openai/response_model.go:105`

**Key Transformations**:

- Messages → Input array
- System message → Instructions field
- Tools → Response API tool format
- Function call history → Text summaries
- Parameters mapping (temperature, top_p, etc.)

**Function Call History Handling**:
The Response API doesn't support ChatCompletion's function call history format. The converter creates text summaries:

```plaintext
Previous function calls:
- Called get_current_datetime({}) → {"year":2025,"month":6,"day":12}
- Called get_weather({"location":"Boston"}) → {"temperature":22,"condition":"sunny"}
```

### 2. Response Conversion (Non-streaming)

**Function**: `ConvertResponseAPIToChatCompletion()`
**Location**: `relay/adaptor/openai/response_model.go:383`

**Handler**: `ResponseAPIHandler()`
**Location**: `relay/adaptor/openai/main.go:330`

**Key Transformations**:

- Output array → Choices array
- Message content → Choice message content
- Function calls → Tool calls
- Status → Finish reason
- Usage field mapping

### 3. Response Conversion (Streaming)

**Function**: `ConvertResponseAPIStreamToChatCompletion()`
**Location**: `relay/adaptor/openai/response_model.go:487`

**Handler**: `ResponseAPIStreamHandler()`
**Location**: `relay/adaptor/openai/main.go:489`

**Stream Event Processing**:

- `response.output_text.delta` → Content deltas
- `response.reasoning_summary_text.delta` → Reasoning deltas
- `response.completed` → Usage information
- Function call events → Tool call deltas

## Data Structure Mappings

### Request Format Mapping

| ChatCompletion Field   | Response API Field  | Notes                                |
| ---------------------- | ------------------- | ------------------------------------ |
| `messages`             | `input`             | Array of message objects             |
| `messages[0]` (system) | `instructions`      | System message moved to instructions |
| `tools`                | `tools`             | Tool format conversion required      |
| `max_tokens`           | `max_output_tokens` | Direct mapping                       |
| `temperature`          | `temperature`       | Direct mapping                       |
| `stream`               | `stream`            | Direct mapping                       |
| `user`                 | `user`              | Direct mapping                       |

### Response Format Mapping

| Response API Field              | ChatCompletion Field           | Notes             |
| ------------------------------- | ------------------------------ | ----------------- |
| `output[].content[].text`       | `choices[].message.content`    | Text content      |
| `output[].summary[].text`       | `choices[].message.reasoning`  | Reasoning content |
| `output[].type="function_call"` | `choices[].message.tool_calls` | Function calls    |
| `status`                        | `choices[].finish_reason`      | Status mapping    |
| `usage.input_tokens`            | `usage.prompt_tokens`          | Token usage       |
| `usage.output_tokens`           | `usage.completion_tokens`      | Token usage       |

### Status Mapping

| Response API Status | ChatCompletion finish_reason | Notes                                    |
| ------------------- | ---------------------------- | ---------------------------------------- |
| `completed`         | `stop` or `tool_calls`       | `tool_calls` when function calls present |
| `failed`            | `stop`                       |                                          |
| `incomplete`        | `length`                     |                                          |
| `cancelled`         | `stop`                       |                                          |

## Function Calling Support

### Request Flow

1. ChatCompletion tools → Response API tools (format conversion)
2. Function call history → Text summaries in input
3. Tool choice → Tool choice (preserved)

### Response Flow

1. Response API function_call output → ChatCompletion tool_calls
2. Call ID mapping with prefix handling (`fc_` ↔ `call_`)
3. Function name and arguments preservation
4. Finish reason set to `tool_calls` when functions present

### Example Conversion

**Input (ChatCompletion)**:

```json
{
  "model": "gpt-4",
  "messages": [
    { "role": "user", "content": "What's the weather?" },
    {
      "role": "assistant",
      "tool_calls": [
        { "id": "call_123", "function": { "name": "get_weather" } }
      ]
    },
    { "role": "tool", "tool_call_id": "call_123", "content": "Sunny, 22°C" }
  ]
}
```

**Converted to Response API**:

```json
{
  "model": "gpt-4",
  "input": [
    { "role": "user", "content": "What's the weather?" },
    {
      "role": "assistant",
      "content": "Previous function calls:\n- Called get_weather() → Sunny, 22°C"
    }
  ]
}
```

## Streaming Implementation

### Event Processing

The streaming handler processes different event types:

- **Delta Events**: `response.output_text.delta`, `response.reasoning_summary_text.delta`

  - Converted to ChatCompletion streaming chunks
  - Content accumulated for token counting

- **Completion Events**: `response.output_text.done`, `response.content_part.done`

  - Discarded to prevent duplicate content
  - Only usage information from `response.completed` is forwarded

- **Function Call Events**: Function call streaming support
  - Converted to tool_call deltas in ChatCompletion format

### Deduplication Strategy

Response API emits both delta and completion events. The implementation:

1. Only processes delta events for content streaming
2. Discards completion events to prevent duplication
3. Forwards usage information from final completion events

## Error Handling

### Parse Errors

- Request conversion errors wrapped with `ErrorWrapper()`
- Response parsing errors logged and processing continues
- Malformed chunks skipped with debug logging

### API Errors

- Response API errors passed through unchanged
- Error format preserved for client compatibility

### Fallback Mechanisms

- Token usage calculation fallback when API doesn't provide usage
- Content extraction fallback for malformed responses

## Testing Infrastructure

### Test Coverage

**Location**: `relay/adaptor/openai/response_model_test.go`

**Key Test Categories**:

- `TestConvertChatCompletionToResponseAPI()` - Request conversion
- `TestConvertResponseAPIToChatCompletion()` - Response conversion
- `TestConvertResponseAPIStreamToChatCompletion()` - Streaming conversion
- `TestFunctionCallWorkflow()` - End-to-end function calling
- `TestChannelSpecificConversion()` - Channel type filtering

### Integration Tests

**Location**: `relay/adaptor/openai/channel_conversion_test.go`

Tests conversion behavior for different channel types:

- OpenAI: Conversion enabled
- Azure, AI360, etc.: Conversion disabled

## Context Management

### Context Keys

**Location**: `common/ctxkey/key.go`

**Key Constant**: `ConvertedRequest = "converted_request"`

**Usage**:

- Request phase: Store converted ResponseAPI request
- Response phase: Detect need for response conversion

### Context Flow

1. **Request**: `c.Set(ctxkey.ConvertedRequest, responseAPIRequest)`
2. **Response**: `c.Get(ctxkey.ConvertedRequest)` to detect conversion need

## Model Support Integration

### Current Implementation

✅ **Function**: `IsModelsOnlySupportedByChatCompletionAPI(modelName string) bool`
**Location**: `relay/adaptor/openai/response_model.go:15`

**Model Detection Logic**:

```go
func IsModelsOnlySupportedByChatCompletionAPI(actualModel string) bool {
	switch {
	case strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-search-"):
		return true
	default:
		return false
	}
}
```

### Model Categories

#### ChatCompletion-Only Models (API: `/v1/chat/completions`)

These models return `true` from `IsModelsOnlySupportedByChatCompletionAPI()`:

- ✅ **Search Models**: `gpt-4-search-*`, `gpt-4o-search-*`, `gpt-3.5-turbo-search-*`
- 🔍 **Pattern**: Contains both "gpt" and "-search-"
- 📍 **Endpoint**: `https://api.openai.com/v1/chat/completions`
- 🔄 **Conversion**: **Disabled** - Request stays in ChatCompletion format

#### Response API Compatible Models (API: `/v1/responses`)

These models return `false` from `IsModelsOnlySupportedByChatCompletionAPI()`:

- ✅ **Regular GPT Models**: `gpt-4`, `gpt-4o`, `gpt-3.5-turbo`
- ✅ **Reasoning Models**: `o1-preview`, `o1-mini`, `o3`
- ✅ **All Other Models**: Any model not matching the ChatCompletion-only pattern
- 📍 **Endpoint**: `https://api.openai.com/v1/responses`
- 🔄 **Conversion**: **Enabled** - ChatCompletion → Response API → ChatCompletion

### Integration Points

#### 1. Request Processing

**Location**: `relay/adaptor/openai/adaptor.go:117`

**✅ Current Implementation**:

```go
if relayMode == relaymode.ChatCompletions &&
   meta.ChannelType == channeltype.OpenAI &&
   !IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
   // Proceed with Response API conversion
}
```

#### 2. URL Generation

**Location**: `relay/adaptor/openai/adaptor.go:84`

**✅ Current Implementation**:

```go
if meta.Mode == relaymode.ChatCompletions &&
   meta.ChannelType == channeltype.OpenAI &&
   !IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
   responseAPIPath := "/v1/responses"
   return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
}
return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
```

### Implementation Strategy

✅ **Completed**:

1. ✅ Function implementation with search model detection
2. ✅ Integration in request conversion logic
3. ✅ Integration in URL generation logic
4. ✅ Comprehensive test coverage
5. ✅ Documentation updates

🔄 **Future Enhancements**:

1. **Dynamic Model Detection**: API-based model capability queries
2. **Configuration-Driven**: External configuration for model support mapping
3. **Runtime Updates**: Dynamic model support updates without code changes
4. **Enhanced Patterns**: More sophisticated model pattern matching

## Performance Considerations

### Memory Management

- Streaming buffers: 1MB buffer for large messages
- Content accumulation: Separate tracking for reasoning vs content
- Context storage: Minimal object stored in gin context

### Processing Efficiency

- Single-pass conversion: Request and response converted once
- Lazy evaluation: Conversion only when needed
- Early detection: Context check before processing

## Future Enhancements

### 1. Dynamic Model Support Detection

- API-based model capability detection
- Configuration-driven model support mapping
- Runtime model support updates

### 2. Enhanced Error Recovery

- Partial response recovery for streaming failures
- Automatic fallback to ChatCompletion for unsupported features

### 3. Performance Optimizations

- Response format detection optimization
- Memory usage optimization for large responses
- Caching for repeated conversions

## Configuration

### Channel Type Detection

**Location**: `relay/channeltype/define.go`

**OpenAI Channel Type**: `channeltype.OpenAI = 1`

### Relay Mode Detection

**Location**: `relay/relaymode/`

**ChatCompletion Mode**: `relaymode.ChatCompletions`

## Summary

The API conversion system provides transparent bidirectional conversion between ChatCompletion and Response API formats, enabling:

1. **Backward Compatibility**: Users can continue using ChatCompletion API
2. **Forward Compatibility**: Access to Response API features and models
3. **Selective Conversion**: Model-specific conversion control
4. **Full Feature Support**: Function calling, streaming, reasoning content
5. **Error Resilience**: Comprehensive error handling and fallbacks

The implementation maintains the familiar ChatCompletion interface while leveraging Response API capabilities when beneficial, with proper safeguards for models that only support the traditional format.

---

# Claude Messages API Conversion Architecture

## Overview

The Claude Messages API conversion system enables users to access various AI models through the standardized Claude Messages API format (`/v1/messages`). This system automatically converts Claude Messages requests to the appropriate format for each adapter (OpenAI, Gemini, Groq, etc.) and converts responses back to Claude Messages format.

## Problem Statement

Different AI providers use different API formats:

- **Anthropic**: Native Claude Messages API format
- **OpenAI-compatible providers**: OpenAI ChatCompletion format
- **Google**: Gemini API format
- **Other providers**: Various proprietary formats

The system needs to:

1. Accept requests in Claude Messages API format
2. Convert to the appropriate format for each adapter
3. Convert responses back to Claude Messages format
4. Maintain full feature compatibility including function calling, streaming, and structured content

## Architecture

### High-Level Flow

```plaintext
User Request (Claude Messages API)
    ↓
[Route to Claude Messages Controller]
    ↓
[Determine Target Adapter]
    ↓
┌─ If Anthropic Adapter ─→ Native Processing
│
└─ If Other Adapter
    ↓
[Convert to Adapter Format]
    ↓
[Send to Upstream via Adapter]
    ↓
[Adapter Response]
    ↓
[Convert back to Claude Messages]
    ↓
User Response (Claude Messages API)
```

### Key Components

#### 1. Claude Messages Controller

**Location**: `relay/controller/claude_messages.go`

**Entry Point**: `RelayClaudeMessagesHelper()` method

**Key Responsibilities**:

- Accept Claude Messages API requests at `/v1/messages`
- Route to appropriate adapter based on model
- Handle response conversion coordination
- Manage streaming and non-streaming responses

#### 2. Adapter Conversion Interface

**Interface Methods**:

- `ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error)`
- `DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode)`

**Conversion Patterns**:

1. **Native Support** (Anthropic):

   - Sets `ClaudeMessagesNative` flag
   - Uses native Claude handlers directly

2. **Conversion Support** (OpenAI, Gemini, etc.):
   - Sets `ClaudeMessagesConversion` flag
   - Converts request format in `ConvertClaudeRequest`
   - Converts response format in `DoResponse`

#### 3. Shared OpenAI-Compatible Conversion

**Location**: `relay/adaptor/openai_compatible/claude_messages.go`

**Function**: `ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error)`

**Used by**: All OpenAI-compatible adapters (DeepSeek, Groq, Mistral, XAI, etc.)

## Adapter Implementation Patterns

### Pattern 1: Native Claude Support (Anthropic)

```go
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
    // Set native processing flags
    c.Set(ctxkey.ClaudeMessagesNative, true)
    c.Set(ctxkey.ClaudeDirectPassthrough, true)

    return request, nil
}
```

### Pattern 2: OpenAI-Compatible Conversion

```go
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
    // Use shared OpenAI-compatible conversion
    return openai_compatible.ConvertClaudeRequest(c, request)
}
```

### Pattern 3: Custom Conversion (Gemini)

```go
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
    // Convert to OpenAI format first
    openaiRequest := convertClaudeToOpenAI(request)

    // Set conversion flags
    c.Set(ctxkey.ClaudeMessagesConversion, true)
    c.Set(ctxkey.OriginalClaudeRequest, request)

    // Use Gemini's existing conversion logic
    return a.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
    // Check for Claude Messages conversion
    if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
        // Convert Gemini response to Claude format
        claudeResp, convertErr := a.convertToClaudeResponse(c, resp, meta)
        if convertErr != nil {
            return nil, convertErr
        }

        // Set converted response for controller to use
        c.Set(ctxkey.ConvertedResponse, claudeResp)
        return nil, nil
    }

    // Normal processing for non-Claude requests
    return a.normalDoResponse(c, resp, meta)
}
```

## Supported Adapters

### ✅ Native Claude Messages Support

- **Anthropic**: Native Claude Messages API support

### ✅ OpenAI-Compatible Conversion

These adapters use the shared `openai_compatible.ConvertClaudeRequest()`:

- **DeepSeek**: `relay/adaptor/deepseek/adaptor.go`
- **Moonshot**: `relay/adaptor/moonshot/adaptor.go`
- **Groq**: `relay/adaptor/groq/adaptor.go`
- **Mistral**: `relay/adaptor/mistral/adaptor.go`
- **XAI**: `relay/adaptor/xai/adaptor.go`
- **TogetherAI**: `relay/adaptor/togetherai/adaptor.go`
- **OpenRouter**: `relay/adaptor/openrouter/adaptor.go`
- **SiliconFlow**: `relay/adaptor/siliconflow/adaptor.go`
- **Doubao**: `relay/adaptor/doubao/adaptor.go`
- **StepFun**: `relay/adaptor/stepfun/adaptor.go`
- **Novita**: `relay/adaptor/novita/adaptor.go`
- **AIProxy**: `relay/adaptor/aiproxy/adaptor.go`
- **LingYiWanWu**: `relay/adaptor/lingyiwanwu/adaptor.go`
- **AI360**: `relay/adaptor/ai360/adaptor.go`

### ✅ Custom Conversion

These adapters implement custom Claude Messages conversion logic:

- **Anthropic**: Native Claude support with direct pass-through
- **OpenAI**: Full conversion with response format transformation
- **Gemini**: Custom conversion with response format transformation
- **Ali**: Custom conversion implementation
- **Baidu**: Custom conversion implementation
- **Zhipu**: Custom conversion implementation
- **Xunfei**: Custom conversion implementation
- **Tencent**: Custom conversion implementation
- **AWS**: Custom conversion for Bedrock Claude models
- **VertexAI**: Custom conversion with sub-adapter routing
- **Replicate**: Custom conversion implementation
- **Cohere**: Custom conversion implementation
- **Cloudflare**: Custom conversion implementation
- **Palm**: Basic text-only conversion support
- **Ollama**: Basic text-only conversion support
- **Coze**: Basic text-only conversion support

### ❌ Limited or No Support

These adapters have limited or no Claude Messages support:

- **DeepL**: Translation service, not applicable for chat completion
- **Minimax**: Stub implementation, returns "not implemented" error
- **Baichuan**: Stub implementation, returns "not implemented" error

### 📋 Test Results Summary

Based on comprehensive testing:

- **✅ Fully Working**: 25+ adapters with complete Claude Messages support
- **⚠️ Configuration Required**: Some adapters (Baidu, Tencent, VertexAI) require valid API keys/configuration
- **❌ Not Applicable**: 3 adapters (DeepL, Minimax, Baichuan) correctly return appropriate errors

## Data Structure Mappings

### Claude Messages to OpenAI Format

| Claude Messages Field | OpenAI Field  | Notes                              |
| --------------------- | ------------- | ---------------------------------- |
| `model`               | `model`       | Direct mapping                     |
| `max_tokens`          | `max_tokens`  | Direct mapping                     |
| `messages`            | `messages`    | Message format conversion required |
| `system`              | `messages[0]` | System message as first message    |
| `tools`               | `tools`       | Tool format conversion required    |
| `tool_choice`         | `tool_choice` | Direct mapping                     |
| `temperature`         | `temperature` | Direct mapping                     |
| `top_p`               | `top_p`       | Direct mapping                     |
| `stream`              | `stream`      | Direct mapping                     |
| `stop_sequences`      | `stop`        | Direct mapping                     |

### Message Content Conversion

#### Text Content

```json
// Claude Messages
{"role": "user", "content": "Hello"}

// OpenAI
{"role": "user", "content": "Hello"}
```

#### Structured Content

```json
// Claude Messages
{
  "role": "user",
  "content": [
    {"type": "text", "text": "Hello"},
    {"type": "image", "source": {"type": "base64", "media_type": "image/jpeg", "data": "..."}}
  ]
}

// OpenAI
{
  "role": "user",
  "content": [
    {"type": "text", "text": "Hello"},
    {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,..."}}
  ]
}
```

#### Tool Use

```json
// Claude Messages
{
  "role": "assistant",
  "content": [
    {"type": "tool_use", "id": "toolu_123", "name": "get_weather", "input": {"location": "NYC"}}
  ]
}

// OpenAI
{
  "role": "assistant",
  "tool_calls": [
    {"id": "toolu_123", "type": "function", "function": {"name": "get_weather", "arguments": "{\"location\":\"NYC\"}"}}
  ]
}
```

### Response Format Conversion

#### OpenAI to Claude Messages

| OpenAI Field                    | Claude Messages Field | Notes                               |
| ------------------------------- | --------------------- | ----------------------------------- |
| `id`                            | `id`                  | Generate Claude-style ID if missing |
| `choices[0].message.content`    | `content[0].text`     | Text content                        |
| `choices[0].message.tool_calls` | `content[].tool_use`  | Tool calls conversion               |
| `choices[0].finish_reason`      | `stop_reason`         | Reason mapping required             |
| `usage.prompt_tokens`           | `usage.input_tokens`  | Direct mapping                      |
| `usage.completion_tokens`       | `usage.output_tokens` | Direct mapping                      |

#### Finish Reason Mapping

| OpenAI finish_reason | Claude stop_reason | Notes               |
| -------------------- | ------------------ | ------------------- |
| `stop`               | `end_turn`         | Normal completion   |
| `length`             | `max_tokens`       | Token limit reached |
| `tool_calls`         | `tool_use`         | Function calling    |
| `content_filter`     | `stop_sequence`    | Content filtered    |

## Context Management

### Context Keys

**Location**: `common/ctxkey/key.go`

**Key Constants**:

- `ClaudeMessagesConversion = "claude_messages_conversion"`
- `ClaudeMessagesNative = "claude_messages_native"`
- `ClaudeDirectPassthrough = "claude_direct_passthrough"`
- `OriginalClaudeRequest = "original_claude_request"`
- `ConvertedResponse = "converted_response"`

### Context Flow

1. **Request Phase**:

   - `ConvertClaudeRequest()` sets conversion flags
   - Original request stored for reference

2. **Response Phase**:

   - `DoResponse()` checks conversion flags
   - Converts response format if needed
   - Sets converted response in context

3. **Controller Phase**:
   - Controller checks for converted response
   - Uses converted response or falls back to native handlers

## Error Handling

### Conversion Errors

- Request conversion errors wrapped with proper error types
- Response parsing errors logged with debug information
- Malformed content handled gracefully with fallbacks

### Adapter Errors

- Upstream adapter errors passed through unchanged
- Error format preserved for client compatibility
- Proper HTTP status codes maintained

### Fallback Mechanisms

- Token usage calculation fallback when adapter doesn't provide usage
- Content extraction fallback for malformed responses
- Default values for missing required fields

## Performance Considerations

### Memory Management

- Minimal context storage for conversion flags
- Efficient message content transformation
- Streaming support with proper buffer management

### Processing Efficiency

- Single-pass conversion for request and response
- Lazy evaluation - conversion only when needed
- Early detection of conversion requirements

## Testing

### Test Coverage

**Locations**:

- `relay/adaptor/gemini/adaptor_test.go` - Gemini conversion tests
- `relay/adaptor/openai_compatible/claude_messages_test.go` - Shared conversion tests
- Individual adapter test files for specific conversion logic

**Key Test Categories**:

- Request format conversion
- Response format conversion
- Streaming conversion
- Function calling workflows
- Error handling scenarios

## Future Enhancements

### 1. Enhanced Content Support

- Support for more Claude Messages content types
- Better handling of complex structured content
- Improved image and file handling

### 2. Performance Optimizations

- Response format detection optimization
- Memory usage optimization for large messages
- Caching for repeated conversions

### 3. Extended Adapter Support

- Support for more specialized adapters
- Dynamic adapter capability detection
- Runtime adapter registration

## Summary

The Claude Messages API conversion system provides:

1. **Universal Access**: Single API endpoint for multiple AI providers
2. **Format Transparency**: Automatic format conversion between different APIs
3. **Feature Preservation**: Full support for function calling, streaming, and structured content
4. **Extensible Architecture**: Easy addition of new adapters
5. **Error Resilience**: Comprehensive error handling and fallbacks

The implementation allows users to interact with various AI models through the familiar Claude Messages API while maintaining compatibility with each provider's native capabilities.
