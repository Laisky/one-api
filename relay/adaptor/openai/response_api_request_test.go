package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseAPIRequestParsing tests comprehensive parsing of ResponseAPIRequest
// with all different request body formats outlined in the Response API documentation
func TestResponseAPIRequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		validate    func(t *testing.T, req *ResponseAPIRequest)
	}{
		{
			name: "Simple string input",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "Write a one-sentence bedtime story about a unicorn."
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Equal(t, "gpt-4.1", req.Model, "Expected model 'gpt-4.1'")
				require.Len(t, req.Input, 1, "Expected input length 1")
				require.Equal(t, "Write a one-sentence bedtime story about a unicorn.", req.Input[0], "Unexpected input content")
			},
		},
		{
			name: "String input with instructions",
			jsonData: `{
				"model": "gpt-4.1",
				"instructions": "Talk like a pirate.",
				"input": "Are semicolons optional in JavaScript?"
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.NotNil(t, req.Instructions, "Expected instructions to be set")
				require.Equal(t, "Talk like a pirate.", *req.Instructions, "Expected instructions 'Talk like a pirate.'")
				require.Equal(t, "Are semicolons optional in JavaScript?", req.Input[0], "Unexpected input content")
			},
		},
		{
			name: "Array input with message roles",
			jsonData: `{
				"model": "gpt-4.1",
				"input": [
					{
						"role": "developer",
						"content": "Talk like a pirate."
					},
					{
						"role": "user",
						"content": "Are semicolons optional in JavaScript?"
					}
				]
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Input, 2, "Expected input length 2")
				// Verify first message
				msg1, ok := req.Input[0].(map[string]any)
				require.True(t, ok, "Expected first input to be a message object")
				require.Equal(t, "developer", msg1["role"], "Expected first message role 'developer'")
				require.Equal(t, "Talk like a pirate.", msg1["content"], "Expected first message content 'Talk like a pirate.'")
			},
		},
		{
			name: "Prompt template with variables",
			jsonData: `{
				"model": "gpt-4.1",
				"prompt": {
					"id": "pmpt_abc123",
					"version": "2",
					"variables": {
						"customer_name": "Jane Doe",
						"product": "40oz juice box"
					}
				}
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.NotNil(t, req.Prompt, "Expected prompt to be set")
				require.Equal(t, "pmpt_abc123", req.Prompt.Id, "Expected prompt id 'pmpt_abc123'")
				require.NotNil(t, req.Prompt.Version, "Expected prompt version to be set")
				require.Equal(t, "2", *req.Prompt.Version, "Expected prompt version '2'")
				require.Equal(t, "Jane Doe", req.Prompt.Variables["customer_name"], "Expected customer_name 'Jane Doe'")
			},
		},
		{
			name: "Image input with URL",
			jsonData: `{
				"model": "gpt-4.1-mini",
				"input": [
					{
						"role": "user",
						"content": [
							{"type": "input_text", "text": "what's in this image?"},
							{
								"type": "input_image",
								"image_url": "https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/2560px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg"
							}
						]
					}
				]
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Input, 1, "Expected input length 1")
				msg, ok := req.Input[0].(map[string]any)
				require.True(t, ok, "Expected input to be a message object")
				content, ok := msg["content"].([]any)
				require.True(t, ok, "Expected content to be an array")
				require.Len(t, content, 2, "Expected content length 2")
			},
		},
		{
			name: "Image input with base64",
			jsonData: `{
				"model": "gpt-4.1-mini",
				"input": [
					{
						"role": "user",
						"content": [
							{"type": "input_text", "text": "what's in this image?"},
							{
								"type": "input_image",
								"image_url": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSj/2wBDAQcHBwoIChMKChMoGhYaKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCj/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAv/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIRAxEAPwCdABmX/9k="
							}
						]
					}
				]
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Input, 1, "Expected input length 1")
				msg, ok := req.Input[0].(map[string]any)
				require.True(t, ok, "Expected input to be a message object")
				content, ok := msg["content"].([]any)
				require.True(t, ok, "Expected content to be an array")
				require.Len(t, content, 2, "Expected content length 2")
				// Verify image content
				imageContent, ok := content[1].(map[string]any)
				require.True(t, ok, "Expected second content item to be an object")
				require.Equal(t, "input_image", imageContent["type"], "Expected type 'input_image'")
			},
		},
		{
			name: "Image input with file_id",
			jsonData: `{
				"model": "gpt-4.1-mini",
				"input": [
					{
						"role": "user",
						"content": [
							{"type": "input_text", "text": "what's in this image?"},
							{
								"type": "input_image",
								"file_id": "file-abc123"
							}
						]
					}
				]
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Input, 1, "Expected input length 1")
				msg, ok := req.Input[0].(map[string]any)
				require.True(t, ok, "Expected input to be a message object")
				content, ok := msg["content"].([]any)
				require.True(t, ok, "Expected content to be an array")
				// Verify image content with file_id
				imageContent, ok := content[1].(map[string]any)
				require.True(t, ok, "Expected second content item to be an object")
				require.Equal(t, "file-abc123", imageContent["file_id"], "Expected file_id 'file-abc123'")
			},
		},
		{
			name: "Tools with image generation",
			jsonData: `{
				"model": "gpt-4.1-mini",
				"input": "Generate an image of gray tabby cat hugging an otter with an orange scarf",
				"tools": [{"type": "image_generation"}]
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Tools, 1, "Expected tools length 1")
				require.Equal(t, "image_generation", req.Tools[0].Type, "Expected tool type 'image_generation'")
			},
		},
		{
			name: "Function tools",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "What's the weather like in Boston?",
				"tools": [
					{
						"type": "function",
						"name": "get_weather",
						"description": "Get current weather",
						"parameters": {
							"type": "object",
							"properties": {
								"location": {
									"type": "string",
									"description": "City name"
								}
							},
							"required": ["location"]
						}
					}
				],
				"tool_choice": "auto"
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.Len(t, req.Tools, 1, "Expected tools length 1")
				tool := req.Tools[0]
				require.Equal(t, "function", tool.Type, "Expected tool type 'function'")
				require.Equal(t, "get_weather", tool.Name, "Expected tool name 'get_weather'")
				require.Equal(t, "Get current weather", tool.Description, "Expected tool description 'Get current weather'")
				require.Equal(t, "auto", req.ToolChoice, "Expected tool_choice 'auto'")
			},
		},
		{
			name: "Structured outputs with JSON schema",
			jsonData: `{
				"model": "gpt-4o-2024-08-06",
				"input": [
					{"role": "system", "content": "You are a helpful math tutor. Guide the user through the solution step by step."},
					{"role": "user", "content": "how can I solve 8x + 7 = -23"}
				],
				"text": {
					"format": {
						"type": "json_schema",
						"name": "math_reasoning",
						"schema": {
							"type": "object",
							"properties": {
								"steps": {
									"type": "array",
									"items": {
										"type": "object",
										"properties": {
											"explanation": {"type": "string"},
											"output": {"type": "string"}
										},
										"required": ["explanation", "output"],
										"additionalProperties": false
									}
								},
								"final_answer": {"type": "string"}
							},
							"required": ["steps", "final_answer"],
							"additionalProperties": false
						},
						"strict": true
					}
				}
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.NotNil(t, req.Text, "Expected text config to be set")
				require.NotNil(t, req.Text.Format, "Expected text format to be set")
				require.Equal(t, "json_schema", req.Text.Format.Type, "Expected format type 'json_schema'")
				require.Equal(t, "math_reasoning", req.Text.Format.Name, "Expected format name 'math_reasoning'")
				require.NotNil(t, req.Text.Format.Strict, "Expected strict to be set")
				require.True(t, *req.Text.Format.Strict, "Expected strict mode to be true")
				require.NotNil(t, req.Text.Format.Schema, "Expected schema to be set")
			},
		},
		{
			name: "Reasoning model with effort",
			jsonData: `{
				"model": "o3-2025-04-16",
				"input": "Solve this complex math problem: 3x + 7 = 22",
				"reasoning": {
					"effort": "high",
					"summary": "detailed"
				}
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.NotNil(t, req.Reasoning, "Expected reasoning config to be set")
				require.NotNil(t, req.Reasoning.Effort, "Expected reasoning effort to be set")
				require.Equal(t, "high", *req.Reasoning.Effort, "Expected reasoning effort 'high'")
				require.NotNil(t, req.Reasoning.Summary, "Expected reasoning summary to be set")
				require.Equal(t, "detailed", *req.Reasoning.Summary, "Expected reasoning summary 'detailed'")
			},
		},
		{
			name: "Complete request with all optional parameters",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "Tell me a story",
				"background": true,
				"include": ["usage", "metadata"],
				"instructions": "Be creative and engaging",
				"max_output_tokens": 1000,
				"metadata": {"user_id": "123", "session": "abc"},
				"parallel_tool_calls": true,
				"previous_response_id": "resp_456",
				"service_tier": "default",
				"store": true,
				"stream": false,
				"temperature": 0.7,
				"top_p": 0.9,
				"truncation": "auto",
				"user": "test_user"
			}`,
			expectError: false,
			validate: func(t *testing.T, req *ResponseAPIRequest) {
				require.NotNil(t, req.Background, "Expected background to be set")
				require.True(t, *req.Background, "Expected background to be true")
				require.Equal(t, []string{"usage", "metadata"}, req.Include, "Expected include to be ['usage', 'metadata']")
				require.NotNil(t, req.MaxOutputTokens, "Expected max_output_tokens to be set")
				require.Equal(t, 1000, *req.MaxOutputTokens, "Expected max_output_tokens 1000")
				require.NotNil(t, req.ParallelToolCalls, "Expected parallel_tool_calls to be set")
				require.True(t, *req.ParallelToolCalls, "Expected parallel_tool_calls to be true")
				require.NotNil(t, req.PreviousResponseId, "Expected previous_response_id to be set")
				require.Equal(t, "resp_456", *req.PreviousResponseId, "Expected previous_response_id 'resp_456'")
				require.NotNil(t, req.ServiceTier, "Expected service_tier to be set")
				require.Equal(t, "default", *req.ServiceTier, "Expected service_tier 'default'")
				require.NotNil(t, req.Store, "Expected store to be set")
				require.True(t, *req.Store, "Expected store to be true")
				require.NotNil(t, req.Stream, "Expected stream to be set")
				require.False(t, *req.Stream, "Expected stream to be false")
				require.NotNil(t, req.Temperature, "Expected temperature to be set")
				require.Equal(t, 0.7, *req.Temperature, "Expected temperature 0.7")
				require.NotNil(t, req.TopP, "Expected top_p to be set")
				require.Equal(t, 0.9, *req.TopP, "Expected top_p 0.9")
				require.NotNil(t, req.Truncation, "Expected truncation to be set")
				require.Equal(t, "auto", *req.Truncation, "Expected truncation 'auto'")
				require.NotNil(t, req.User, "Expected user to be set")
				require.Equal(t, "test_user", *req.User, "Expected user 'test_user'")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request ResponseAPIRequest
			err := json.Unmarshal([]byte(tt.jsonData), &request)

			if tt.expectError {
				require.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
			}

			if err == nil && tt.validate != nil {
				tt.validate(t, &request)
			}
		})
	}
}

// TestResponseAPIRequestEdgeCases tests edge cases and error handling
func TestResponseAPIRequestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Missing required model field",
			jsonData: `{
				"input": "Hello world"
			}`,
			expectError: false, // JSON unmarshaling doesn't enforce required fields
		},
		{
			name: "Empty input and no prompt",
			jsonData: `{
				"model": "gpt-4.1"
			}`,
			expectError: false, // Valid - input and prompt are optional
		},
		{
			name: "Both input and prompt provided",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "Hello",
				"prompt": {
					"id": "pmpt_123"
				}
			}`,
			expectError: false, // JSON parsing allows both, validation would be at business logic level
		},
		{
			name: "Invalid JSON",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "Hello"
				"missing_comma": true
			}`,
			expectError: true,
		},
		{
			name: "Null values for optional fields",
			jsonData: `{
				"model": "gpt-4.1",
				"input": "Hello",
				"temperature": null,
				"max_output_tokens": null,
				"stream": null
			}`,
			expectError: false,
		},
		{
			name: "Empty arrays and objects",
			jsonData: `{
				"model": "gpt-4.1",
				"input": [],
				"tools": [],
				"include": [],
				"metadata": {}
			}`,
			expectError: false,
		},
		{
			name: "Complex nested structures",
			jsonData: `{
				"model": "gpt-4.1",
				"input": [
					{
						"role": "user",
						"content": [
							{
								"type": "input_text",
								"text": "Analyze this complex data"
							},
							{
								"type": "input_image",
								"image_url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
								"detail": "high"
							}
						]
					}
				],
				"tools": [
					{
						"type": "function",
						"name": "complex_analysis",
						"description": "Perform complex data analysis",
						"parameters": {
							"type": "object",
							"properties": {
								"data": {
									"type": "array",
									"items": {
										"type": "object",
										"properties": {
											"id": {"type": "string"},
											"value": {"type": "number"},
											"metadata": {
												"type": "object",
												"additionalProperties": true
											}
										}
									}
								}
							}
						}
					}
				],
				"text": {
					"format": {
						"type": "json_schema",
						"name": "analysis_result",
						"schema": {
							"type": "object",
							"properties": {
								"summary": {"type": "string"},
								"details": {
									"type": "array",
									"items": {"type": "string"}
								},
								"confidence": {
									"type": "number",
									"minimum": 0,
									"maximum": 1
								}
							}
						}
					}
				}
			}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request ResponseAPIRequest
			err := json.Unmarshal([]byte(tt.jsonData), &request)

			if tt.expectError {
				require.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
			}

			// Basic validation that the struct was populated correctly for successful cases
			if err == nil {
				// Verify that the request can be marshaled back to JSON
				_, marshalErr := json.Marshal(request)
				require.NoError(t, marshalErr, "Failed to marshal request back to JSON")
			}
		})
	}
}

// TestResponseAPIInputMarshalUnmarshal tests bidirectional conversion of ResponseAPIInput
func TestResponseAPIInputMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input ResponseAPIInput
	}{
		{
			name:  "Single string",
			input: ResponseAPIInput{"Hello world"},
		},
		{
			name:  "Multiple strings",
			input: ResponseAPIInput{"Hello", "world"},
		},
		{
			name: "Mixed content",
			input: ResponseAPIInput{
				"Hello",
				map[string]any{
					"role":    "user",
					"content": "world",
				},
			},
		},
		{
			name: "Complex message structure",
			input: ResponseAPIInput{
				map[string]any{
					"role": "user",
					"content": []any{
						map[string]any{
							"type": "input_text",
							"text": "What's in this image?",
						},
						map[string]any{
							"type":      "input_image",
							"image_url": "https://example.com/image.jpg",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err, "Failed to marshal input")

			// Unmarshal back
			var result ResponseAPIInput
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err, "Failed to unmarshal input")

			// Verify length matches
			require.Len(t, result, len(tt.input), "Expected length to match")

			// For single string case, verify it marshals as string not array
			if len(tt.input) == 1 {
				if str, ok := tt.input[0].(string); ok {
					var directString string
					err = json.Unmarshal(jsonData, &directString)
					require.NoError(t, err, "Single string should marshal as string, not array")
					require.Equal(t, str, directString, "Expected string to match")
				}
			}
		})
	}
}
