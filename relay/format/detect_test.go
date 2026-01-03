package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectFormat_Ambiguous tests that ambiguous requests return Unknown
// to preserve backward compatibility. These payloads could be valid for
// either ChatCompletion or Claude Messages, so we don't try to distinguish them.
func TestDetectFormat_Ambiguous(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		// ==========================================================================
		// Basic message formats - shared between ChatCompletion and Claude
		// ==========================================================================
		{
			name: "simple messages - could be either ChatCompletion or Claude",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with max_tokens - common to both APIs",
			body: `{"model": "gpt-4", "max_tokens": 1024, "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with temperature - common to both APIs",
			body: `{"model": "gpt-4", "temperature": 0.7, "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with top_p - common to both APIs",
			body: `{"model": "gpt-4", "top_p": 0.9, "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with stream true - common to both APIs",
			body: `{"model": "gpt-4", "stream": true, "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with stream false - common to both APIs",
			body: `{"model": "gpt-4", "stream": false, "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with all common parameters",
			body: `{
				"model": "gpt-4",
				"max_tokens": 2048,
				"temperature": 0.7,
				"top_p": 0.9,
				"stream": true,
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
		},

		// ==========================================================================
		// System message variations
		// ==========================================================================
		{
			name: "messages with system role in array (OpenAI style)",
			body: `{"model": "gpt-4", "messages": [{"role": "system", "content": "You are helpful"}, {"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with top-level system string field - ambiguous",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"system": "You are a helpful assistant",
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
		},
		{
			name: "messages with top-level system array field - ambiguous",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"system": [{"type": "text", "text": "You are helpful"}],
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
		},
		{
			name: "messages with both system field and system role - ambiguous edge case",
			body: `{
				"model": "gpt-4",
				"system": "Top level system",
				"messages": [
					{"role": "system", "content": "System in messages"},
					{"role": "user", "content": "Hello"}
				]
			}`,
		},

		// ==========================================================================
		// Tool formats - OpenAI style (type=function with nested function object)
		// ==========================================================================
		{
			name: "messages with OpenAI-style tools (type=function)",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "What is the weather?"}],
				"tools": [{
					"type": "function",
					"function": {
						"name": "get_weather",
						"description": "Get the weather",
						"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
					}
				}]
			}`,
		},
		{
			name: "messages with multiple OpenAI-style tools",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Help me"}],
				"tools": [
					{
						"type": "function",
						"function": {
							"name": "get_weather",
							"description": "Get the weather",
							"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
						}
					},
					{
						"type": "function",
						"function": {
							"name": "search",
							"description": "Search the web",
							"parameters": {"type": "object", "properties": {"query": {"type": "string"}}}
						}
					}
				]
			}`,
		},
		{
			name: "messages with tool_choice auto",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "What is the weather?"}],
				"tools": [{"type": "function", "function": {"name": "get_weather", "parameters": {}}}],
				"tool_choice": "auto"
			}`,
		},
		{
			name: "messages with tool_choice none",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "What is the weather?"}],
				"tools": [{"type": "function", "function": {"name": "get_weather", "parameters": {}}}],
				"tool_choice": "none"
			}`,
		},

		// ==========================================================================
		// Assistant messages with tool_calls (OpenAI style - NOT Claude tool_use)
		// ==========================================================================
		{
			name: "messages with assistant tool_calls (OpenAI style)",
			body: `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "Get weather"},
					{"role": "assistant", "content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{}"}}]},
					{"role": "tool", "tool_call_id": "call_1", "content": "Sunny"}
				]
			}`,
		},
		{
			name: "messages with multiple tool_calls in assistant message",
			body: `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "Get weather and search"},
					{"role": "assistant", "tool_calls": [
						{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{}"}},
						{"id": "call_2", "type": "function", "function": {"name": "search", "arguments": "{}"}}
					]},
					{"role": "tool", "tool_call_id": "call_1", "content": "Sunny"},
					{"role": "tool", "tool_call_id": "call_2", "content": "Results"}
				]
			}`,
		},

		// ==========================================================================
		// Multimodal content - shared types (text, image_url, image)
		// ==========================================================================
		{
			name: "messages with multimodal content (text type)",
			body: `{
				"model": "gpt-4-vision",
				"messages": [{
					"role": "user",
					"content": [{"type": "text", "text": "What is in this image?"}]
				}]
			}`,
		},
		{
			name: "messages with multimodal content (text + image_url)",
			body: `{
				"model": "gpt-4-vision",
				"messages": [{
					"role": "user",
					"content": [
						{"type": "text", "text": "What is in this image?"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
					]
				}]
			}`,
		},
		{
			name: "messages with image type (shared between APIs)",
			body: `{
				"model": "gpt-4-vision",
				"messages": [{
					"role": "user",
					"content": [
						{"type": "text", "text": "Describe this"},
						{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "..."}}
					]
				}]
			}`,
		},

		// ==========================================================================
		// Stop sequences - common to both APIs
		// ==========================================================================
		{
			name: "messages with stop string",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Hello"}],
				"stop": "END"
			}`,
		},
		{
			name: "messages with stop array",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Hello"}],
				"stop": ["END", "STOP"]
			}`,
		},
		{
			name: "messages with stop_sequences (Claude style but ambiguous)",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"messages": [{"role": "user", "content": "Hello"}],
				"stop_sequences": ["END"]
			}`,
		},

		// ==========================================================================
		// Model name variations - should not affect detection
		// ==========================================================================
		{
			name: "messages with OpenAI model name",
			body: `{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with Claude model name",
			body: `{"model": "claude-3-5-sonnet-20241022", "messages": [{"role": "user", "content": "Hello"}]}`,
		},
		{
			name: "messages with custom/unknown model name",
			body: `{"model": "my-custom-model", "messages": [{"role": "user", "content": "Hello"}]}`,
		},

		// ==========================================================================
		// Complex conversation patterns
		// ==========================================================================
		{
			name: "multi-turn conversation",
			body: `{
				"model": "gpt-4",
				"messages": [
					{"role": "system", "content": "You are helpful"},
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi there!"},
					{"role": "user", "content": "How are you?"}
				]
			}`,
		},
		{
			name: "conversation with mixed content types",
			body: `{
				"model": "gpt-4-vision",
				"messages": [
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi!"},
					{"role": "user", "content": [{"type": "text", "text": "Look at this"}, {"type": "image_url", "image_url": {"url": "https://example.com/img.jpg"}}]}
				]
			}`,
		},

		// ==========================================================================
		// Edge cases with empty or minimal content
		// ==========================================================================
		{
			name: "messages with empty content string",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": ""}]}`,
		},
		{
			name: "messages with null content",
			body: `{"model": "gpt-4", "messages": [{"role": "assistant", "content": null}]}`,
		},
		{
			name: "messages with empty content array",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": []}]}`,
		},

		// ==========================================================================
		// Additional parameters that are common
		// ==========================================================================
		{
			name: "messages with user field",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}], "user": "user123"}`,
		},
		{
			name: "messages with metadata",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}], "metadata": {"key": "value"}}`,
		},
		{
			name: "messages with top_k (Claude param but some OpenAI-compatible APIs support it)",
			body: `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}], "top_k": 40}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, Unknown, format, "ambiguous format should return Unknown for backward compatibility")
		})
	}
}

func TestDetectFormat_ResponseAPI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{
			name: "simple response api with string input",
			body: `{"model": "gpt-4", "input": "Hello, how are you?"}`,
		},
		{
			name: "response api with array input",
			body: `{"model": "gpt-4", "input": [{"type": "text", "text": "Hello"}]}`,
		},
		{
			name: "response api with instructions",
			body: `{"model": "gpt-4", "input": "Hello", "instructions": "Be concise"}`,
		},
		{
			name: "response api with max_output_tokens",
			body: `{"model": "gpt-4", "input": "Hello", "max_output_tokens": 1000}`,
		},
		{
			name: "response api with tools",
			body: `{
				"model": "gpt-4",
				"input": "What is the weather?",
				"tools": [{"type": "function", "name": "get_weather", "description": "Get weather"}]
			}`,
		},
		{
			name: "response api with previous_response_id",
			body: `{"model": "gpt-4", "input": "Continue", "previous_response_id": "resp_123"}`,
		},
		{
			name: "response api with multimodal input",
			body: `{
				"model": "gpt-4o",
				"input": [
					{"type": "input_text", "text": "Describe this image"},
					{"type": "input_image", "image_url": "https://example.com/img.png"}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, ResponseAPI, format, "expected ResponseAPI format")
		})
	}
}

func TestDetectFormat_ClaudeMessages(t *testing.T) {
	t.Parallel()
	// Only test Claude-EXCLUSIVE features that cannot be ChatCompletion
	tests := []struct {
		name string
		body string
	}{
		{
			name: "claude with tool_use content block (Claude-exclusive)",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Get weather"},
					{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {}}]}
				]
			}`,
		},
		{
			name: "claude with tool_result content block (Claude-exclusive)",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_1", "content": "Sunny"}]}
				]
			}`,
		},
		{
			name: "claude with input_schema tools (Claude-exclusive tool format)",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"messages": [{"role": "user", "content": "Hello"}],
				"tools": [{
					"name": "get_weather",
					"description": "Get the weather",
					"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
				}]
			}`,
		},
		{
			name: "claude with thinking content block (Claude-exclusive)",
			body: `{
				"model": "claude-3-opus",
				"max_tokens": 1024,
				"messages": [
					{"role": "assistant", "content": [{"type": "thinking", "thinking": "Let me think..."}]}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, ClaudeMessages, format, "expected ClaudeMessages format for Claude-exclusive features")
		})
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty object",
			body: `{}`,
		},
		{
			name: "only model",
			body: `{"model": "gpt-4"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, Unknown, format, "expected Unknown format")
		})
	}
}

func TestDetectFormat_Error(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty body",
			body: "",
		},
		{
			name: "invalid json",
			body: "{invalid}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := DetectFormat([]byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestAPIFormat_String(t *testing.T) {
	t.Parallel()
	require.Equal(t, "chat_completion", ChatCompletion.String())
	require.Equal(t, "response_api", ResponseAPI.String())
	require.Equal(t, "claude_messages", ClaudeMessages.String())
	require.Equal(t, "unknown", Unknown.String())
}

func TestAPIFormat_Endpoint(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/v1/chat/completions", ChatCompletion.Endpoint())
	require.Equal(t, "/v1/responses", ResponseAPI.Endpoint())
	require.Equal(t, "/v1/messages", ClaudeMessages.Endpoint())
	require.Equal(t, "", Unknown.Endpoint())
}

func TestFormatFromPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected APIFormat
	}{
		{"/v1/chat/completions", ChatCompletion},
		{"/v1/chat/completions/", ChatCompletion},
		{"/v1/responses", ResponseAPI},
		{"/v1/responses/resp_123", ResponseAPI},
		{"/v1/messages", ClaudeMessages},
		{"/v1/messages/", ClaudeMessages},
		{"/v1/embeddings", Unknown},
		{"/v1/models", Unknown},
		{"/", Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			format := FormatFromPath(tt.path)
			require.Equal(t, tt.expected, format)
		})
	}
}

// TestDetectFormat_RealWorldCursor tests detection with real-world Cursor requests
// that users have reported being sent to wrong endpoints.
func TestDetectFormat_RealWorldCursor(t *testing.T) {
	t.Parallel()
	// Cursor sometimes sends Response API format to /v1/chat/completions
	cursorResponseFormatRequest := `{
		"model": "claude-3-5-sonnet-20241022",
		"input": [
			{"type": "input_text", "text": "Write a hello world program"}
		],
		"max_output_tokens": 8096,
		"stream": true
	}`

	format, err := DetectFormat([]byte(cursorResponseFormatRequest))
	require.NoError(t, err)
	require.Equal(t, ResponseAPI, format, "should detect Response API format even when sent to wrong endpoint")
}

// TestDetectFormat_EdgeCases tests edge cases and ambiguous payloads.
func TestDetectFormat_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     string
		expected APIFormat
	}{
		{
			name: "messages with input - has messages so check for Claude indicators, none found = Unknown",
			body: `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Hello"}],
				"input": "This should be ignored"
			}`,
			expected: Unknown,
		},
		{
			name: "response api instructions without input - unambiguously Response API",
			body: `{
				"model": "gpt-4",
				"instructions": "Be concise"
			}`,
			expected: ResponseAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.expected, format)
		})
	}
}

// TestDetectFormat_BackwardCompatibility ensures we don't break existing services
// by incorrectly rerouting ambiguous requests.
func TestDetectFormat_BackwardCompatibility(t *testing.T) {
	t.Parallel()
	// This is the exact example from the user - it should return Unknown
	// because it could be valid for either ChatCompletion or Claude Messages
	ambiguousRequest := `{
		"model": "gpt-4o-mini",
		"max_tokens": 2048,
		"messages": [
			{
				"role": "user",
				"content": "Hello, world"
			}
		]
	}`

	format, err := DetectFormat([]byte(ambiguousRequest))
	require.NoError(t, err)
	require.Equal(t, Unknown, format, "ambiguous request should return Unknown to preserve backward compatibility")
}
