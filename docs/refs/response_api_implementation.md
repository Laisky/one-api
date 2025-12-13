# Response API Implementation

## Overview

This document describes the implementation of conversion from Response API format back to ChatCompletion format in the OpenAI adaptor. This ensures that when the system converts ChatCompletion requests to Response API format for upstream processing, the responses are converted back to the familiar ChatCompletion format before being returned to users.

## Implementation Summary

### Key Components

1. **Request Conversion (Already Implemented)**
   - `ConvertChatCompletionToResponseAPI()` in `response_model.go`
   - Converts ChatCompletion requests to Response API format when `relayMode == relaymode.ChatCompletions`
   - Located at line 118 in `adaptor.go`

2. **Response Conversion (New Implementation)**
   - `ConvertResponseAPIToChatCompletion()` in `response_model.go`
   - `ConvertResponseAPIStreamToChatCompletion()` in `response_model.go`
   - `ResponseAPIHandler()` in `main.go`
   - `ResponseAPIStreamHandler()` in `main.go`

### Request Flow

1. **Request Processing** (`adaptor.go:112-122`)
   ```go
   if relayMode == relaymode.ChatCompletions {
       // Convert to Response API format
       responseAPIRequest := ConvertChatCompletionToResponseAPI(request)

       // Store in context for response detection
       c.Set(ctxkey.ConvertedRequest, responseAPIRequest)

       return responseAPIRequest, nil
   }
   ```

2. **Response Detection** (`adaptor.go:233-249`)
   ```go
   case relaymode.ChatCompletions:
       // Check if we need to convert Response API response back
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

3. **Streaming Response Detection** (`adaptor.go:224-237`)
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

### Data Structure Mapping

#### OutputItem Structure Enhancement

The `OutputItem` struct was enhanced to support function calls:

```go
type OutputItem struct {
    Type      string `json:"type"`
    ID        string `json:"id,omitempty"`
    Status    string `json:"status,omitempty"`
    Role      string `json:"role,omitempty"`
    Content   []ContentItem `json:"content,omitempty"`

    // Function call fields
    CallId    string `json:"call_id,omitempty"`
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`
}
```

**Supported Output Item Types:**
- `"message"` - Regular assistant messages with text content
- `"reasoning"` - Reasoning/thinking content (for reasoning models)
- `"function_call"` - Function calls with call ID, name, and arguments

#### Response API Response Format
```json
{
  "id": "resp_123",
  "object": "response",
  "created_at": 1234567890,
  "status": "completed",
  "model": "gpt-4",
  "output": [
    {
      "type": "message",
      "id": "msg_123",
      "status": "completed",
      "role": "assistant",
      "content": [
        {
          "type": "output_text",
          "text": "Hello! How can I help you today?",
          "annotations": []
        }
      ]
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 8,
    "total_tokens": 18
  }
}
```

#### ChatCompletion Response Format (Output)
```json
{
  "id": "resp_123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 8,
    "total_tokens": 18
  }
}
```

#### Function Calling Response Formats

**Response API Function Call Format:**
```json
{
  "id": "resp_123",
  "object": "response",
  "created_at": 1234567890,
  "status": "completed",
  "model": "gpt-4",
  "output": [
    {
      "type": "function_call",
      "id": "call_123",
      "status": "completed",
      "call_id": "call_abc123",
      "name": "get_weather",
      "arguments": "{\"location\": \"San Francisco\"}"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 10,
    "total_tokens": 25
  }
}
```

**ChatCompletion Function Call Format (Output):**
```json
{
  "id": "resp_123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_abc123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"location\": \"San Francisco\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 10,
    "total_tokens": 25
  }
}
```

### Status Mapping

| Response API Status | ChatCompletion finish_reason | Notes |
|-------------------|------------------------------|-------|
| `completed`       | `stop` or `tool_calls`       | `tool_calls` when function calls present |
| `failed`          | `stop`                       | |
| `incomplete`      | `length`                     | |
| `cancelled`       | `stop`                       | |

**Function Call Detection:**
- When output contains items with `type: "function_call"`, the finish reason is set to `tool_calls`
- Function calls are converted to `tool_calls` array in the message
- Both regular messages and function calls can be present in the same response

### Error Handling

- Parse errors are wrapped with `ErrorWrapper()` for consistent error format
- API errors from Response API are passed through unchanged
- Stream parsing errors are logged and processing continues
- Malformed chunks are skipped with error logging

### Features Supported

1. **Content Types**
   - `output_text` → standard message content
   - `reasoning` → reasoning content (if present)

2. **Function Calling / Tools**
   - `function_call` output items → ChatCompletion tool calls
   - Proper mapping of call ID, function name, and arguments
   - Support for both streaming and non-streaming function calls
   - Full round-trip conversion (ChatCompletion tools → Response API → function calls → ChatCompletion tools)

3. **Streaming**
   - Line-by-line processing of SSE stream
   - Conversion of each chunk to ChatCompletion streaming format
   - Proper `[DONE]` handling
   - Function call support in streaming responses

4. **Usage Tracking**
   - Token usage from Response API preserved
   - Fallback calculation if usage not provided
   - Reasoning tokens handled appropriately

5. **Reasoning Content**
   - Reasoning text extraction and formatting
   - Support for different reasoning formats via query parameter

## File Structure

```
relay/adaptor/openai/
├── adaptor.go              # Main adaptor logic with request/response routing
├── main.go                 # Response handlers (ResponseAPIHandler, ResponseAPIStreamHandler)
├── response_model.go       # Data structures and conversion functions
├── response_model_test.go  # Comprehensive test suite
└── docs/
    ├── response_api.md                 # Response API documentation
    └── response_api_implementation.md  # This file
```

## Testing

Comprehensive test suite added to `response_model_test.go`:

- `TestConvertResponseAPIToChatCompletion()` - Tests non-streaming conversion
- `TestConvertResponseAPIStreamToChatCompletion()` - Tests streaming conversion
- `TestConvertResponseAPIToChatCompletionWithFunctionCall()` - Tests function call conversion
- `TestConvertResponseAPIStreamToChatCompletionWithFunctionCall()` - Tests streaming function calls
- `TestFunctionCallWorkflow()` - Tests complete end-to-end function calling workflow
- Status mapping verification
- Content extraction verification
- Usage preservation verification
- Function call mapping verification

All existing tests continue to pass, ensuring no regressions. The function calling tests verify:
- Proper conversion of `function_call` output items to `tool_calls`
- Correct mapping of call ID, function name, and arguments
- Appropriate finish reason setting (`tool_calls` vs `stop`)
- Support for both streaming and non-streaming scenarios

## Usage Example

The conversion is transparent to users. When a ChatCompletion request is made:

1. System automatically detects it's a ChatCompletion request
2. Converts to Response API format for upstream processing (including tool definitions)
3. Stores conversion context for response handling
4. When response arrives, detects it needs conversion
5. Converts Response API response back to ChatCompletion format (preserving function calls)
6. Returns familiar ChatCompletion format to user

This ensures backward compatibility while leveraging the new Response API capabilities. Function calling is fully supported - tools defined in ChatCompletion requests are preserved through the conversion, and function calls in Response API responses are properly converted back to `tool_calls` in the ChatCompletion format.
