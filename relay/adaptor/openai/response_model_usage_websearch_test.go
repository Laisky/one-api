package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertResponseAPIToChatCompletionRequestWithFunctionHistory(t *testing.T) {
	callSuffix := "weather_history_1"
	req := &ResponseAPIRequest{
		Model: "gpt-4o-mini",
		Input: ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "What's the weather?",
					},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_" + callSuffix,
				"call_id":   "call_" + callSuffix,
				"name":      "get_weather",
				"arguments": map[string]any{"location": "San Francisco"},
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_" + callSuffix,
				"output":  map[string]any{"temperature_c": 15},
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(req)
	require.NoError(t, err, "expected conversion without error")
	require.Len(t, chatReq.Messages, 3, "expected 3 messages")
	require.Equal(t, "user", chatReq.Messages[0].Role, "expected first message role user")

	assistantMsg := chatReq.Messages[1]
	require.Equal(t, "assistant", assistantMsg.Role, "expected second message role assistant")
	require.Len(t, assistantMsg.ToolCalls, 1, "expected second message to include 1 tool call")
	toolCall := assistantMsg.ToolCalls[0]
	require.Equal(t, callSuffix, toolCall.Id, "expected tool call id")
	require.NotNil(t, toolCall.Function, "expected tool call to include function details")
	require.Equal(t, "get_weather", toolCall.Function.Name, "expected function name get_weather")
	require.NotEmpty(t, toolCall.Function.Arguments, "expected function arguments to be populated")

	toolMsg := chatReq.Messages[2]
	require.Equal(t, "tool", toolMsg.Role, "expected third message role tool")
	require.Equal(t, callSuffix, toolMsg.ToolCallId, "expected tool message tool_call_id")
	require.NotEmpty(t, toolMsg.Content, "expected tool message content to be populated")
}

// TestStreamingToolCallsIndexField tests that the Index field is properly set in streaming tool calls
func TestStreamingToolCallsIndexField(t *testing.T) {
	// Create a Response API streaming chunk with function call
	responseAPIChunk := &ResponseAPIResponse{
		Id:        "resp_123",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "in_progress",
		Output: []OutputItem{
			{
				Type:      "function_call",
				CallId:    "call_abc123",
				Name:      "get_weather",
				Arguments: `{"location": "Paris"}`,
			},
		},
	}

	// Convert to ChatCompletion streaming format
	chatCompletionChunk := ConvertResponseAPIStreamToChatCompletion(responseAPIChunk)

	// Verify the response structure
	require.Len(t, chatCompletionChunk.Choices, 1, "Expected 1 choice")

	choice := chatCompletionChunk.Choices[0]
	require.Len(t, choice.Delta.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Delta.ToolCalls[0]

	// Verify that the Index field is set
	require.NotNil(t, toolCall.Index, "Index field should be set for streaming tool calls")
	require.Equal(t, 0, *toolCall.Index, "Expected index to be 0")

	// Verify other tool call fields
	require.Equal(t, "call_abc123", toolCall.Id)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, "get_weather", toolCall.Function.Name)

	expectedArgs := `{"location": "Paris"}`
	require.Equal(t, expectedArgs, toolCall.Function.Arguments)
}

// TestStreamingToolCallsWithOutputIndex tests that the Index field is properly set using output_index from streaming events
func TestStreamingToolCallsWithOutputIndex(t *testing.T) {
	// Test with explicit output_index from streaming event
	responseAPIChunk := &ResponseAPIResponse{
		Id:        "resp_456",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "in_progress",
		Output: []OutputItem{
			{
				Type:      "function_call",
				CallId:    "call_def456",
				Name:      "send_email",
				Arguments: `{"to": "test@example.com"}`,
			},
		},
	}

	// Simulate output_index = 2 from a streaming event (e.g., this is the 3rd tool call)
	outputIndex := 2
	chatCompletionChunk := ConvertResponseAPIStreamToChatCompletionWithIndex(responseAPIChunk, &outputIndex)

	// Verify the response structure
	require.Len(t, chatCompletionChunk.Choices, 1, "Expected 1 choice")

	choice := chatCompletionChunk.Choices[0]
	require.Len(t, choice.Delta.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Delta.ToolCalls[0]

	// Verify that the Index field is set to the provided output_index
	require.NotNil(t, toolCall.Index, "Index field should be set for streaming tool calls")
	require.Equal(t, 2, *toolCall.Index, "Expected index to be 2 (from output_index)")

	// Verify other tool call fields
	require.Equal(t, "call_def456", toolCall.Id)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, "send_email", toolCall.Function.Name)
}

// TestMultipleStreamingToolCallsIndexConsistency tests that multiple tool calls get consistent indices
func TestMultipleStreamingToolCallsIndexConsistency(t *testing.T) {
	// Test multiple tool calls with different output_index values
	testCases := []struct {
		name        string
		outputIndex *int
		expectedIdx int
	}{
		{
			name:        "First tool call with output_index 0",
			outputIndex: func() *int { i := 0; return &i }(),
			expectedIdx: 0,
		},
		{
			name:        "Second tool call with output_index 1",
			outputIndex: func() *int { i := 1; return &i }(),
			expectedIdx: 1,
		},
		{
			name:        "Third tool call with output_index 2",
			outputIndex: func() *int { i := 2; return &i }(),
			expectedIdx: 2,
		},
		{
			name:        "Tool call without output_index (fallback to position)",
			outputIndex: nil,
			expectedIdx: 0, // Should fallback to position in slice (0)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			responseAPIChunk := &ResponseAPIResponse{
				Id:        "resp_multi",
				Object:    "response",
				CreatedAt: 1234567890,
				Status:    "in_progress",
				Output: []OutputItem{
					{
						Type:      "function_call",
						CallId:    "call_multi_123",
						Name:      "test_function",
						Arguments: `{"param": "value"}`,
					},
				},
			}

			chatCompletionChunk := ConvertResponseAPIStreamToChatCompletionWithIndex(responseAPIChunk, tc.outputIndex)

			// Verify the index is set correctly
			require.Len(t, chatCompletionChunk.Choices, 1, "Expected 1 choice")

			choice := chatCompletionChunk.Choices[0]
			require.Len(t, choice.Delta.ToolCalls, 1, "Expected 1 tool call")

			toolCall := choice.Delta.ToolCalls[0]
			require.NotNil(t, toolCall.Index, "Index field should be set for streaming tool calls")
			require.Equal(t, tc.expectedIdx, *toolCall.Index, "Expected index to be %d", tc.expectedIdx)
		})
	}
}

func TestResponseAPIUsageConversion(t *testing.T) {
	// Test JSON containing OpenAI Response API usage format
	responseJSON := `{
		"id": "resp_test",
		"object": "response",
		"created_at": 1749860991,
		"status": "completed",
		"model": "gpt-4o-2024-11-20",
		"output": [],
		"usage": {
			"input_tokens": 97,
			"output_tokens": 165,
			"total_tokens": 262
		}
	}`

	var responseAPI ResponseAPIResponse
	err := json.Unmarshal([]byte(responseJSON), &responseAPI)
	require.NoError(t, err, "Failed to unmarshal ResponseAPI")

	// Verify the ResponseAPIUsage fields are correctly parsed
	require.NotNil(t, responseAPI.Usage, "Usage should not be nil")
	require.Equal(t, 97, responseAPI.Usage.InputTokens)
	require.Equal(t, 165, responseAPI.Usage.OutputTokens)
	require.Equal(t, 262, responseAPI.Usage.TotalTokens)

	// Test conversion to model.Usage
	modelUsage := responseAPI.Usage.ToModelUsage()
	require.NotNil(t, modelUsage, "Converted usage should not be nil")
	require.Equal(t, 97, modelUsage.PromptTokens)
	require.Equal(t, 165, modelUsage.CompletionTokens)
	require.Equal(t, 262, modelUsage.TotalTokens)

	// Test conversion to ChatCompletion format
	chatCompletion := ConvertResponseAPIToChatCompletion(&responseAPI)
	require.NotNil(t, chatCompletion, "Converted chat completion should not be nil")
	require.Equal(t, 97, chatCompletion.Usage.PromptTokens)
	require.Equal(t, 165, chatCompletion.Usage.CompletionTokens)
	require.Equal(t, 262, chatCompletion.Usage.TotalTokens)
}

func TestResponseAPIUsageWithFallback(t *testing.T) {
	// Test case 1: No usage provided by OpenAI
	responseWithoutUsage := `{
		"id": "resp_no_usage",
		"object": "response",
		"created_at": 1749860991,
		"status": "completed",
		"model": "gpt-4o-2024-11-20",
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{
						"type": "output_text",
						"text": "Hello! How can I help you today?"
					}
				]
			}
		]
	}`

	var responseAPI ResponseAPIResponse
	err := json.Unmarshal([]byte(responseWithoutUsage), &responseAPI)
	require.NoError(t, err, "Failed to unmarshal ResponseAPI")

	// Convert to ChatCompletion format
	chatCompletion := ConvertResponseAPIToChatCompletion(&responseAPI)

	// Usage should be zero/empty since no usage was provided and no fallback calculation is done in the conversion function
	require.Equal(t, 0, chatCompletion.Usage.PromptTokens, "Expected zero usage when no usage provided")
	require.Equal(t, 0, chatCompletion.Usage.CompletionTokens, "Expected zero usage when no usage provided")

	// Test case 2: Zero usage provided by OpenAI
	responseWithZeroUsage := `{
		"id": "resp_zero_usage",
		"object": "response",
		"created_at": 1749860991,
		"status": "completed",
		"model": "gpt-4o-2024-11-20",
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{
						"type": "output_text",
						"text": "This is a test response"
					}
				]
			}
		],
		"usage": {
			"input_tokens": 0,
			"output_tokens": 0,
			"total_tokens": 0
		}
	}`

	err = json.Unmarshal([]byte(responseWithZeroUsage), &responseAPI)
	require.NoError(t, err, "Failed to unmarshal ResponseAPI with zero usage")

	// Convert to ChatCompletion format
	chatCompletion = ConvertResponseAPIToChatCompletion(&responseAPI)

	// Usage should still be zero since the conversion function doesn't set zero usage
	require.Equal(t, 0, chatCompletion.Usage.PromptTokens, "Expected zero usage when zero usage provided")
	require.Equal(t, 0, chatCompletion.Usage.CompletionTokens, "Expected zero usage when zero usage provided")
}

func TestResponseAPIUsageToModelMatchesRealLog(t *testing.T) {
	payload := []byte(`{"input_tokens":8555,"input_tokens_details":{"cached_tokens":4224},"output_tokens":889,"output_tokens_details":{"reasoning_tokens":640},"total_tokens":9444}`)
	var usage ResponseAPIUsage
	err := json.Unmarshal(payload, &usage)
	require.NoError(t, err, "failed to unmarshal usage")

	modelUsage := usage.ToModelUsage()
	require.NotNil(t, modelUsage, "expected model usage")
	require.Equal(t, 8555, modelUsage.PromptTokens, "expected prompt tokens 8555")
	require.Equal(t, 889, modelUsage.CompletionTokens, "expected completion tokens 889")
	require.Equal(t, 9444, modelUsage.TotalTokens, "expected total tokens 9444")
	require.NotNil(t, modelUsage.PromptTokensDetails, "expected prompt token details")
	require.Equal(t, 4224, modelUsage.PromptTokensDetails.CachedTokens, "expected cached tokens 4224")
	require.NotNil(t, modelUsage.CompletionTokensDetails, "expected completion token details")
	require.Equal(t, 640, modelUsage.CompletionTokensDetails.ReasoningTokens, "expected reasoning tokens 640")
}

func TestResponseAPIUsageRoundTripPreservesKnownDetails(t *testing.T) {
	modelUsage := &model.Usage{
		PromptTokens:     12000,
		CompletionTokens: 900,
		TotalTokens:      12900,
		PromptTokensDetails: &model.UsagePromptTokensDetails{
			CachedTokens: 4224,
		},
		CompletionTokensDetails: &model.UsageCompletionTokensDetails{ReasoningTokens: 640},
	}

	converted := (&ResponseAPIUsage{}).FromModelUsage(modelUsage)
	require.NotNil(t, converted, "expected converted usage")
	require.NotNil(t, converted.InputTokensDetails, "expected input token details in converted usage")
	require.Equal(t, 4224, converted.InputTokensDetails.CachedTokens, "expected cached tokens 4224")

	encoded, err := json.Marshal(converted)
	require.NoError(t, err, "failed to marshal converted usage")

	var generic map[string]any
	err = json.Unmarshal(encoded, &generic)
	require.NoError(t, err, "failed to unmarshal converted usage json")

	inputAny, exists := generic["input_tokens_details"]
	require.True(t, exists, "expected input_tokens_details key in marshalled usage")

	inputMap, ok := inputAny.(map[string]any)
	require.True(t, ok, "expected input_tokens_details to be object, got %T", inputAny)

	_, exists = inputMap["web_search_content_tokens"]
	require.False(t, exists, "did not expect web_search_content_tokens to be present in marshalled usage")
}

func TestResponseAPIInputTokensDetailsWebSearchInvocationCount(t *testing.T) {
	details := &ResponseAPIInputTokensDetails{
		WebSearch: map[string]any{"requests": float64(4)},
	}
	require.Equal(t, 4, details.WebSearchInvocationCount(), "expected 4 requests")

	details.WebSearch = map[string]any{"requests": []any{map[string]any{}, map[string]any{}, map[string]any{}}}
	require.Equal(t, 3, details.WebSearchInvocationCount(), "expected 3 requests from slice")

	details.WebSearch = " 2 "
	require.Equal(t, 2, details.WebSearchInvocationCount(), "expected string count 2")

	details.WebSearch = nil
	require.Equal(t, 0, details.WebSearchInvocationCount(), "expected zero count for nil")
}

func TestCountWebSearchSearchActionsFromLog(t *testing.T) {
	outputs := []OutputItem{
		{Id: "rs_1", Type: "reasoning"},
		{Id: "ws_08eb", Type: "web_search_call", Status: "completed", Action: &WebSearchCallAction{Type: "search", Query: "positive news today October 8 2025 good news Oct 8 2025"}},
		{Id: "msg_1", Type: "message", Role: "assistant", Content: []OutputContent{{Type: "output_text", Text: "Today positive news."}}},
	}

	require.Equal(t, 1, countWebSearchSearchActions(outputs), "expected 1 web search call")

	seen := map[string]struct{}{"ws_08eb": {}}
	require.Equal(t, 0, countNewWebSearchSearchActions(outputs, seen), "expected duplicate detection to yield 0 new calls")
}

func TestConvertChatCompletionToResponseAPIWebSearch(t *testing.T) {
	req := &model.GeneralOpenAIRequest{
		Model:  "gpt-4o-search-preview",
		Stream: true,
		Messages: []model.Message{
			{Role: "user", Content: "What was a positive news story from today?"},
		},
		WebSearchOptions: &model.WebSearchOptions{},
	}

	converted := ConvertChatCompletionToResponseAPI(req)
	require.NotNil(t, converted, "expected converted request")
	require.Equal(t, "gpt-4o-search-preview", converted.Model)
	require.Len(t, converted.Tools, 1, "expected exactly one tool")
	require.True(t, strings.EqualFold(converted.Tools[0].Type, "web_search"), "expected tool type web_search")
	require.NotNil(t, converted.Stream, "expected stream flag to be set")
	require.True(t, *converted.Stream, "expected stream flag to be set to true")
}

func TestConvertResponseAPIToChatCompletionWebSearch(t *testing.T) {
	resp := &ResponseAPIResponse{
		Id:     "resp_08eb",
		Object: "response",
		Model:  "gpt-5-mini-2025-08-07",
		Output: []OutputItem{
			{Id: "rs_1", Type: "reasoning"},
			{Id: "ws_08eb", Type: "web_search_call", Status: "completed", Action: &WebSearchCallAction{Type: "search", Query: "positive news today October 8 2025 good news Oct 8 2025"}},
			{Id: "msg_1", Type: "message", Role: "assistant", Content: []OutputContent{{Type: "output_text", Text: "Today (October 8, 2025) one clear positive story was..."}}},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:         8555,
			OutputTokens:        889,
			TotalTokens:         9444,
			InputTokensDetails:  &ResponseAPIInputTokensDetails{CachedTokens: 4224},
			OutputTokensDetails: &ResponseAPIOutputTokensDetails{ReasoningTokens: 640},
		},
	}

	chat := ConvertResponseAPIToChatCompletion(resp)
	require.NotEmpty(t, chat.Choices, "expected chat choices")

	choice := chat.Choices[0]
	content, ok := choice.Message.Content.(string)
	require.True(t, ok, "expected message content string, got %T", choice.Message.Content)
	require.Contains(t, content, "positive story", "expected converted content to include summary text")
	require.Equal(t, 8555, chat.Usage.PromptTokens, "expected prompt tokens 8555")
	require.NotNil(t, chat.Usage.PromptTokensDetails, "expected prompt token details in converted chat response")
	require.Equal(t, 4224, chat.Usage.PromptTokensDetails.CachedTokens, "expected cached tokens 4224")
	require.NotNil(t, chat.Usage.CompletionTokensDetails, "expected completion token details")
	require.Equal(t, 640, chat.Usage.CompletionTokensDetails.ReasoningTokens, "expected reasoning tokens 640 in completion details")
	require.Equal(t, 1, countWebSearchSearchActions(resp.Output), "expected web search action count 1")
}

func TestConvertResponseAPIToClaudeResponseWebSearch(t *testing.T) {
	resp := &ResponseAPIResponse{
		Id:     "resp_08eb",
		Object: "response",
		Model:  "gpt-5-mini-2025-08-07",
		Output: []OutputItem{
			{Id: "rs_1", Type: "reasoning", Summary: []OutputContent{{Type: "summary_text", Text: "analysis"}}},
			{Id: "msg_1", Type: "message", Role: "assistant", Content: []OutputContent{{Type: "output_text", Text: "Today positive developments."}}},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:         8555,
			OutputTokens:        889,
			TotalTokens:         9444,
			InputTokensDetails:  &ResponseAPIInputTokensDetails{CachedTokens: 4224},
			OutputTokensDetails: &ResponseAPIOutputTokensDetails{ReasoningTokens: 640},
		},
	}

	upstream := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	converted, errResp := (&Adaptor{}).ConvertResponseAPIToClaudeResponse(nil, upstream, resp)
	require.Nil(t, errResp, "unexpected error from conversion")
	require.Equal(t, http.StatusOK, converted.StatusCode)

	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err, "failed to read converted body")

	err = converted.Body.Close()
	require.NoError(t, err, "failed to close response body")

	var claude model.ClaudeResponse
	err = json.Unmarshal(body, &claude)
	require.NoError(t, err, "failed to unmarshal claude response")
	require.Equal(t, 8555, claude.Usage.InputTokens, "expected usage tokens 8555")
	require.Equal(t, 889, claude.Usage.OutputTokens, "expected usage tokens 889")
	require.NotEmpty(t, claude.Content, "expected claude content")

	foundText := false
	for _, content := range claude.Content {
		if content.Type == "text" && strings.Contains(content.Text, "positive") {
			foundText = true
			break
		}
	}
	require.True(t, foundText, "expected claude content to include assistant text")
}

func TestConvertResponseAPIToClaudeResponseAddsToolUse(t *testing.T) {
	resp := &ResponseAPIResponse{
		Id:     "resp_tool",
		Object: "response",
		Model:  "gpt-4o-mini-2024-07-18",
		Output: []OutputItem{
			{Type: "function_call", Id: "fc_tool", CallId: "call_weather", Name: "get_weather", Arguments: "{\"location\":\"San Francisco, CA\"}"},
		},
	}

	upstream := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	converted, errResp := (&Adaptor{}).ConvertResponseAPIToClaudeResponse(nil, upstream, resp)
	require.Nil(t, errResp, "unexpected error from conversion")

	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err, "failed to read converted body")

	err = converted.Body.Close()
	require.NoError(t, err, "failed to close response body")

	var claude model.ClaudeResponse
	err = json.Unmarshal(body, &claude)
	require.NoError(t, err, "failed to unmarshal claude response")
	require.Equal(t, "tool_use", claude.StopReason, "expected stop reason tool_use")

	foundToolUse := false
	for _, content := range claude.Content {
		if content.Type == "tool_use" {
			foundToolUse = true
			require.Equal(t, "call_weather", content.ID, "expected tool_use id call_weather")
			require.Equal(t, "get_weather", content.Name, "expected tool_use name get_weather")
			require.NotEmpty(t, content.Input, "expected tool_use input to be populated")
		}
	}

	require.True(t, foundToolUse, "expected tool_use content block in claude response")
}

func TestDeepResearchConversionIncludesWebSearchTool(t *testing.T) {
	req := &model.GeneralOpenAIRequest{
		Model: "o3-deep-research",
		Messages: []model.Message{
			{Role: "user", Content: "Research topic"},
		},
	}

	adaptor := &Adaptor{}
	metaInfo := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "o3-deep-research"}

	err := adaptor.applyRequestTransformations(metaInfo, req)
	require.NoError(t, err, "applyRequestTransformations returned error")

	converted := ConvertChatCompletionToResponseAPI(req)
	require.NotEmpty(t, converted.Tools, "expected tools to include web_search for deep research model")

	found := false
	for _, tool := range converted.Tools {
		if strings.EqualFold(tool.Type, "web_search") {
			found = true
			break
		}
	}

	require.True(t, found, "web_search tool not found in converted Response API request")
}
