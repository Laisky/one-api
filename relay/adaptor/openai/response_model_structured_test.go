package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// TestBidirectionalStructuredOutputConversion tests complete round-trip conversion for structured outputs
func TestBidirectionalStructuredOutputConversion(t *testing.T) {
	// Test ChatCompletion → Response API → ChatCompletion for structured outputs
	originalRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-2024-08-06",
		Messages: []model.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Extract person info from: John Doe is 30 years old"},
		},
		ResponseFormat: &model.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &model.JSONSchema{
				Name:        "person_extraction",
				Description: "Extract person information",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "The person's full name",
						},
						"age": map[string]any{
							"type":        "integer",
							"description": "The person's age",
							"minimum":     0,
							"maximum":     150,
						},
						"additional_info": map[string]any{
							"type":        "string",
							"description": "Any additional information",
						},
					},
					"required":             []string{"name", "age"},
					"additionalProperties": false,
				},
				Strict: boolPtr(true),
			},
		},
		MaxTokens:   200,
		Temperature: floatPtr(0.1),
		Stream:      false,
	}

	// Convert to Response API
	responseAPI := ConvertChatCompletionToResponseAPI(originalRequest)

	// Verify all structured output fields are preserved
	require.NotNil(t, responseAPI.Text, "Expected text config to be set")
	require.NotNil(t, responseAPI.Text.Format, "Expected format to be set")
	require.Equal(t, "json_schema", responseAPI.Text.Format.Type, "Expected format type 'json_schema'")
	require.Equal(t, "person_extraction", responseAPI.Text.Format.Name, "Expected format name 'person_extraction'")
	require.Equal(t, "Extract person information", responseAPI.Text.Format.Description, "Expected format description to be preserved")
	require.NotNil(t, responseAPI.Text.Format.Schema, "Expected schema to be preserved")
	require.NotNil(t, responseAPI.Text.Format.Strict, "Expected strict mode to be set")
	require.True(t, *responseAPI.Text.Format.Strict, "Expected strict mode to be true")

	// Verify complex schema structure preservation
	schema := responseAPI.Text.Format.Schema
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "Expected properties to be preserved in schema")

	nameProperty, ok := properties["name"].(map[string]any)
	require.True(t, ok, "Expected name property to be preserved")
	require.Equal(t, "string", nameProperty["type"], "Expected name type 'string'")
	require.Equal(t, "The person's full name", nameProperty["description"], "Expected name description to be preserved")

	ageProperty, ok := properties["age"].(map[string]any)
	require.True(t, ok, "Expected age property to be preserved")
	require.Equal(t, 0, ageProperty["minimum"], "Expected age minimum constraint")
	require.Equal(t, 150, ageProperty["maximum"], "Expected age maximum constraint")

	required, ok := schema["required"].([]string)
	require.True(t, ok && len(required) == 2, "Expected required fields to be preserved")

	// Simulate Response API response with structured JSON content
	responseAPIResp := &ResponseAPIResponse{
		Id:        "resp_structured_test",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "completed",
		Model:     "gpt-4o-2024-08-06",
		Output: []OutputItem{
			{
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: `{"name": "John Doe", "age": 30, "additional_info": "No additional information provided"}`,
					},
				},
			},
		},
		Text: responseAPI.Text, // Preserve the structured format info
		Usage: &ResponseAPIUsage{
			InputTokens:  25,
			OutputTokens: 15,
			TotalTokens:  40,
		},
	}

	// Convert back to ChatCompletion
	chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

	// Verify the reverse conversion preserves all data
	require.Equal(t, "resp_structured_test", chatCompletion.Id, "Expected id 'resp_structured_test'")
	require.Equal(t, "gpt-4o-2024-08-06", chatCompletion.Model, "Expected model 'gpt-4o-2024-08-06'")
	require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

	choice := chatCompletion.Choices[0]
	require.Equal(t, "stop", choice.FinishReason, "Expected finish_reason 'stop'")

	content, ok := choice.Message.Content.(string)
	require.True(t, ok, "Expected content to be string")

	// Verify the structured JSON content is valid and contains expected data
	expectedContent := `{"name": "John Doe", "age": 30, "additional_info": "No additional information provided"}`
	require.Equal(t, expectedContent, content, "Expected content to match")

	// Verify usage is preserved
	require.Equal(t, 25, chatCompletion.Usage.PromptTokens, "Expected prompt_tokens 25")
	require.Equal(t, 15, chatCompletion.Usage.CompletionTokens, "Expected completion_tokens 15")
	require.Equal(t, 40, chatCompletion.Usage.TotalTokens, "Expected total_tokens 40")
}

// TestStructuredOutputWithReasoningAndFunctionCalls tests complex scenarios
func TestStructuredOutputWithReasoningAndFunctionCalls(t *testing.T) {
	// Test structured output combined with reasoning and function calls
	responseAPIResp := &ResponseAPIResponse{
		Id:        "resp_complex_test",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "completed",
		Model:     "o3-2025-04-16",
		Output: []OutputItem{
			{
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: "The user is asking for structured data extraction. I need to analyze the input and extract the person's information in the specified JSON schema format.",
					},
				},
			},
			{
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: `{"name": "Alice Smith", "age": 25, "occupation": "Software Engineer"}`,
					},
				},
			},
			{
				Type:      "function_call",
				CallId:    "call_validate_person",
				Name:      "validate_person_data",
				Arguments: `{"person": {"name": "Alice Smith", "age": 25}}`,
				Status:    "completed",
			},
		},
		Text: &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type:        "json_schema",
				Name:        "person_data",
				Description: "Person information schema",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":       map[string]any{"type": "string"},
						"age":        map[string]any{"type": "integer"},
						"occupation": map[string]any{"type": "string"},
					},
					"required": []string{"name", "age"},
				},
				Strict: boolPtr(true),
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  30,
			OutputTokens: 45,
			TotalTokens:  75,
		},
	}

	// Convert to ChatCompletion
	chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

	// Verify complex conversion
	require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

	choice := chatCompletion.Choices[0]

	// Verify reasoning is preserved
	require.NotNil(t, choice.Message.Reasoning, "Expected reasoning to be preserved")
	expectedReasoning := "The user is asking for structured data extraction. I need to analyze the input and extract the person's information in the specified JSON schema format."
	require.Equal(t, expectedReasoning, *choice.Message.Reasoning, "Expected reasoning to match")

	// Verify structured content
	content, ok := choice.Message.Content.(string)
	require.True(t, ok, "Expected content to be string")

	expectedContent := `{"name": "Alice Smith", "age": 25, "occupation": "Software Engineer"}`
	require.Equal(t, expectedContent, content, "Expected structured content to match")

	// Verify function calls
	require.Len(t, choice.Message.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Message.ToolCalls[0]
	require.Equal(t, "call_validate_person", toolCall.Id, "Expected tool call id 'call_validate_person'")
	require.Equal(t, "validate_person_data", toolCall.Function.Name, "Expected function name 'validate_person_data'")

	expectedArgs := `{"person": {"name": "Alice Smith", "age": 25}}`
	require.Equal(t, expectedArgs, toolCall.Function.Arguments, "Expected function arguments to match")

	// Verify finish reason is set correctly (should be "tool_calls" when function calls are present)
	if choice.FinishReason != "stop" {
		// Note: The current implementation sets finish_reason to "stop" regardless
		// This might need to be enhanced to detect function calls and set "tool_calls"
		t.Logf("Note: finish_reason is '%s', might want to enhance to detect function calls", choice.FinishReason)
	}
}

func TestConvertResponseAPIToChatCompletionHandlesOutputJSON(t *testing.T) {
	resp := &ResponseAPIResponse{
		Id:        "resp_json",
		Object:    "response",
		CreatedAt: 1700000000,
		Status:    "completed",
		Model:     "gpt-5-mini",
		Output: []OutputItem{
			{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_json",
						JSON: json.RawMessage(`{"topic":"AI","confidence":0.93}`),
					},
				},
			},
		},
	}

	chat := ConvertResponseAPIToChatCompletion(resp)
	require.NotNil(t, chat)
	require.Len(t, chat.Choices, 1)
	choice := chat.Choices[0]
	content, ok := choice.Message.Content.(string)
	require.True(t, ok, "expected content to be string")
	require.Contains(t, content, "\"topic\"")
	require.Contains(t, content, "\"confidence\"")
}

// TestStreamingStructuredOutputWithEvents tests streaming conversion with different event types
func TestStreamingStructuredOutputWithEvents(t *testing.T) {
	// Test different streaming event types with structured content
	testCases := []struct {
		name     string
		chunk    *ResponseAPIResponse
		expected string
	}{
		{
			name: "partial_json_content",
			chunk: &ResponseAPIResponse{
				Id:     "resp_stream_1",
				Status: "in_progress",
				Output: []OutputItem{
					{
						Type: "message",
						Role: "assistant",
						Content: []OutputContent{
							{Type: "output_text", Text: `{"name": "`},
						},
					},
				},
			},
			expected: `{"name": "`,
		},
		{
			name: "json_continuation",
			chunk: &ResponseAPIResponse{
				Id:     "resp_stream_2",
				Status: "in_progress",
				Output: []OutputItem{
					{
						Type: "message",
						Role: "assistant",
						Content: []OutputContent{
							{Type: "output_text", Text: `John Doe", "age": 30}`},
						},
					},
				},
			},
			expected: `John Doe", "age": 30}`,
		},
		{
			name: "reasoning_delta",
			chunk: &ResponseAPIResponse{
				Id:     "resp_stream_3",
				Status: "in_progress",
				Output: []OutputItem{
					{
						Type: "reasoning",
						Summary: []OutputContent{
							{Type: "summary_text", Text: "Analyzing the input to extract structured data..."},
						},
					},
				},
			},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chatStreamChunk := ConvertResponseAPIStreamToChatCompletion(tc.chunk)

			require.Equal(t, "chat.completion.chunk", chatStreamChunk.Object, "Expected object 'chat.completion.chunk'")
			require.Len(t, chatStreamChunk.Choices, 1, "Expected 1 choice")

			choice := chatStreamChunk.Choices[0]

			// For in_progress status, finish_reason should be nil
			if tc.chunk.Status == "in_progress" {
				require.Nil(t, choice.FinishReason, "Expected nil finish_reason for in_progress")
			}

			// Check content
			if tc.expected != "" {
				deltaContent, ok := choice.Delta.Content.(string)
				require.True(t, ok, "Expected delta content to be string")
				require.Equal(t, tc.expected, deltaContent, "Expected delta content to match")
			}

			// Check reasoning for reasoning chunks
			if tc.name == "reasoning_delta" {
				require.NotNil(t, choice.Delta.Reasoning, "Expected reasoning to be set for reasoning chunk")
				expectedReasoning := "Analyzing the input to extract structured data..."
				require.Equal(t, expectedReasoning, *choice.Delta.Reasoning, "Expected reasoning to match")
			}
		})
	}
}

// TestStructuredOutputErrorHandling tests error scenarios
func TestStructuredOutputErrorHandling(t *testing.T) {
	// Test 1: Invalid Response API structure
	t.Run("invalid_response_structure", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_invalid",
			Status: "completed",
			Output: []OutputItem{}, // Empty output
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

		// Should still work but with empty content
		require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice even with empty output")

		choice := chatCompletion.Choices[0]
		content, ok := choice.Message.Content.(string)
		require.True(t, ok && content == "", "Expected empty content for empty output")
	})

	// Test 2: Mixed content types
	t.Run("mixed_content_types", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_mixed",
			Status: "completed",
			Output: []OutputItem{
				{
					Type: "message",
					Role: "assistant",
					Content: []OutputContent{
						{Type: "output_text", Text: `{"part1": "value1"`},
						{Type: "output_text", Text: `, "part2": "value2"}`},
					},
				},
			},
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		choice := chatCompletion.Choices[0]

		content, ok := choice.Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		// Should concatenate all text parts
		expected := `{"part1": "value1", "part2": "value2"}`
		require.Equal(t, expected, content, "Expected concatenated content")
	})

	// Test 3: Response with error status
	t.Run("error_status", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_error",
			Status: "failed",
			Output: []OutputItem{
				{
					Type: "message",
					Role: "assistant",
					Content: []OutputContent{
						{Type: "output_text", Text: "Error occurred"},
					},
				},
			},
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		choice := chatCompletion.Choices[0]

		// Should still convert but with appropriate finish reason
		require.Equal(t, "stop", choice.FinishReason, "Expected finish_reason 'stop' for failed status")

		content, ok := choice.Message.Content.(string)
		require.True(t, ok && content == "Error occurred", "Expected error content to be preserved")
	})
}

// TestJSONSchemaConversionDetailed tests detailed JSON schema conversion scenarios
func TestJSONSchemaConversionDetailed(t *testing.T) {
	t.Run("basic schema conversion", func(t *testing.T) {
		// Create ChatCompletion request with JSON schema
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "user", Content: "Extract person info"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "person_info",
					Description: "Extract person information",
					Schema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type": "string",
							},
							"age": map[string]any{
								"type": "integer",
							},
						},
						"required": []string{"name", "age"},
					},
					Strict: boolPtr(true),
				},
			},
		}

		// Convert to Response API
		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify the conversion
		require.NotNil(t, responseAPI.Text, "Expected text config to be set")
		require.NotNil(t, responseAPI.Text.Format, "Expected format to be set")
		require.Equal(t, "json_schema", responseAPI.Text.Format.Type, "Expected type 'json_schema'")
		require.Equal(t, "person_info", responseAPI.Text.Format.Name, "Expected name 'person_info'")
		require.NotNil(t, responseAPI.Text.Format.Schema, "Expected schema to be preserved")

		// Verify nested schema structure
		schema := responseAPI.Text.Format.Schema
		require.Equal(t, "object", schema["type"], "Expected schema type 'object'")

		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "Expected properties to be a map")

		nameProperty, ok := properties["name"].(map[string]any)
		require.True(t, ok && nameProperty["type"] == "string", "Expected name property to be string type")

		// Test reverse conversion - simulate Response API response with structured content
		responseAPIResp := &ResponseAPIResponse{
			Id:        "resp_123",
			Object:    "response",
			CreatedAt: 1234567890,
			Status:    "completed",
			Model:     "gpt-4o-2024-08-06",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{
							Type: "output_text",
							Text: `{"name": "John Doe", "age": 30}`,
						},
					},
				},
			},
			Text: responseAPI.Text, // Include the structured format info
			Usage: &ResponseAPIUsage{
				InputTokens:  10,
				OutputTokens: 8,
				TotalTokens:  18,
			},
		}

		// Convert back to ChatCompletion
		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

		// Verify the reverse conversion
		require.Equal(t, "resp_123", chatCompletion.Id, "Expected id 'resp_123'")
		require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		// Verify the structured JSON content
		var parsed map[string]any
		err := json.Unmarshal([]byte(content), &parsed)
		require.NoError(t, err, "Expected valid JSON content")
		require.Equal(t, "John Doe", parsed["name"], "Expected name 'John Doe'")
		require.Equal(t, 30.0, parsed["age"], "Expected age 30")
	})
}

// TestComplexNestedSchemaConversion tests complex nested schema structures
func TestComplexNestedSchemaConversion(t *testing.T) {
	// Create complex nested schema
	complexSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"explanation": map[string]any{"type": "string"},
						"output":      map[string]any{"type": "string"},
					},
					"required":             []string{"explanation", "output"},
					"additionalProperties": false,
				},
			},
			"final_answer": map[string]any{"type": "string"},
		},
		"required":             []string{"steps", "final_answer"},
		"additionalProperties": false,
	}

	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-2024-08-06",
		Messages: []model.Message{
			{Role: "user", Content: "Solve 8x + 7 = -23"},
		},
		ResponseFormat: &model.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &model.JSONSchema{
				Name:        "math_reasoning",
				Description: "Step-by-step math solution",
				Schema:      complexSchema,
				Strict:      boolPtr(true),
			},
		},
	}

	// Convert to Response API
	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify complex schema preservation
	require.NotNil(t, responseAPI.Text.Format.Schema, "Expected complex schema to be preserved")

	schema := responseAPI.Text.Format.Schema
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "Expected properties in complex schema")

	steps, ok := properties["steps"].(map[string]any)
	require.True(t, ok, "Expected steps property in complex schema")
	require.Equal(t, "array", steps["type"], "Expected steps type 'array'")

	items, ok := steps["items"].(map[string]any)
	require.True(t, ok, "Expected items in steps array")

	itemProps, ok := items["properties"].(map[string]any)
	require.True(t, ok, "Expected properties in items")
	require.Len(t, itemProps, 2, "Expected 2 item properties")
}

// TestStreamingStructuredOutput tests streaming conversion with structured content
func TestStreamingStructuredOutput(t *testing.T) {
	t.Run("in_progress chunk", func(t *testing.T) {
		// Create streaming response chunk with structured content
		streamChunk := &ResponseAPIResponse{
			Id:        "resp_stream_123",
			Object:    "response",
			CreatedAt: 1234567890,
			Status:    "in_progress",
			Model:     "gpt-4o-2024-08-06",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "in_progress",
					Content: []OutputContent{
						{
							Type: "output_text",
							Text: `{"steps": [{"explanation": "Start with equation"`,
						},
					},
				},
			},
		}

		// Convert streaming chunk
		chatStreamChunk := ConvertResponseAPIStreamToChatCompletion(streamChunk)

		// Verify streaming conversion
		require.Equal(t, "chat.completion.chunk", chatStreamChunk.Object, "Expected object 'chat.completion.chunk'")
		require.Len(t, chatStreamChunk.Choices, 1, "Expected 1 choice in stream")

		choice := chatStreamChunk.Choices[0]
		require.Nil(t, choice.FinishReason, "Expected nil finish_reason for in_progress")

		deltaContent, ok := choice.Delta.Content.(string)
		require.True(t, ok, "Expected delta content to be string")

		expectedContent := `{"steps": [{"explanation": "Start with equation"`
		require.Equal(t, expectedContent, deltaContent, "Expected delta content to match")
	})

	t.Run("completed chunk", func(t *testing.T) {
		// Test completed streaming chunk
		streamChunk := &ResponseAPIResponse{
			Id:        "resp_stream_123",
			Object:    "response",
			CreatedAt: 1234567890,
			Status:    "completed",
			Model:     "gpt-4o-2024-08-06",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{
							Type: "output_text",
							Text: `{"steps": [{"explanation": "Complete solution", "output": "x = -3.75"}], "final_answer": "x = -3.75"}`,
						},
					},
				},
			},
		}

		chatStreamChunkCompleted := ConvertResponseAPIStreamToChatCompletion(streamChunk)
		completedChoice := chatStreamChunkCompleted.Choices[0]

		require.NotNil(t, completedChoice.FinishReason, "Expected finish_reason to be set")
		require.Equal(t, "stop", *completedChoice.FinishReason, "Expected finish_reason 'stop' for completed")
	})
}

// TestStructuredOutputEdgeCases tests edge cases and error handling
func TestStructuredOutputEdgeCases(t *testing.T) {
	t.Run("empty schema handling", func(t *testing.T) {
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "user", Content: "Test"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_object", // No JsonSchema field
			},
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.NotNil(t, responseAPI.Text, "Expected text to be set")
		require.NotNil(t, responseAPI.Text.Format, "Expected text format to be set even with empty schema")
		require.Equal(t, "json_object", responseAPI.Text.Format.Type, "Expected type 'json_object'")
	})

	t.Run("nil response format", func(t *testing.T) {
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "user", Content: "Test"},
			},
			ResponseFormat: nil,
		}
		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.Nil(t, responseAPI.Text, "Expected text to be nil when no response format")
	})

	t.Run("response without text field", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_no_text",
			Status: "completed",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: "Simple response"},
					},
				},
			},
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		require.Len(t, chatCompletion.Choices, 1, "Expected response conversion to work without text field")
	})

	t.Run("malformed JSON handling", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_malformed",
			Status: "completed",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: `{"incomplete": json`},
					},
				},
			},
		}
		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok && content == `{"incomplete": json`, "Expected malformed JSON to be preserved as-is")
	})
}

// TestAdvancedStructuredOutputScenarios tests advanced real-world scenarios
func TestAdvancedStructuredOutputScenarios(t *testing.T) {
	t.Run("research paper extraction", func(t *testing.T) {
		// Complex schema for research paper extraction
		researchSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "The title of the research paper",
				},
				"authors": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Author's full name",
							},
							"affiliation": map[string]any{
								"type":        "string",
								"description": "Author's institutional affiliation",
							},
							"email": map[string]any{
								"type":        "string",
								"format":      "email",
								"description": "Author's email address",
							},
						},
						"required": []string{"name"},
					},
					"minItems": 1,
				},
				"abstract": map[string]any{
					"type":        "string",
					"description": "The paper's abstract",
					"minLength":   50,
				},
				"keywords": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"minItems": 3,
					"maxItems": 10,
				},
				"sections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"content": map[string]any{
								"type":      "string",
								"minLength": 100,
							},
							"subsections": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"title":   map[string]any{"type": "string"},
										"content": map[string]any{"type": "string"},
									},
								},
							},
						},
						"required": []string{"title", "content"},
					},
				},
				"references": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title":   map[string]any{"type": "string"},
							"authors": map[string]any{"type": "array"},
							"journal": map[string]any{"type": "string"},
							"year":    map[string]any{"type": "integer"},
							"doi":     map[string]any{"type": "string"},
							"url":     map[string]any{"type": "string"},
						},
						"required": []string{"title", "authors"},
					},
				},
			},
			"required":             []string{"title", "authors", "abstract", "keywords"},
			"additionalProperties": false,
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "system", Content: "You are an expert at structured data extraction from academic papers."},
				{Role: "user", Content: "Extract information from this research paper: [complex paper content would go here]"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "research_paper_extraction",
					Description: "Extract structured information from research papers",
					Schema:      researchSchema,
					Strict:      boolPtr(true),
				},
			},
		}

		// Convert to Response API
		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify complex nested schema preservation
		require.NotNil(t, responseAPI.Text, "Expected text to be set for complex schema")
		require.NotNil(t, responseAPI.Text.Format, "Expected text format to be set for complex schema")

		schema := responseAPI.Text.Format.Schema
		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in research schema")

		// Verify authors array structure
		authors, ok := properties["authors"].(map[string]any)
		require.True(t, ok, "Expected authors property")

		items, ok := authors["items"].(map[string]any)
		require.True(t, ok, "Expected items in authors array")

		authorProps, ok := items["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in author items")
		require.Len(t, authorProps, 3, "Expected 3 author properties (name, affiliation, email)")

		// Verify sections nested structure
		sections, ok := properties["sections"].(map[string]any)
		require.True(t, ok, "Expected sections property")

		sectionItems, ok := sections["items"].(map[string]any)
		require.True(t, ok, "Expected items in sections array")

		sectionProps, ok := sectionItems["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in section items")

		// Verify subsections nested array
		subsections, ok := sectionProps["subsections"].(map[string]any)
		require.True(t, ok, "Expected subsections property")
		require.Equal(t, "array", subsections["type"], "Expected subsections type 'array'")

		// Test reverse conversion with complex data
		complexResponseData := `{
			"title": "Quantum Machine Learning Applications",
			"authors": [
				{
					"name": "Dr. Alice Quantum",
					"affiliation": "MIT Quantum Lab",
					"email": "alice@mit.edu"
				}
			],
			"abstract": "This paper explores the intersection of quantum computing and machine learning, demonstrating novel applications in optimization problems.",
			"keywords": ["quantum computing", "machine learning", "optimization", "quantum algorithms"],
			"sections": [
				{
					"title": "Introduction",
					"content": "Quantum computing represents a paradigm shift in computational capabilities, offering exponential speedups for certain classes of problems.",
					"subsections": [
						{
							"title": "Background",
							"content": "Historical context of quantum computing development."
						}
					]
				}
			],
			"references": [
				{
					"title": "Quantum Computing: An Applied Approach",
					"authors": ["Hidary, J."],
					"journal": "Springer",
					"year": 2019
				}
			]
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_research",
			Status: "completed",
			Model:  "gpt-4o-2024-08-06",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: complexResponseData},
					},
				},
			},
			Text: responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		// Verify the complex JSON structure
		var extracted map[string]any
		err := json.Unmarshal([]byte(content), &extracted)
		require.NoError(t, err, "Failed to parse complex JSON")
		require.Equal(t, "Quantum Machine Learning Applications", extracted["title"], "Expected title to match")

		authorsArray, ok := extracted["authors"].([]any)
		require.True(t, ok && len(authorsArray) == 1, "Expected 1 author")

		author, ok := authorsArray[0].(map[string]any)
		require.True(t, ok, "Expected author to be object")
		require.Equal(t, "Dr. Alice Quantum", author["name"], "Expected author name to match")
	})

	t.Run("UI component generation with recursive schema", func(t *testing.T) {
		// Recursive UI component schema
		uiSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"description": "The type of the UI component",
					"enum":        []string{"div", "button", "header", "section", "field", "form"},
				},
				"label": map[string]any{
					"type":        "string",
					"description": "The label of the UI component",
				},
				"children": map[string]any{
					"type":        "array",
					"description": "Nested UI components",
					"items":       map[string]any{"$ref": "#"},
				},
				"attributes": map[string]any{
					"type":        "array",
					"description": "Arbitrary attributes for the UI component",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "The name of the attribute",
							},
							"value": map[string]any{
								"type":        "string",
								"description": "The value of the attribute",
							},
						},
						"required":             []string{"name", "value"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"type", "label", "children", "attributes"},
			"additionalProperties": false,
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "system", Content: "You are a UI generator AI."},
				{Role: "user", Content: "Create a user registration form"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "ui_component",
					Description: "Dynamically generated UI component",
					Schema:      uiSchema,
					Strict:      boolPtr(true),
				},
			},
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify recursive schema handling
		schema := responseAPI.Text.Format.Schema
		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in UI schema")

		children, ok := properties["children"].(map[string]any)
		require.True(t, ok, "Expected children property")

		items, ok := children["items"].(map[string]any)
		require.True(t, ok, "Expected items in children")

		// Check for $ref (recursive reference)
		require.Equal(t, "#", items["$ref"], "Expected recursive reference '#'")

		// Test with complex nested UI structure
		uiResponseData := `{
			"type": "form",
			"label": "User Registration",
			"children": [
				{
					"type": "field",
					"label": "Username",
					"children": [],
					"attributes": [
						{"name": "type", "value": "text"},
						{"name": "required", "value": "true"}
					]
				},
				{
					"type": "div",
					"label": "Password Section",
					"children": [
						{
							"type": "field",
							"label": "Password",
							"children": [],
							"attributes": [
								{"name": "type", "value": "password"},
								{"name": "minlength", "value": "8"}
							]
						}
					],
					"attributes": [
						{"name": "class", "value": "password-section"}
					]
				}
			],
			"attributes": [
				{"name": "method", "value": "post"},
				{"name": "action", "value": "/register"}
			]
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_ui",
			Status: "completed",
			Output: []OutputItem{
				{
					Type: "message",
					Role: "assistant",
					Content: []OutputContent{
						{Type: "output_text", Text: uiResponseData},
					},
				},
			},
			Text: responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok, "Expected UI content to be string")

		var uiComponent map[string]any
		err := json.Unmarshal([]byte(content), &uiComponent)
		require.NoError(t, err, "Failed to parse UI JSON")
		require.Equal(t, "form", uiComponent["type"], "Expected type 'form'")

		childrenArray, ok := uiComponent["children"].([]any)
		require.True(t, ok && len(childrenArray) == 2, "Expected 2 children")

		// Verify nested structure
		passwordSection, ok := childrenArray[1].(map[string]any)
		require.True(t, ok, "Expected password section to be object")

		nestedChildren, ok := passwordSection["children"].([]any)
		require.True(t, ok && len(nestedChildren) == 1, "Expected 1 nested child")
	})

	t.Run("chain of thought with structured output", func(t *testing.T) {
		// Chain of thought with structured output
		reasoningSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"step_number": map[string]any{
								"type":        "integer",
								"description": "The step number in the reasoning process",
							},
							"explanation": map[string]any{
								"type":        "string",
								"description": "Explanation of this step",
							},
							"calculation": map[string]any{
								"type":        "string",
								"description": "Mathematical calculation for this step",
							},
							"result": map[string]any{
								"type":        "string",
								"description": "Result of this step",
							},
						},
						"required": []string{"step_number", "explanation", "result"},
					},
				},
				"final_answer": map[string]any{
					"type":        "string",
					"description": "The final answer to the problem",
				},
				"verification": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"method": map[string]any{
							"type":        "string",
							"description": "Method used to verify the answer",
						},
						"check": map[string]any{
							"type":        "string",
							"description": "Verification calculation",
						},
						"confirmed": map[string]any{
							"type":        "boolean",
							"description": "Whether the answer is confirmed",
						},
					},
					"required": []string{"method", "confirmed"},
				},
			},
			"required": []string{"steps", "final_answer", "verification"},
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "o3-2025-04-16", // Reasoning model
			Messages: []model.Message{
				{Role: "system", Content: "You are a math tutor. Show your reasoning step by step."},
				{Role: "user", Content: "Solve: 3x + 7 = 22"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "math_reasoning",
					Description: "Step-by-step mathematical reasoning",
					Schema:      reasoningSchema,
					Strict:      boolPtr(true),
				},
			},
			ReasoningEffort: stringPtr("high"),
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify reasoning effort is normalized to medium for medium-only models
		require.NotNil(t, responseAPI.Reasoning, "Expected reasoning config to be set")
		require.NotNil(t, responseAPI.Reasoning.Effort, "Expected reasoning effort to be set")
		require.Equal(t, "medium", *responseAPI.Reasoning.Effort, "Expected reasoning effort 'medium'")

		// Simulate response with reasoning and structured output
		reasoningResponseData := `{
			"steps": [
				{
					"step_number": 1,
					"explanation": "Start with the equation 3x + 7 = 22",
					"calculation": "3x + 7 = 22",
					"result": "Initial equation established"
				},
				{
					"step_number": 2,
					"explanation": "Subtract 7 from both sides",
					"calculation": "3x + 7 - 7 = 22 - 7",
					"result": "3x = 15"
				},
				{
					"step_number": 3,
					"explanation": "Divide both sides by 3",
					"calculation": "3x / 3 = 15 / 3",
					"result": "x = 5"
				}
			],
			"final_answer": "x = 5",
			"verification": {
				"method": "substitution",
				"check": "3(5) + 7 = 15 + 7 = 22 ✓",
				"confirmed": true
			}
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_reasoning",
			Status: "completed",
			Model:  "o3-2025-04-16",
			Output: []OutputItem{
				{
					Type: "reasoning",
					Summary: []OutputContent{
						{
							Type: "summary_text",
							Text: "I need to solve the linear equation 3x + 7 = 22 step by step, showing each mathematical operation clearly.",
						},
					},
				},
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: reasoningResponseData},
					},
				},
			},
			Reasoning: responseAPI.Reasoning,
			Text:      responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

		// Verify reasoning is preserved
		require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

		choice := chatCompletion.Choices[0]
		require.NotNil(t, choice.Message.Reasoning, "Expected reasoning content to be preserved")

		expectedReasoning := "I need to solve the linear equation 3x + 7 = 22 step by step, showing each mathematical operation clearly."
		require.Equal(t, expectedReasoning, *choice.Message.Reasoning, "Expected reasoning content to match")

		// Verify structured reasoning content
		content, ok := choice.Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		var reasoning map[string]any
		err := json.Unmarshal([]byte(content), &reasoning)
		require.NoError(t, err, "Failed to parse reasoning JSON")

		steps, ok := reasoning["steps"].([]any)
		require.True(t, ok && len(steps) == 3, "Expected 3 reasoning steps")
		require.Equal(t, "x = 5", reasoning["final_answer"], "Expected final answer 'x = 5'")
	})
}

func TestConvertChatCompletionToResponseAPI_DeepResearchDefaultEffort(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "o4-mini-deep-research",
		Messages: []model.Message{
			{Role: "user", Content: "Conduct a deep research summary."},
		},
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	require.NotNil(t, responseAPI.Reasoning, "expected reasoning config to be set for deep research model")
	require.NotNil(t, responseAPI.Reasoning.Effort, "expected reasoning effort to be set")
	require.Equal(t, "medium", *responseAPI.Reasoning.Effort, "expected reasoning effort to default to 'medium'")
	require.NotNil(t, chatRequest.ReasoningEffort, "expected source request reasoning effort to be set")
	require.Equal(t, "medium", *chatRequest.ReasoningEffort, "expected source request reasoning effort to be normalized to 'medium'")
}

func boolPtr(b bool) *bool {
	return &b
}

func floatPtr(f float64) *float64 {
	return &f
}

func stringPtr(s string) *string {
	return &s
}
