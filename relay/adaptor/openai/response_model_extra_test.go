package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// TestConvertResponseAPIStreamToChatCompletion tests the conversion from Response API streaming format to ChatCompletion streaming format
func TestConvertResponseAPIStreamToChatCompletion(t *testing.T) {
	// Create a Response API streaming chunk
	responseAPIChunk := &ResponseAPIResponse{
		Id:        "resp_123",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "in_progress",
		Model:     "gpt-4",
		Output: []OutputItem{
			{
				Type:   "message",
				Id:     "msg_123",
				Status: "in_progress",
				Role:   "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: "Hello",
					},
				},
			},
		},
	}

	// Convert to ChatCompletion streaming format
	streamChunk := ConvertResponseAPIStreamToChatCompletion(responseAPIChunk)

	// Verify basic fields
	require.Equal(t, "resp_123", streamChunk.Id)
	require.Equal(t, "chat.completion.chunk", streamChunk.Object)
	require.Equal(t, "gpt-4", streamChunk.Model)
	require.Equal(t, int64(1234567890), streamChunk.Created)

	// Verify choices
	require.Len(t, streamChunk.Choices, 1, "Expected 1 choice")

	choice := streamChunk.Choices[0]
	require.Equal(t, 0, choice.Index, "Expected choice index 0")
	require.Equal(t, "assistant", choice.Delta.Role)
	require.Equal(t, "Hello", choice.Delta.Content)

	// For in_progress status, finish_reason should be nil
	require.Nil(t, choice.FinishReason, "Expected finish_reason to be nil for in_progress status")

	// Test completed status
	responseAPIChunk.Status = "completed"
	streamChunk = ConvertResponseAPIStreamToChatCompletion(responseAPIChunk)
	choice = streamChunk.Choices[0]

	require.NotNil(t, choice.FinishReason, "Expected finish_reason to be set for completed status")
	require.Equal(t, "stop", *choice.FinishReason, "Expected finish_reason 'stop' for completed status")
}

// TestConvertResponseAPIStreamToChatCompletionWithFunctionCall tests streaming conversion with function calls
func TestConvertResponseAPIStreamToChatCompletionWithFunctionCall(t *testing.T) {
	// Create a Response API streaming chunk with function call
	responseAPIChunk := &ResponseAPIResponse{
		Id:        "resp_123",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "completed",
		Model:     "gpt-4",
		Output: []OutputItem{
			{
				Type:      "function_call",
				Id:        "fc_123",
				CallId:    "call_456",
				Name:      "get_weather",
				Arguments: "{\"location\":\"Boston\"}",
				Status:    "completed",
			},
		},
	}

	// Convert to ChatCompletion streaming format
	streamChunk := ConvertResponseAPIStreamToChatCompletion(responseAPIChunk)

	// Verify basic fields
	require.Equal(t, "resp_123", streamChunk.Id)
	require.Equal(t, "chat.completion.chunk", streamChunk.Object)

	// Verify choices
	require.Len(t, streamChunk.Choices, 1, "Expected 1 choice")

	choice := streamChunk.Choices[0]
	require.Equal(t, 0, choice.Index, "Expected choice index 0")
	require.Equal(t, "assistant", choice.Delta.Role)

	// Verify tool calls
	require.Len(t, choice.Delta.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Delta.ToolCalls[0]
	require.Equal(t, "call_456", toolCall.Id)
	require.Equal(t, "get_weather", toolCall.Function.Name)
	require.Equal(t, "{\"location\":\"Boston\"}", toolCall.Function.Arguments)

	// For completed status, finish_reason should be "stop"
	require.NotNil(t, choice.FinishReason, "Expected finish_reason to be set for completed status")
	require.Equal(t, "stop", *choice.FinishReason)
}

// TestConvertResponseAPIToChatCompletionWithReasoning tests the conversion with reasoning content
func TestConvertResponseAPIToChatCompletionWithReasoning(t *testing.T) {
	// Create a Response API response with reasoning content (based on the real example)
	responseAPI := &ResponseAPIResponse{
		Id:        "resp_6848f7a7ac94819cba6af50194a156e7050d57f0136932b5",
		Object:    "response",
		CreatedAt: 1749612455,
		Status:    "completed",
		Model:     "o3-2025-04-16",
		Output: []OutputItem{
			{
				Id:   "rs_6848f7a7f800819ca52a87ae9a6a59ef050d57f0136932b5",
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: "**Telling a joke**\n\nThe user asked for a joke, which is a straightforward request. There's no conflict with the guidelines, so I can definitely comply.",
					},
				},
			},
			{
				Id:     "msg_6848f7abc86c819c877542f4a72a3f1d050d57f0136932b5",
				Type:   "message",
				Status: "completed",
				Role:   "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: "Why don't scientists trust atoms?\n\nBecause they make up everything!",
					},
				},
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  9,
			OutputTokens: 83,
			TotalTokens:  92,
		},
	}

	// Convert to ChatCompletion format
	chatCompletion := ConvertResponseAPIToChatCompletion(responseAPI)

	// Verify basic fields
	require.Equal(t, "resp_6848f7a7ac94819cba6af50194a156e7050d57f0136932b5", chatCompletion.Id)
	require.Equal(t, "o3-2025-04-16", chatCompletion.Model)

	// Verify choices
	require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

	choice := chatCompletion.Choices[0]
	require.Equal(t, "assistant", choice.Message.Role)

	expectedContent := "Why don't scientists trust atoms?\n\nBecause they make up everything!"
	require.Equal(t, expectedContent, choice.Message.Content)

	// Verify reasoning content is properly extracted
	require.NotNil(t, choice.Message.Reasoning, "Expected reasoning content to be present")

	expectedReasoning := "**Telling a joke**\n\nThe user asked for a joke, which is a straightforward request. There's no conflict with the guidelines, so I can definitely comply."
	require.Equal(t, expectedReasoning, *choice.Message.Reasoning)
	require.Equal(t, "stop", choice.FinishReason)

	// Verify usage
	require.Equal(t, 9, chatCompletion.Usage.PromptTokens)
	require.Equal(t, 83, chatCompletion.Usage.CompletionTokens)
	require.Equal(t, 92, chatCompletion.Usage.TotalTokens)
}

// TestFunctionCallWorkflow tests the complete function calling workflow:
// ChatCompletion -> ResponseAPI -> ResponseAPI Response -> ChatCompletion
func TestFunctionCallWorkflow(t *testing.T) {
	// Step 1: Create original ChatCompletion request with tools
	originalRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "What's the weather like in Boston today?"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_current_weather",
					Description: "Get the current weather in a given location",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The city and state, e.g. San Francisco, CA",
							},
							"unit": map[string]any{
								"type": "string",
								"enum": []string{"celsius", "fahrenheit"},
							},
						},
						"required": []string{"location", "unit"},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	// Step 2: Convert ChatCompletion to Response API format
	responseAPIRequest := ConvertChatCompletionToResponseAPI(originalRequest)

	// Verify tools are preserved in request
	require.Len(t, responseAPIRequest.Tools, 1, "Expected 1 tool in request")
	require.NotNil(t, responseAPIRequest.Tools[0].Function, "Expected function definition on response tool")
	require.Equal(t, "get_current_weather", responseAPIRequest.Tools[0].Function.Name)
	require.Equal(t, "auto", responseAPIRequest.ToolChoice)

	// Step 3: Create a Response API response with function call (simulates upstream response)
	responseAPIResponse := &ResponseAPIResponse{
		Id:        "resp_67ca09c5efe0819096d0511c92b8c890096610f474011cc0",
		Object:    "response",
		CreatedAt: 1741294021,
		Status:    "completed",
		Model:     "gpt-4.1-2025-04-14",
		Output: []OutputItem{
			{
				Type:      "function_call",
				Id:        "fc_67ca09c6bedc8190a7abfec07b1a1332096610f474011cc0",
				CallId:    "call_unLAR8MvFNptuiZK6K6HCy5k",
				Name:      "get_current_weather",
				Arguments: "{\"location\":\"Boston, MA\",\"unit\":\"celsius\"}",
				Status:    "completed",
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  291,
			OutputTokens: 23,
			TotalTokens:  314,
		},
	}

	// Step 4: Convert Response API response back to ChatCompletion format
	finalChatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResponse)

	// Step 5: Verify the final ChatCompletion response preserves all function call information
	require.Len(t, finalChatCompletion.Choices, 1, "Expected 1 choice")

	choice := finalChatCompletion.Choices[0]
	require.Equal(t, "assistant", choice.Message.Role)

	// Verify tool calls are preserved
	require.Len(t, choice.Message.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Message.ToolCalls[0]
	require.Equal(t, "call_unLAR8MvFNptuiZK6K6HCy5k", toolCall.Id)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, "get_current_weather", toolCall.Function.Name)

	expectedArgs := "{\"location\":\"Boston, MA\",\"unit\":\"celsius\"}"
	require.Equal(t, expectedArgs, toolCall.Function.Arguments)

	// Verify usage is preserved
	require.Equal(t, 291, finalChatCompletion.Usage.PromptTokens)
	require.Equal(t, 23, finalChatCompletion.Usage.CompletionTokens)
	require.Equal(t, 314, finalChatCompletion.Usage.TotalTokens)

	t.Log("Function call workflow test completed successfully!")
	t.Logf("Original request tools: %d", len(originalRequest.Tools))
	t.Logf("Response API request tools: %d", len(responseAPIRequest.Tools))
	t.Logf("Final response tool calls: %d", len(choice.Message.ToolCalls))
}

func TestConvertWithLegacyFunctions(t *testing.T) {
	// Test legacy functions conversion
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "What's the weather?"},
		},
		Functions: []model.Function{
			{
				Name:        "get_current_weather",
				Description: "Get current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The city and state, e.g. San Francisco, CA",
						},
						"unit": map[string]any{
							"type": "string",
							"enum": []string{"celsius", "fahrenheit"},
						},
					},
					"required": []string{"location"},
				},
			},
		},
		FunctionCall: "auto",
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify functions are converted to tools
	require.Len(t, responseAPI.Tools, 1, "Expected 1 tool")
	require.Equal(t, "function", responseAPI.Tools[0].Type)
	require.NotNil(t, responseAPI.Tools[0].Function, "Expected response tool to include function definition")
	require.Equal(t, "get_current_weather", responseAPI.Tools[0].Function.Name)
	require.Equal(t, "auto", responseAPI.ToolChoice)

	// Verify the function parameters are preserved
	require.NotNil(t, responseAPI.Tools[0].Parameters, "Expected function parameters to be preserved")

	// Verify properties are preserved
	props, ok := responseAPI.Tools[0].Parameters["properties"].(map[string]any)
	require.True(t, ok, "Expected properties to be preserved")
	location, ok := props["location"].(map[string]any)
	require.True(t, ok, "Expected location property to be preserved")
	require.Equal(t, "string", location["type"], "Expected location type 'string'")
}

// TestLegacyFunctionCallWorkflow tests the complete legacy function calling workflow:
// ChatCompletion with Functions -> ResponseAPI -> ResponseAPI Response -> ChatCompletion
func TestLegacyFunctionCallWorkflow(t *testing.T) {
	// Step 1: Create original ChatCompletion request with legacy functions
	originalRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "What's the weather like in Boston today?"},
		},
		Functions: []model.Function{
			{
				Name:        "get_current_weather",
				Description: "Get the current weather in a given location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The city and state, e.g. San Francisco, CA",
						},
						"unit": map[string]any{
							"type": "string",
							"enum": []string{"celsius", "fahrenheit"},
						},
					},
					"required": []string{"location", "unit"},
				},
			},
		},
		FunctionCall: "auto",
	}

	// Step 2: Convert ChatCompletion to Response API format
	responseAPIRequest := ConvertChatCompletionToResponseAPI(originalRequest)

	// Verify functions are converted to tools in request
	require.Len(t, responseAPIRequest.Tools, 1, "Expected 1 tool in request")
	require.NotNil(t, responseAPIRequest.Tools[0].Function, "Expected function definition on response tool")
	require.Equal(t, "get_current_weather", responseAPIRequest.Tools[0].Function.Name)
	require.Equal(t, "auto", responseAPIRequest.ToolChoice)

	// Step 3: Create mock Response API response (simulating what the API would return)
	responseAPIResponse := &ResponseAPIResponse{
		Id:        "resp_legacy_test",
		Object:    "response",
		CreatedAt: 1741294021,
		Status:    "completed",
		Model:     "gpt-4.1-2025-04-14",
		Output: []OutputItem{
			{
				Type:      "function_call",
				Id:        "fc_legacy_test",
				CallId:    "call_legacy_test_123",
				Name:      "get_current_weather",
				Arguments: "{\"location\":\"Boston, MA\",\"unit\":\"celsius\"}",
				Status:    "completed",
			},
		},
		ParallelToolCalls: true,
		ToolChoice:        "auto",
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_current_weather",
					Description: "Get the current weather in a given location",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The city and state, e.g. San Francisco, CA",
							},
							"unit": map[string]any{
								"type": "string",
								"enum": []string{"celsius", "fahrenheit"},
							},
						},
						"required": []string{"location", "unit"},
					},
				},
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  291,
			OutputTokens: 23,
			TotalTokens:  314,
		},
	}

	// Step 4: Convert Response API response back to ChatCompletion format
	finalChatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResponse)

	// Step 5: Verify the final ChatCompletion response preserves all function call information
	require.Len(t, finalChatCompletion.Choices, 1, "Expected 1 choice")

	choice := finalChatCompletion.Choices[0]
	require.Equal(t, "assistant", choice.Message.Role)

	// Verify tool calls are preserved
	require.Len(t, choice.Message.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Message.ToolCalls[0]
	require.Equal(t, "call_legacy_test_123", toolCall.Id)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, "get_current_weather", toolCall.Function.Name)

	expectedArgs := "{\"location\":\"Boston, MA\",\"unit\":\"celsius\"}"
	require.Equal(t, expectedArgs, toolCall.Function.Arguments)

	// Verify usage is preserved
	require.Equal(t, 291, finalChatCompletion.Usage.PromptTokens)
	require.Equal(t, 23, finalChatCompletion.Usage.CompletionTokens)
	require.Equal(t, 314, finalChatCompletion.Usage.TotalTokens)

	t.Log("Legacy function call workflow test completed successfully!")
	t.Logf("Original request functions: %d", len(originalRequest.Functions))
	t.Logf("Response API request tools: %d", len(responseAPIRequest.Tools))
	t.Logf("Final response tool calls: %d", len(choice.Message.ToolCalls))
}

// TestParseResponseAPIStreamEvent tests the flexible parsing of Response API streaming events
func TestParseResponseAPIStreamEvent(t *testing.T) {
	t.Run("Parse response.output_text.done event", func(t *testing.T) {
		// This is the problematic event that was causing parsing failures
		eventData := `{"type":"response.output_text.done","sequence_number":22,"item_id":"msg_6849865110908191a4809c86e082ff710008bd3c6060334b","output_index":1,"content_index":0,"text":"Why don't skeletons fight each other?\n\nThey don't have the guts."}`

		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.NoError(t, err, "Failed to parse streaming event")

		// Should parse as streaming event, not full response
		require.Nil(t, fullResponse, "Expected fullResponse to be nil for streaming event")
		require.NotNil(t, streamEvent, "Expected streamEvent to be non-nil")

		// Verify event fields
		require.Equal(t, "response.output_text.done", streamEvent.Type)
		require.Equal(t, 22, streamEvent.SequenceNumber)
		require.Equal(t, "msg_6849865110908191a4809c86e082ff710008bd3c6060334b", streamEvent.ItemId)

		expectedText := "Why don't skeletons fight each other?\n\nThey don't have the guts."
		require.Equal(t, expectedText, streamEvent.Text)
	})

	t.Run("Parse response.output_text.delta event", func(t *testing.T) {
		eventData := `{"type":"response.output_text.delta","sequence_number":6,"item_id":"msg_6849865110908191a4809c86e082ff710008bd3c6060334b","output_index":1,"content_index":0,"delta":"Why"}`

		_, streamEvent, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.NoError(t, err, "Failed to parse delta event")
		require.NotNil(t, streamEvent, "Expected streamEvent to be non-nil")

		// Verify event fields
		require.Equal(t, "response.output_text.delta", streamEvent.Type)

		delta := extractStringFromRaw(streamEvent.Delta, "text", "delta")
		require.Equal(t, "Why", delta)
	})

	t.Run("Parse full response event", func(t *testing.T) {
		eventData := `{"id":"resp_123","object":"response","created_at":1749648976,"status":"completed","model":"o3-2025-04-16","output":[{"type":"message","id":"msg_123","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}],"usage":{"input_tokens":9,"output_tokens":22,"total_tokens":31}}`

		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.NoError(t, err, "Failed to parse full response event")

		// Should parse as full response, not streaming event
		require.Nil(t, streamEvent, "Expected streamEvent to be nil for full response")
		require.NotNil(t, fullResponse, "Expected fullResponse to be non-nil")

		// Verify response fields
		require.Equal(t, "resp_123", fullResponse.Id)
		require.Equal(t, "completed", fullResponse.Status)
		require.NotNil(t, fullResponse.Usage)
		require.Equal(t, 31, fullResponse.Usage.TotalTokens)
	})

	t.Run("Parse invalid JSON", func(t *testing.T) {
		eventData := `{"invalid": json}`

		_, _, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.Error(t, err, "Expected error for invalid JSON")
	})
}

// TestConvertStreamEventToResponse tests the conversion of streaming events to ResponseAPIResponse format
func TestConvertStreamEventToResponse(t *testing.T) {
	t.Run("Convert response.output_text.done event", func(t *testing.T) {
		streamEvent := &ResponseAPIStreamEvent{
			Type:           "response.output_text.done",
			SequenceNumber: 22,
			ItemId:         "msg_123",
			OutputIndex:    1,
			ContentIndex:   0,
			Text:           "Hello, world!",
		}

		response := ConvertStreamEventToResponse(streamEvent)

		// Verify basic fields
		require.Equal(t, "response", response.Object)
		require.Equal(t, "in_progress", response.Status)

		// Verify output
		require.Len(t, response.Output, 1, "Expected 1 output item")

		output := response.Output[0]
		require.Equal(t, "message", output.Type)
		require.Equal(t, "assistant", output.Role)
		require.Len(t, output.Content, 1, "Expected 1 content item")

		content := output.Content[0]
		require.Equal(t, "output_text", content.Type)
		require.Equal(t, "Hello, world!", content.Text)
	})

	t.Run("Convert response.output_text.delta event", func(t *testing.T) {
		streamEvent := &ResponseAPIStreamEvent{
			Type:           "response.output_text.delta",
			SequenceNumber: 6,
			ItemId:         "msg_123",
			OutputIndex:    1,
			ContentIndex:   0,
			Delta:          json.RawMessage(`"Hello"`),
		}

		response := ConvertStreamEventToResponse(streamEvent)

		// Verify basic fields
		require.Equal(t, "response", response.Object)
		require.Equal(t, "in_progress", response.Status)

		// Verify output
		require.Len(t, response.Output, 1, "Expected 1 output item")

		output := response.Output[0]
		require.Equal(t, "message", output.Type)
		require.Equal(t, "assistant", output.Role)
		require.Len(t, output.Content, 1, "Expected 1 content item")

		content := output.Content[0]
		require.Equal(t, "output_text", content.Type)
		require.Equal(t, "Hello", content.Text)
	})

	t.Run("Convert unknown event type", func(t *testing.T) {
		streamEvent := &ResponseAPIStreamEvent{
			Type:           "response.unknown.event",
			SequenceNumber: 1,
			ItemId:         "msg_123",
		}

		response := ConvertStreamEventToResponse(streamEvent)

		// Should still create a basic response structure
		require.Equal(t, "response", response.Object)
		require.Equal(t, "in_progress", response.Status)

		// Output should be empty for unknown event types
		require.Empty(t, response.Output, "Expected 0 output items for unknown event")
	})
}

// TestStreamEventIntegration tests the complete integration of streaming event parsing with ChatCompletion conversion
func TestStreamEventIntegration(t *testing.T) {
	t.Run("End-to-end streaming event processing", func(t *testing.T) {
		// Test the problematic event that was causing the original bug
		eventData := `{"type":"response.output_text.done","sequence_number":22,"item_id":"msg_6849865110908191a4809c86e082ff710008bd3c6060334b","output_index":1,"content_index":0,"text":"Why don't skeletons fight each other?\n\nThey don't have the guts."}`

		// Step 1: Parse the streaming event
		_, streamEvent, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.NoError(t, err, "Failed to parse streaming event")
		require.NotNil(t, streamEvent, "Expected streamEvent to be non-nil")

		// Step 2: Convert to ResponseAPIResponse format
		responseAPIChunk := ConvertStreamEventToResponse(streamEvent)

		// Step 3: Convert to ChatCompletion streaming format
		chatCompletionChunk := ConvertResponseAPIStreamToChatCompletion(&responseAPIChunk)

		// Verify the final result
		require.Len(t, chatCompletionChunk.Choices, 1, "Expected 1 choice")

		choice := chatCompletionChunk.Choices[0]
		expectedContent := "Why don't skeletons fight each other?\n\nThey don't have the guts."
		content, ok := choice.Delta.Content.(string)
		require.True(t, ok, "Expected delta content to be string")
		require.Equal(t, expectedContent, content)
	})

	t.Run("Delta event processing", func(t *testing.T) {
		eventData := `{"type":"response.output_text.delta","sequence_number":6,"item_id":"msg_6849865110908191a4809c86e082ff710008bd3c6060334b","output_index":1,"content_index":0,"delta":"Why"}`

		// Step 1: Parse the streaming event
		_, streamEvent, err := ParseResponseAPIStreamEvent([]byte(eventData))
		require.NoError(t, err, "Failed to parse delta event")
		require.NotNil(t, streamEvent, "Expected streamEvent to be non-nil")

		// Step 2: Convert to ResponseAPIResponse format
		responseAPIChunk := ConvertStreamEventToResponse(streamEvent)

		// Step 3: Convert to ChatCompletion streaming format
		chatCompletionChunk := ConvertResponseAPIStreamToChatCompletion(&responseAPIChunk)

		// Verify the final result
		require.Len(t, chatCompletionChunk.Choices, 1, "Expected 1 choice")

		choice := chatCompletionChunk.Choices[0]
		content, ok := choice.Delta.Content.(string)
		require.True(t, ok, "Expected delta content to be string")
		require.Equal(t, "Why", content)
	})
}

// TestConvertChatCompletionToResponseAPIWithToolResults tests that tool result messages
// are properly converted to function_call_output format for Response API
func TestContentTypeBasedOnRole(t *testing.T) {
	// Test that user messages use "input_text" and assistant messages use "output_text"
	userMessage := model.Message{
		Role:    "user",
		Content: "Hello, how are you?",
	}

	assistantMessage := model.Message{
		Role:    "assistant",
		Content: "I'm doing well, thank you!",
	}

	// Convert user message
	userResult := convertMessageToResponseAPIFormat(userMessage)
	userContent := userResult["content"].([]map[string]any)
	require.Equal(t, "input_text", userContent[0]["type"], "Expected user message to use 'input_text' type")
	require.Equal(t, "Hello, how are you?", userContent[0]["text"], "Expected user message text to be preserved")

	// Convert assistant message
	assistantResult := convertMessageToResponseAPIFormat(assistantMessage)
	assistantContent := assistantResult["content"].([]map[string]any)
	require.Equal(t, "output_text", assistantContent[0]["type"], "Expected assistant message to use 'output_text' type")
	require.Equal(t, "I'm doing well, thank you!", assistantContent[0]["text"], "Expected assistant message text to be preserved")
}

func TestConversationWithMultipleRoles(t *testing.T) {
	// Test a conversation similar to the error log scenario
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []model.Message{
			{Role: "system", Content: "you are an experienced english translator"},
			{Role: "user", Content: "我认为后端为 invalid 返回 200 是很荒谬的"},
			{Role: "assistant", Content: "I think it's absurd for the backend to return 200 for an invalid response"},
			{Role: "user", Content: "用户发送的 openai 请求，应该被转换为 ResponseAPI"},
			{Role: "assistant", Content: "The OpenAI request sent by the user should be converted into a ResponseAPI"},
			{Role: "user", Content: "halo"},
		},
		MaxTokens:   5000,
		Temperature: floatPtr(1.0),
		Stream:      true,
	}

	// Convert to Response API format
	responseAPIRequest := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify the conversion
	require.Equal(t, "gpt-4o-mini", responseAPIRequest.Model)

	// Check that input array has correct content types
	inputArray := []any(responseAPIRequest.Input)
	for i, item := range inputArray {
		if itemMap, ok := item.(map[string]any); ok {
			role := itemMap["role"].(string)
			content := itemMap["content"].([]map[string]any)

			expectedType := "input_text"
			if role == "assistant" {
				expectedType = "output_text"
			}

			require.Equal(t, expectedType, content[0]["type"],
				"Message %d with role '%s' should use '%s' type", i, role, expectedType)
		}
	}
}

func TestConvertChatCompletionToResponseAPIWithToolResults(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []model.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "What's the current time?"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []model.Tool{
					{
						Id:   "initial_datetime_call",
						Type: "function",
						Function: &model.Function{
							Name:      "get_current_datetime",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallId: "initial_datetime_call",
				Content:    `{"year":2025,"month":6,"day":12,"hour":11,"minute":43,"second":7}`,
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_current_datetime",
					Description: "Get current date and time",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
		},
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify system message was moved to instructions
	require.NotNil(t, responseAPI.Instructions)
	require.Equal(t, "You are a helpful assistant.", *responseAPI.Instructions)

	// Verify input array structure preserves tool call history
	require.Len(t, responseAPI.Input, 3)

	// Verify first message (user)
	msg0, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok, "expected first input to be a map")
	assert.Equal(t, "user", msg0["role"])
	if content, ok := msg0["content"].([]map[string]any); ok && len(content) > 0 {
		assert.Equal(t, "input_text", content[0]["type"])
		assert.Equal(t, "What's the current time?", content[0]["text"])
	}

	// Verify function call item
	msg1, ok := responseAPI.Input[1].(map[string]any)
	require.True(t, ok, "expected function call item to be a map")
	assert.Equal(t, "function_call", msg1["type"])
	if role, exists := msg1["role"]; exists {
		assert.Equal(t, "assistant", role)
	}
	assert.Equal(t, "get_current_datetime", msg1["name"])
	assert.Equal(t, `{}`, msg1["arguments"])
	assert.Equal(t, "fc_initial_datetime_call", msg1["id"])
	assert.Equal(t, "call_initial_datetime_call", msg1["call_id"])

	// Verify tool output item
	msg2, ok := responseAPI.Input[2].(map[string]any)
	require.True(t, ok, "expected function call output item to be a map")
	assert.Equal(t, "function_call_output", msg2["type"])
	if role, exists := msg2["role"]; exists {
		assert.Equal(t, "tool", role)
	}
	assert.Equal(t, "call_initial_datetime_call", msg2["call_id"])
	assert.NotContains(t, msg2, "name")
	if output, ok := msg2["output"].(string); ok {
		assert.Contains(t, output, "\"year\":2025")
	}

	// Verify tools were converted properly
	require.Len(t, responseAPI.Tools, 1, "Expected 1 tool")

	tool := responseAPI.Tools[0]
	require.Equal(t, "get_current_datetime", tool.Name)
	require.Equal(t, "function", tool.Type)
}
