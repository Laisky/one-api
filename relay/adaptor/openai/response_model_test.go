package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertChatCompletionToResponseAPI(t *testing.T) {
	// Test basic conversion
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "Hello, world!"},
		},
		MaxTokens:   100,
		Temperature: floatPtr(0.7),
		TopP:        floatPtr(0.9),
		Stream:      true,
		User:        "test-user",
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify basic fields
	require.Equal(t, "gpt-4", responseAPI.Model, "Expected model 'gpt-4'")
	require.Equal(t, 100, *responseAPI.MaxOutputTokens, "Expected max_output_tokens 100")
	require.Equal(t, 0.7, *responseAPI.Temperature, "Expected temperature 0.7")
	require.Equal(t, 0.9, *responseAPI.TopP, "Expected top_p 0.9")
	require.True(t, *responseAPI.Stream, "Expected stream to be true")
	require.Equal(t, "test-user", *responseAPI.User, "Expected user 'test-user'")

	// Verify input conversion
	require.Len(t, responseAPI.Input, 1, "Expected 1 input item")

	inputMessage, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok, "Expected input item to be map[string]interface{} type")
	require.Equal(t, "user", inputMessage["role"], "Expected message role 'user'")

	// Check content structure
	content, ok := inputMessage["content"].([]map[string]any)
	require.True(t, ok, "Expected content to be []map[string]interface{}")
	require.Len(t, content, 1, "Expected content length 1")
	require.Equal(t, "input_text", content[0]["type"], "Expected content type 'input_text'")
	require.Equal(t, "Hello, world!", content[0]["text"], "Expected message content 'Hello, world!'")
}

func TestConvertChatCompletionToResponseAPI_PreservesToolHistory(t *testing.T) {
	callID := "call_weather_history_1"
	request := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []model.Message{
			{Role: "system", Content: "You are a weather assistant."},
			{Role: "user", Content: "Fetch the current weather."},
			{
				Role: "assistant",
				ToolCalls: []model.Tool{
					{
						Id:   callID,
						Type: "function",
						Function: &model.Function{
							Name:      "get_weather",
							Arguments: `{"location":"San Francisco, CA","unit":"celsius"}`,
						},
					},
				},
			},
			{Role: "tool", ToolCallId: callID, Content: `{"temperature_c":15,"condition":"Foggy"}`},
			{Role: "user", Content: "Thanks, please call the tool again for tomorrow morning in Fahrenheit."},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:       "get_weather",
					Parameters: map[string]any{"type": "object"},
				},
			},
		},
		ToolChoice: map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "get_weather"},
		},
	}

	responseReq := ConvertChatCompletionToResponseAPI(request)
	require.NotNil(t, responseReq)
	require.NotNil(t, responseReq.Instructions)
	require.Equal(t, "You are a weather assistant.", *responseReq.Instructions)
	require.Len(t, responseReq.Input, 4)

	first, ok := responseReq.Input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", first["role"])

	functionCall, ok := responseReq.Input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", functionCall["type"])
	require.Equal(t, "get_weather", functionCall["name"])
	args, ok := functionCall["arguments"].(string)
	require.True(t, ok)
	require.Contains(t, args, "San Francisco")
	callReference, ok := functionCall["call_id"].(string)
	require.True(t, ok)

	callOutput, ok := responseReq.Input[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", callOutput["type"])
	require.Equal(t, callReference, callOutput["call_id"])
	require.Equal(t, `{"temperature_c":15,"condition":"Foggy"}`, callOutput["output"])

	followup, ok := responseReq.Input[3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", followup["role"])

	require.NotEmpty(t, responseReq.Tools)
	require.NotNil(t, responseReq.ToolChoice)
}

func TestNormalizeToolChoiceForResponse_Map(t *testing.T) {
	choice := map[string]any{
		"type": "tool",
		"name": "get_weather",
	}

	result, changed := NormalizeToolChoiceForResponse(choice)
	require.True(t, changed, "expected normalization to report changes")

	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "expected result to be map, got %T", result)

	typeVal, _ := resultMap["type"].(string)
	require.Equal(t, "function", typeVal, "expected type to be 'function'")
	require.Equal(t, "get_weather", stringFromAny(resultMap["name"]), "expected top-level name 'get_weather'")

	_, exists := resultMap["function"]
	require.False(t, exists, "expected function payload to be removed for Response API")
}

func TestNormalizeToolChoiceForResponse_String(t *testing.T) {
	result, changed := NormalizeToolChoiceForResponse(" auto ")
	require.True(t, changed, "expected whitespace trimming to be considered a change")

	str, _ := result.(string)
	require.Equal(t, "auto", str, "expected trimmed value 'auto'")

	result, changed = NormalizeToolChoiceForResponse("none")
	require.False(t, changed, "expected already normalized value to leave changed=false")
	require.Equal(t, "none", result.(string), "expected unchanged string 'none'")

	result, changed = NormalizeToolChoiceForResponse("   ")
	require.True(t, changed, "expected blank string to normalize with change=true")
	require.Nil(t, result, "expected blank string to normalize to nil")
}

func TestConvertWithSystemMessage(t *testing.T) {
	// Test system message conversion to instructions
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
		},
		MaxTokens: 50,
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify system message is converted to instructions
	require.NotNil(t, responseAPI.Instructions, "Expected instructions to be set")
	require.Equal(t, "You are a helpful assistant.", *responseAPI.Instructions, "Expected instructions 'You are a helpful assistant.'")

	// Verify system message is removed from input
	require.Len(t, responseAPI.Input, 1, "Expected 1 input item after system message removal")

	inputMessage, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok, "Expected input item to be map[string]interface{} type")
	require.Equal(t, "user", inputMessage["role"], "Expected remaining message to be user role")
}

func TestConvertWithTools(t *testing.T) {
	// Test tools conversion
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "What's the weather?"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_weather",
					Description: "Get current weather",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify tools are preserved
	require.Len(t, responseAPI.Tools, 1, "Expected 1 tool")
	require.NotNil(t, responseAPI.Tools[0].Function, "Expected function tool definition to be present")
	require.Equal(t, "get_weather", responseAPI.Tools[0].Function.Name, "Expected tool name 'get_weather'")
	require.Equal(t, "auto", responseAPI.ToolChoice, "Expected tool_choice 'auto'")
}

func TestConvertResponseAPIToChatCompletionRequest(t *testing.T) {
	reasoningEffort := "medium"
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "gpt-4",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "Hello there",
					},
				},
			},
		},
		Instructions: func() *string { s := "You are helpful"; return &s }(),
		Tools: []ResponseAPITool{
			{
				Type: "function",
				Name: "lookup",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
			{
				Type:              "web_search",
				SearchContextSize: func() *string { s := "medium"; return &s }(),
			},
		},
		ToolChoice: map[string]any{"type": "auto"},
		Reasoning:  &model.OpenAIResponseReasoning{Effort: &reasoningEffort},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err, "unexpected error")
	require.Equal(t, "gpt-4", chatReq.Model, "expected model gpt-4")
	require.Len(t, chatReq.Messages, 2, "expected 2 messages (system + user)")
	require.Equal(t, "system", chatReq.Messages[0].Role, "expected first message to be system")
	require.Equal(t, "Hello there", chatReq.Messages[1].StringContent(), "expected user message content preserved")
	require.Len(t, chatReq.Tools, 1, "expected 1 tool after filtering")
	require.NotNil(t, chatReq.Tools[0].Function, "function tool not converted correctly")
	require.Equal(t, "lookup", chatReq.Tools[0].Function.Name, "function tool not converted correctly")
	require.NotNil(t, chatReq.ToolChoice, "expected tool choice to be set")
	require.NotNil(t, chatReq.Reasoning, "reasoning not preserved")
	require.NotNil(t, chatReq.Reasoning.Effort, "reasoning effort not preserved")
	require.Equal(t, reasoningEffort, *chatReq.Reasoning.Effort, "reasoning effort not preserved")
}

func TestConvertResponseAPIToChatCompletionRequest_ToolHistoryRoundTrip(t *testing.T) {
	stream := false
	instructions := "You are a weather assistant."
	responseReq := &ResponseAPIRequest{
		Model:        "gpt-4o-mini",
		Stream:       &stream,
		Instructions: &instructions,
		Input: ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "Fetch the current weather for San Francisco, CA.",
					},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_weather_history",
				"call_id":   "call_weather_history",
				"name":      "get_weather",
				"arguments": `{"location":"San Francisco, CA","unit":"celsius"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_weather_history",
				"output":  `{"temperature_c":15,"condition":"Foggy"}`,
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "Please call the tool again for tomorrow morning's forecast in Fahrenheit.",
					},
				},
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)
	require.Len(t, chatReq.Messages, 5)

	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, instructions, chatReq.Messages[0].StringContent())

	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.Equal(t, "Fetch the current weather for San Francisco, CA.", chatReq.Messages[1].StringContent())

	assistant := chatReq.Messages[2]
	require.Equal(t, "assistant", assistant.Role)
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, "get_weather", assistant.ToolCalls[0].Function.Name)
	require.Contains(t, assistant.ToolCalls[0].Function.Arguments, "San Francisco")

	toolMsg := chatReq.Messages[3]
	require.Equal(t, "tool", toolMsg.Role)
	require.Equal(t, assistant.ToolCalls[0].Id, toolMsg.ToolCallId)
	require.Equal(t, `{"temperature_c":15,"condition":"Foggy"}`, toolMsg.StringContent())

	followup := chatReq.Messages[4]
	require.Equal(t, "user", followup.Role)
	require.Contains(t, followup.StringContent(), "tomorrow morning")
}

func TestConvertResponseAPIToChatCompletion_RequiredActionToolCalls(t *testing.T) {
	response := &ResponseAPIResponse{
		Id:     "resp_required",
		Model:  "gpt-5-nano",
		Status: "incomplete",
		RequiredAction: &ResponseAPIRequiredAction{
			Type: "submit_tool_outputs",
			SubmitToolOutputs: &ResponseAPISubmitToolOutputs{
				ToolCalls: []ResponseAPIToolCall{
					{
						Id:   "call_weather",
						Type: "function",
						Function: &ResponseAPIFunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"San Francisco, CA"}`,
						},
					},
				},
			},
		},
	}

	converted := ConvertResponseAPIToChatCompletion(response)
	require.NotNil(t, converted)
	require.Len(t, converted.Choices, 1)
	choice := converted.Choices[0]
	require.Len(t, choice.Message.ToolCalls, 1)
	call := choice.Message.ToolCalls[0]
	require.Equal(t, "call_weather", call.Id)
	require.Equal(t, "function", call.Type)
	require.NotNil(t, call.Function)
	require.Equal(t, "get_weather", call.Function.Name)
	require.Equal(t, `{"location":"San Francisco, CA"}`, call.Function.Arguments)
	require.Equal(t, "tool_calls", choice.FinishReason)
}

func TestConvertResponseAPIStreamToChatCompletion_RequiredActionToolCalls(t *testing.T) {
	response := &ResponseAPIResponse{
		Id:     "resp_required_stream",
		Model:  "gpt-5-nano",
		Status: "incomplete",
		RequiredAction: &ResponseAPIRequiredAction{
			Type: "submit_tool_outputs",
			SubmitToolOutputs: &ResponseAPISubmitToolOutputs{
				ToolCalls: []ResponseAPIToolCall{
					{
						Id:   "call_weather",
						Type: "function",
						Function: &ResponseAPIFunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"San Francisco, CA"}`,
						},
					},
				},
			},
		},
	}

	chunk := ConvertResponseAPIStreamToChatCompletion(response)
	require.NotNil(t, chunk)
	require.Len(t, chunk.Choices, 1)
	choice := chunk.Choices[0]
	require.Equal(t, "assistant", choice.Delta.Role)
	require.Len(t, choice.Delta.ToolCalls, 1)
	call := choice.Delta.ToolCalls[0]
	require.Equal(t, "call_weather", call.Id)
	require.Equal(t, "function", call.Type)
	require.NotNil(t, call.Function)
	require.Equal(t, "get_weather", call.Function.Name)
	require.Equal(t, `{"location":"San Francisco, CA"}`, call.Function.Arguments)
}

func TestConvertResponseAPIToChatCompletionRequestDropsUnsupportedTools(t *testing.T) {
	stream := true
	responseReq := &ResponseAPIRequest{
		Model:  "deepseek-chat",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role":    "system",
				"content": "You are AFFiNE AI.",
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "Hello",
					},
				},
			},
		},
		Tools: []ResponseAPITool{
			{Type: "web_search_preview"},
			{Type: "web_search"},
			{
				Type: "function",
				Function: &model.Function{
					Name:        "section_edit",
					Description: "Edit a section",
					Parameters: map[string]any{
						"type": "object",
					},
				},
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "web_search"},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err, "unexpected error")
	require.Len(t, chatReq.Tools, 1, "expected only supported tools to remain")
	require.NotNil(t, chatReq.Tools[0].Function, "expected section_edit function to remain")
	require.Equal(t, "section_edit", chatReq.Tools[0].Function.Name, "expected section_edit function to remain")

	choice, ok := chatReq.ToolChoice.(map[string]any)
	require.True(t, ok, "expected tool choice map, got %T", chatReq.ToolChoice)
	require.Equal(t, "auto", strings.ToLower(choice["type"].(string)), "expected tool_choice to downgrade to auto")
}

func TestConvertResponseAPIToChatCompletionRequestSanitizesFunctionParameters(t *testing.T) {
	stream := false
	strict := true
	responseReq := &ResponseAPIRequest{
		Model:  "gemini-2.5-flash",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": "Hello",
					},
				},
			},
		},
		Tools: []ResponseAPITool{
			{
				Type: "function",
				Name: "get_weather",
				Parameters: map[string]any{
					"$schema":              "http://json-schema.org/draft-07/schema#",
					"description":          "should be stripped",
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"city": map[string]any{
							"type":                 "string",
							"additionalProperties": map[string]any{"oops": true},
						},
					},
					"required": []any{"city"},
				},
			},
		},
		Text: &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type: "json_schema",
				Schema: map[string]any{
					"$schema":              "http://json-schema.org/draft-07/schema#",
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"answer": map[string]any{
							"type":                 "string",
							"additionalProperties": false,
						},
					},
				},
				Strict: &strict,
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err, "unexpected error")
	require.Len(t, chatReq.Tools, 1, "expected a single sanitized function tool")

	fn := chatReq.Tools[0].Function
	require.NotNil(t, fn, "expected function to be present")
	require.Nil(t, fn.Strict, "expected strict flag to be cleared")

	params, ok := fn.Parameters.(map[string]any)
	require.True(t, ok, "expected parameters map, got %T", fn.Parameters)

	_, exists := params["$schema"]
	require.False(t, exists, "$schema should be removed from function parameters")
	_, exists = params["description"]
	require.False(t, exists, "description should be removed from top-level parameters")
	_, exists = params["additionalProperties"]
	require.False(t, exists, "additionalProperties should be removed from function parameters")

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok, "expected properties map")
	city, ok := props["city"].(map[string]any)
	require.True(t, ok, "expected city property map")
	_, exists = city["additionalProperties"]
	require.False(t, exists, "nested additionalProperties should be removed")

	require.NotNil(t, chatReq.ResponseFormat, "expected response format to be preserved")
	require.NotNil(t, chatReq.ResponseFormat.JsonSchema, "expected response format schema to be preserved")

	schema := chatReq.ResponseFormat.JsonSchema.Schema
	require.NotNil(t, schema, "expected sanitized schema")

	_, exists = schema["$schema"]
	require.False(t, exists, "$schema should be pruned from response schema")
	_, exists = schema["additionalProperties"]
	require.False(t, exists, "additionalProperties should be pruned from response schema")
	require.Nil(t, chatReq.ResponseFormat.JsonSchema.Strict, "expected response schema strict flag to be cleared")
}

func TestConvertChatCompletionToResponseAPISanitizesEncryptedReasoning(t *testing.T) {
	req := &model.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []model.Message{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":              "reasoning",
						"encrypted_content": "gAAAA...",
						"summary": []any{
							map[string]any{
								"type": "summary_text",
								"text": "Concise reasoning summary",
							},
						},
					},
				},
			},
		},
	}

	converted := ConvertChatCompletionToResponseAPI(req)

	require.Len(t, converted.Input, 1, "expected single sanitized message")

	msg, ok := converted.Input[0].(map[string]any)
	require.True(t, ok, "expected map message, got %T", converted.Input[0])

	content, ok := msg["content"].([]map[string]any)
	require.True(t, ok, "expected content slice, got %T", msg["content"])
	require.Len(t, content, 1, "expected single content item")

	item := content[0]
	require.Equal(t, "output_text", item["type"], "expected output_text type")
	require.Equal(t, "Concise reasoning summary", item["text"], "expected sanitized summary text")
	_, exists := item["encrypted_content"]
	require.False(t, exists, "encrypted_content should be removed")
}

func TestConvertChatCompletionToResponseAPIDropsUnverifiableReasoning(t *testing.T) {
	req := &model.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []model.Message{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":              "reasoning",
						"encrypted_content": "gAAAA...",
					},
				},
			},
		},
	}

	converted := ConvertChatCompletionToResponseAPI(req)
	require.Empty(t, converted.Input, "expected unverifiable reasoning message to be dropped")
}

func TestConvertWithResponseFormat(t *testing.T) {
	// Test response format conversion
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "Generate JSON"},
		},
		ResponseFormat: &model.ResponseFormat{
			Type: "json_object",
			JsonSchema: &model.JSONSchema{
				Name:        "response_schema",
				Description: "Test schema",
				Schema: map[string]any{
					"type": "object",
				},
			},
		},
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify response format conversion
	require.NotNil(t, responseAPI.Text, "Expected text config to be set")
	require.NotNil(t, responseAPI.Text.Format, "Expected text format to be set")
	require.Equal(t, "json_object", responseAPI.Text.Format.Type, "Expected text format type to be 'json_object'")
	require.Equal(t, "response_schema", responseAPI.Text.Format.Name, "Expected schema name 'response_schema'")
	require.Equal(t, "Test schema", responseAPI.Text.Format.Description, "Expected schema description 'Test schema'")
	require.NotNil(t, responseAPI.Text.Format.Schema, "Expected JSON schema to be set")
}

// TestConvertResponseAPIToChatCompletion tests the conversion from Response API format back to ChatCompletion format
func TestConvertResponseAPIToChatCompletion(t *testing.T) {
	// Create a Response API response
	responseAPI := &ResponseAPIResponse{
		Id:        "resp_123",
		Object:    "response",
		CreatedAt: 1234567890,
		Status:    "completed",
		Model:     "gpt-4",
		Output: []OutputItem{
			{
				Type:   "message",
				Id:     "msg_123",
				Status: "completed",
				Role:   "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: "Hello! How can I help you today?",
					},
				},
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  10,
			OutputTokens: 8,
			TotalTokens:  18,
		},
	}

	// Convert to ChatCompletion format
	chatCompletion := ConvertResponseAPIToChatCompletion(responseAPI)

	// Verify basic fields
	require.Equal(t, "resp_123", chatCompletion.Id, "Expected id 'resp_123'")
	require.Equal(t, "chat.completion", chatCompletion.Object, "Expected object 'chat.completion'")
	require.Equal(t, "gpt-4", chatCompletion.Model, "Expected model 'gpt-4'")
	require.Equal(t, int64(1234567890), chatCompletion.Created, "Expected created 1234567890")

	// Verify choices
	require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

	choice := chatCompletion.Choices[0]
	require.Equal(t, 0, choice.Index, "Expected choice index 0")
	require.Equal(t, "assistant", choice.Message.Role, "Expected role 'assistant'")
	require.Nil(t, choice.Message.Reasoning, "Expected reasoning to be nil")
	require.Equal(t, "stop", choice.FinishReason, "Expected finish_reason 'stop'")

	// Verify usage
	require.Equal(t, 10, chatCompletion.Usage.PromptTokens, "Expected prompt_tokens 10")
	require.Equal(t, 8, chatCompletion.Usage.CompletionTokens, "Expected completion_tokens 8")
	require.Equal(t, 18, chatCompletion.Usage.TotalTokens, "Expected total_tokens 18")
}

// TestConvertResponseAPIToChatCompletionWithFunctionCall tests the conversion with function calls
func TestConvertResponseAPIToChatCompletionWithFunctionCall(t *testing.T) {
	// Create a Response API response with function call (based on the real example)
	responseAPI := &ResponseAPIResponse{
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

	// Convert to ChatCompletion format
	chatCompletion := ConvertResponseAPIToChatCompletion(responseAPI)

	// Verify basic fields
	require.Equal(t, "resp_67ca09c5efe0819096d0511c92b8c890096610f474011cc0", chatCompletion.Id)
	require.Equal(t, "gpt-4.1-2025-04-14", chatCompletion.Model)

	// Verify choices
	require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

	choice := chatCompletion.Choices[0]
	require.Equal(t, 0, choice.Index, "Expected choice index 0")
	require.Equal(t, "assistant", choice.Message.Role, "Expected role 'assistant'")

	// Verify tool calls
	require.Len(t, choice.Message.ToolCalls, 1, "Expected 1 tool call")

	toolCall := choice.Message.ToolCalls[0]
	require.Equal(t, "call_unLAR8MvFNptuiZK6K6HCy5k", toolCall.Id)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, "get_current_weather", toolCall.Function.Name)

	expectedArgs := "{\"location\":\"Boston, MA\",\"unit\":\"celsius\"}"
	require.Equal(t, expectedArgs, toolCall.Function.Arguments)
	require.Equal(t, "tool_calls", choice.FinishReason)

	// Verify usage
	require.Equal(t, 291, chatCompletion.Usage.PromptTokens)
	require.Equal(t, 23, chatCompletion.Usage.CompletionTokens)
	require.Equal(t, 314, chatCompletion.Usage.TotalTokens)
}

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
