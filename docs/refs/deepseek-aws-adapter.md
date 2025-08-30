# DeepSeek AWS Adapter Documentation

## Overview

The DeepSeek AWS Adapter provides seamless integration with AWS Bedrock's DeepSeek-R1 model through the Converse API. This adapter enables both streaming and non-streaming chat completions with advanced reasoning content support, making it compatible with OpenAI's chat completion format while leveraging AWS Bedrock's infrastructure.

## Features

### Core Capabilities
- **OpenAI Compatibility**: Full compatibility with OpenAI's chat completion API format
- **Streaming Support**: Real-time streaming responses with proper event handling
- **Reasoning Content**: Native support for DeepSeek-R1's reasoning capabilities
- **AWS Converse API**: Utilizes AWS Bedrock's Converse API for optimal performance
- **Token Usage Tracking**: Accurate token counting for billing and monitoring
- **Cross-Region Support**: Works with AWS cross-region inference profiles

### Advanced Features
- **Reasoning Content Streaming**: Separate handling of reasoning content in streaming responses
- **Stop Sequence Support**: Configurable stop sequences for response control
- **Temperature and TopP Control**: Fine-grained control over response generation
- **Error Handling**: Robust error handling with proper status codes
- **Request Conversion**: Automatic conversion between OpenAI and AWS Bedrock formats

## Supported Models

| OpenAI Model Name | AWS Bedrock Model ID | Description |
|-------------------|---------------------|-------------|
| `deepseek-r1` | `deepseek.r1-v1:0` | DeepSeek-R1 reasoning model |

## Configuration

### Environment Variables
No additional environment variables are required beyond standard AWS Bedrock configuration:
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY` 
- `AWS_REGION`

### Channel Configuration
When creating a channel in One API:
1. Set **Type** to `AWS Claude`
2. Set **Model** to `deepseek-r1`
3. Configure AWS credentials and region
4. Enable cross-region inference if needed

## API Usage

### Non-Streaming Request
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token" \
  -d '{
    "model": "deepseek-r1",
    "messages": [
      {
        "role": "user", 
        "content": "Explain quantum computing in simple terms"
      }
    ],
    "max_tokens": 1000,
    "temperature": 0.7,
    "top_p": 0.9
  }'
```

### Streaming Request
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token" \
  -d '{
    "model": "deepseek-r1",
    "messages": [
      {
        "role": "user",
        "content": "Write a Python function to calculate fibonacci numbers"
      }
    ],
    "stream": true,
    "max_tokens": 500
  }'
```

## Response Format

### Non-Streaming Response
```json
{
  "id": "chatcmpl-oneapi-abc123",
  "object": "chat.completion",
  "created": 1693392000,
  "model": "deepseek-r1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": [
          {
            "text": "Here's a simple explanation of quantum computing..."
          },
          {
            "reasoningContent": {
              "reasoningText": "Let me think about how to explain this complex topic in simple terms..."
            }
          }
        ]
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 150,
    "total_tokens": 175
  }
}
```

### Streaming Response
```
data: {"id":"chatcmpl-oneapi-abc123","object":"chat.completion.chunk","created":1693392000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think about this..."}}]}

data: {"id":"chatcmpl-oneapi-abc123","object":"chat.completion.chunk","created":1693392000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"content":"Here's a simple explanation..."}}]}

data: {"id":"chatcmpl-oneapi-abc123","object":"chat.completion.chunk","created":1693392000,"model":"deepseek-r1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

## Implementation Details

### Architecture
The adapter consists of four main components:
- **`main.go`**: Core request/response handlers and streaming logic
- **`adapter.go`**: Interface implementation for the AWS adapter pattern
- **`model.go`**: Data structures and type definitions
- **`main_test.go`**: Comprehensive test coverage

### Request Processing Flow
1. **Request Conversion**: OpenAI format → DeepSeek internal format
2. **AWS Conversion**: DeepSeek format → AWS Converse API format
3. **API Call**: Execute AWS Bedrock Converse/ConverseStream API
4. **Response Conversion**: AWS response → OpenAI compatible format
5. **Streaming**: Handle real-time content and reasoning deltas

### Reasoning Content Handling
DeepSeek-R1's reasoning content is handled specially:
- **Non-streaming**: Reasoning content appears as separate content blocks
- **Streaming**: Reasoning content is streamed via `reasoning_content` field
- **OpenAI Compatibility**: Maintains compatibility while preserving reasoning data

#### Important Streaming Behavior
**Note**: In streaming responses, reasoning content and regular content are streamed together in the same response stream. This requires careful handling in client-side implementations:

- **Mixed Content Streaming**: Both `reasoning_content` and regular `content` deltas can arrive in any order within the same stream
- **Client-Side Processing**: Applications must be prepared to handle both content types simultaneously
- **Chatbot Considerations**: Chat applications should implement proper logic to:
  - Display reasoning content separately from the main response (if desired)
  - Handle the interleaved nature of reasoning and content chunks
  - Maintain proper message ordering and presentation
  - Consider whether to show reasoning content to end users or use it for internal processing only

#### Stream Processing Recommendations
For robust client-side implementation:
1. **Buffer Management**: Maintain separate buffers for reasoning content and regular content
2. **Content Type Detection**: Check each delta for either `content` or `reasoning_content` fields
3. **UI Handling**: Decide how to present reasoning content in your user interface
4. **Error Recovery**: Implement proper error handling for interrupted streams containing mixed content types

### Error Handling
The adapter provides comprehensive error handling:
- Model ID validation
- AWS API error conversion
- Request format validation
- Streaming error recovery

## Performance Considerations

### Optimization Features
- **Efficient Streaming**: Minimal buffering for real-time responses
- **Token Accuracy**: Precise token counting using AWS Converse API
- **Memory Management**: Proper cleanup of streaming connections
- **Connection Pooling**: Leverages AWS SDK connection pooling

### Best Practices
- Use streaming for long responses to improve user experience
- Configure appropriate timeouts for your use case
- Monitor token usage for cost optimization
- Implement proper retry logic for production usage
- **Mixed Content Handling**: When implementing streaming clients, ensure proper handling of interleaved reasoning and content deltas
- **Chatbot Integration**: For chatbot applications, implement separate rendering logic for reasoning content vs. main response content
- **Real-time Processing**: Design your client to process both content types in real-time without blocking the stream
- **Content Validation**: Always check for both `content` and `reasoning_content` fields in each streaming delta

## Testing

### Unit Tests
The adapter includes comprehensive unit tests covering:
- Message conversion functions
- Request parameter handling
- Stop sequence processing
- Model ID validation

### Running Tests
```bash
cd relay/adaptor/aws/deepseek
go test -v ./...
```

## Troubleshooting

### Common Issues

**Model Not Found Error**
```
Error: model deepseek-r1 not found
```
- Verify the model name is exactly `deepseek-r1`
- Check AWS Bedrock model availability in your region

**AWS Authentication Error**
```
Error: invalid AWS credentials
```
- Verify AWS credentials are properly configured
- Check IAM permissions for Bedrock access

**Streaming Connection Issues**
```
Error: stream connection interrupted
```
- Check network connectivity
- Verify proxy settings if applicable
- Implement proper retry logic

### Debug Mode
Enable debug logging by setting:
```bash
DEBUG=true
DEBUG_SQL=true
```

## Limitations

- Only supports the `deepseek-r1` model
- Requires AWS Bedrock access in supported regions
- Reasoning content format may differ from other providers
- Maximum token limits are governed by AWS Bedrock
