package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertResponseAPIToChatCompletionRequest verifies the established Responses-to-Chat conversion contract.
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

// TestConvertResponseAPIToChatCompletionRequest_DefaultsFunctionSchema verifies missing function schemas receive their established default.
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

// TestConvertResponseAPIToChatCompletionRequest_ToolHistoryRoundTrip verifies function call and output identifiers remain paired.
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

// TestConvertResponseAPIToChatCompletionRequestDropsUnsupportedTools verifies the legacy tool conversion behavior for unsupported definitions.
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

// TestConvertResponseAPIToChatCompletionRequestSanitizesFunctionParameters verifies function-parameter sanitization remains unchanged.
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
	require.NotNil(t, schema, "expected explicit strict response schema")
	require.Equal(t, responseReq.Text.Format.Schema, schema, "explicit strict preserves the complete text schema")
	require.NotNil(t, chatReq.ResponseFormat.JsonSchema.Strict)
	require.True(t, *chatReq.ResponseFormat.JsonSchema.Strict)
}

// assertToolMessagesFollowToolCalls enforces the ChatCompletion invariant that DeepSeek
// (and OpenAI) require: every message with role "tool" must be immediately preceded by an
// assistant message carrying tool_calls, or by another tool message that belongs to the same
// preceding assistant tool_calls group. Violating this is exactly the upstream 400:
// "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'".
func assertToolMessagesFollowToolCalls(t *testing.T, messages []model.Message) {
	t.Helper()
	for i, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		require.Greaterf(t, i, 0, "tool message at index %d has no preceding message", i)
		prev := messages[i-1]
		switch prev.Role {
		case "assistant":
			require.NotEmptyf(t, prev.ToolCalls,
				"tool message at index %d follows an assistant message without tool_calls", i)
		case "tool":
			// allowed: part of a parallel tool-call response group
		default:
			t.Fatalf("tool message at index %d follows a %q message, not an assistant with tool_calls", i, prev.Role)
		}
	}
}

// TestConvertResponseAPIToChatCompletionRequest_ParallelToolCalls reproduces the upstream 400
// "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'".
//
// When an assistant turn issues parallel tool calls, the OpenAI Responses API represents them
// as multiple consecutive function_call items. Converting each into its own assistant message
// breaks the ChatCompletion adjacency rule, because the second tool result ends up following a
// tool message instead of an assistant-with-tool_calls message.
func TestConvertResponseAPIToChatCompletionRequest_ParallelToolCalls(t *testing.T) {
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "deepseek-v4-flash",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Read both files."},
				},
			},
			// Parallel tool calls within a single assistant turn: two consecutive
			// function_call items, no output between them.
			map[string]any{
				"type":      "function_call",
				"id":        "fc_a",
				"call_id":   "call_a",
				"name":      "read_file",
				"arguments": `{"path":"a.txt"}`,
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_b",
				"call_id":   "call_b",
				"name":      "read_file",
				"arguments": `{"path":"b.txt"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_a",
				"output":  "contents of a",
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_b",
				"output":  "contents of b",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	// The two parallel function_call items must collapse into a single assistant message
	// carrying both tool_calls, followed by the two tool results.
	require.Len(t, chatReq.Messages, 4)

	require.Equal(t, "user", chatReq.Messages[0].Role)

	assistant := chatReq.Messages[1]
	require.Equal(t, "assistant", assistant.Role)
	require.Len(t, assistant.ToolCalls, 2,
		"parallel function_call items must be merged into one assistant message")
	// IDs are normalized by convertResponseAPIIDToToolCall (fc_/call_ prefixes stripped),
	// and the function_call_output items normalize to the same values so they still match.
	require.Equal(t, "a", assistant.ToolCalls[0].Id)
	require.Equal(t, "b", assistant.ToolCalls[1].Id)

	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, assistant.ToolCalls[0].Id, chatReq.Messages[2].ToolCallId)
	require.Equal(t, "tool", chatReq.Messages[3].Role)
	require.Equal(t, assistant.ToolCalls[1].Id, chatReq.Messages[3].ToolCallId)

	// The invariant DeepSeek enforces.
	assertToolMessagesFollowToolCalls(t, chatReq.Messages)
}

// TestConvertResponseAPIToChatCompletionRequest_SequentialToolCallsNotMerged guards against
// over-merging: tool calls from distinct turns (separated by their outputs) must remain on
// separate assistant messages.
func TestConvertResponseAPIToChatCompletionRequest_SequentialToolCallsNotMerged(t *testing.T) {
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "deepseek-v4-flash",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "go"}},
			},
			map[string]any{
				"type": "function_call", "id": "fc_1", "call_id": "call_1",
				"name": "step", "arguments": `{"n":1}`,
			},
			map[string]any{
				"type": "function_call_output", "call_id": "call_1", "output": "ok1",
			},
			map[string]any{
				"type": "function_call", "id": "fc_2", "call_id": "call_2",
				"name": "step", "arguments": `{"n":2}`,
			},
			map[string]any{
				"type": "function_call_output", "call_id": "call_2", "output": "ok2",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	// user, assistant(call_1), tool(call_1), assistant(call_2), tool(call_2)
	require.Len(t, chatReq.Messages, 5)
	require.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.Len(t, chatReq.Messages[1].ToolCalls, 1)
	require.Equal(t, "1", chatReq.Messages[1].ToolCalls[0].Id)
	require.Equal(t, "assistant", chatReq.Messages[3].Role)
	require.Len(t, chatReq.Messages[3].ToolCalls, 1)
	require.Equal(t, "2", chatReq.Messages[3].ToolCalls[0].Id)

	assertToolMessagesFollowToolCalls(t, chatReq.Messages)
}

// TestConvertResponseAPIToChatCompletionRequest_OrphanToolOutputDowngraded verifies that a
// function_call_output with no matching in-flight function_call (e.g. trimmed history) is
// downgraded to a user message instead of being forwarded as an invalid tool message.
func TestConvertResponseAPIToChatCompletionRequest_OrphanToolOutputDowngraded(t *testing.T) {
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "deepseek-v4-flash",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "hi"}},
			},
			// No preceding function_call for this output.
			map[string]any{
				"type": "function_call_output", "call_id": "call_ghost", "output": "ghost result",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, "user", chatReq.Messages[0].Role)
	downgraded := chatReq.Messages[1]
	require.Equal(t, "user", downgraded.Role, "orphan tool output must be downgraded to a user message")
	require.Empty(t, downgraded.ToolCallId)
	require.Equal(t, "ghost result", downgraded.StringContent())

	// No `tool` message must be emitted, so the upstream never sees an invalid sequence.
	for _, msg := range chatReq.Messages {
		require.NotEqual(t, "tool", msg.Role)
	}
	assertToolMessagesFollowToolCalls(t, chatReq.Messages)
}

// TestConvertResponseAPIToChatCompletionRequest_MismatchedToolOutputDowngraded verifies that
// when a valid tool output is followed by an extra output with an unknown call_id, the valid
// one stays a tool message and only the stray output is downgraded.
func TestConvertResponseAPIToChatCompletionRequest_MismatchedToolOutputDowngraded(t *testing.T) {
	stream := false
	responseReq := &ResponseAPIRequest{
		Model:  "deepseek-v4-flash",
		Stream: &stream,
		Input: ResponseAPIInput{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "go"}},
			},
			map[string]any{
				"type": "function_call", "id": "fc_a", "call_id": "call_a",
				"name": "tool", "arguments": `{}`,
			},
			map[string]any{
				"type": "function_call_output", "call_id": "call_a", "output": "real",
			},
			// Stray output with no matching in-flight call.
			map[string]any{
				"type": "function_call_output", "call_id": "call_z", "output": "stray",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	// user, assistant(call_a), tool(call_a), user(stray)
	require.Len(t, chatReq.Messages, 4)
	require.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.Len(t, chatReq.Messages[1].ToolCalls, 1)

	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, "real", chatReq.Messages[2].StringContent())

	require.Equal(t, "user", chatReq.Messages[3].Role, "stray tool output must be downgraded")
	require.Empty(t, chatReq.Messages[3].ToolCallId)
	require.Equal(t, "stray", chatReq.Messages[3].StringContent())

	assertToolMessagesFollowToolCalls(t, chatReq.Messages)
}
