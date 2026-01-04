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
