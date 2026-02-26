package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

func TestConvertResponseAPIToChatCompletionRequest_DefaultsFunctionSchema(t *testing.T) {
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "claude-sonnet-4-5",
		Stream: &stream,
		Input:  ResponseAPIInput{"hi"},
		Tools: []ResponseAPITool{
			{Type: "function", Name: "how-to-subscribe", Description: "explain subscription"},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)
	require.Len(t, chatReq.Tools, 1)
	require.NotNil(t, chatReq.Tools[0].Function)

	params, ok := chatReq.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", params["type"])
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

func TestConvertChatCompletionToResponseAPI_ToolOutputAlwaysIncludesOutputField(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-5-nano",
		Messages: []model.Message{
			{
				Role: "assistant",
				ToolCalls: []model.Tool{
					{
						Id:   "call_missing_output",
						Type: "function",
						Function: &model.Function{
							Name:      "demo_tool",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallId: "call_missing_output",
				Content:    "",
			},
		},
	}

	responseReq := ConvertChatCompletionToResponseAPI(chatRequest)
	require.NotNil(t, responseReq)
	require.Len(t, responseReq.Input, 2)

	outputItem, ok := responseReq.Input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", outputItem["type"])

	output, exists := outputItem["output"]
	require.True(t, exists, "output field must always exist for function_call_output items")
	require.Equal(t, "", output)
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
