package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole verifies that
// Responses API developer messages become portable ChatCompletion system
// messages for fallback providers that reject role "developer".
func TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole(t *testing.T) {
	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{
				"role":    "developer",
				"content": "Follow repository instructions.",
			},
			map[string]any{
				"role":    "user",
				"content": "Why did the request fail?",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.NotNil(t, chatReq)
	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, "system", chatReq.Messages[0].Role)
	require.Equal(t, "Follow repository instructions.", chatReq.Messages[0].StringContent())
	require.Equal(t, "user", chatReq.Messages[1].Role)
	require.Equal(t, "Why did the request fail?", chatReq.Messages[1].StringContent())
}

// TestConvertResponseAPIToChatCompletionRequestPreservesToolCallReasoning
// reproduces a DeepSeek thinking-mode continuation and verifies that Responses
// reasoning is replayed on the assistant message that issued the tool call.
func TestConvertResponseAPIToChatCompletionRequestPreservesToolCallReasoning(t *testing.T) {
	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{
				"type": "reasoning",
				"content": []any{
					map[string]any{"type": "text", "text": "Inspect the file before answering."},
				},
				"summary": []any{
					map[string]any{"type": "summary_text", "text": "Inspect first."},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_read",
				"call_id":   "call_read",
				"name":      "read_file",
				"arguments": `{"path":"README.md"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_read",
				"output":  "file contents",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)

	assistant := chatReq.Messages[0]
	require.Equal(t, "assistant", assistant.Role)
	require.Len(t, assistant.ToolCalls, 1)
	require.NotNil(t, assistant.ReasoningContent)
	require.Equal(t, "Inspect the file before answering.", *assistant.ReasoningContent)
	require.Equal(t, "", assistant.StringContent(), "DeepSeek requires non-null assistant content on tool-call history")

	toolResult := chatReq.Messages[1]
	require.Equal(t, "tool", toolResult.Role)
	require.Equal(t, assistant.ToolCalls[0].Id, toolResult.ToolCallId)

	reasoningItem := responseReq.Input[0].(map[string]any)
	delete(reasoningItem, "content")
	chatReq, err = ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)
	require.NotNil(t, chatReq.Messages[0].ReasoningContent)
	require.Equal(t, "Inspect first.", *chatReq.Messages[0].ReasoningContent,
		"older summary-only bridge output must remain replayable")
}

// TestConvertResponseAPIToChatCompletionRequestMergesAssistantTextWithToolCalls
// reproduces a DeepSeek thinking-mode turn where the model writes text before
// calling a tool. Responses clients replay it as reasoning, message, and
// function_call items; DeepSeek requires one assistant message carrying the
// text, the tool calls, and the reasoning_content together.
func TestConvertResponseAPIToChatCompletionRequestMergesAssistantTextWithToolCalls(t *testing.T) {
	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"role": "user", "content": "Summarize README.md"},
			map[string]any{
				"type": "reasoning",
				"content": []any{
					map[string]any{"type": "text", "text": "Read the file first."},
				},
			},
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": "Let me read it."},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_read",
				"call_id":   "call_read",
				"name":      "read_file",
				"arguments": `{"path":"README.md"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_read",
				"output":  "file contents",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)

	assistant := chatReq.Messages[1]
	require.Equal(t, "assistant", assistant.Role)
	require.Equal(t, "Let me read it.", assistant.StringContent())
	require.Len(t, assistant.ToolCalls, 1)
	require.NotNil(t, assistant.ReasoningContent)
	require.Equal(t, "Read the file first.", *assistant.ReasoningContent)

	toolResult := chatReq.Messages[2]
	require.Equal(t, "tool", toolResult.Role)
	require.Equal(t, assistant.ToolCalls[0].Id, toolResult.ToolCallId)
}

// TestConvertResponseAPIToChatCompletionRequestPreservesDeepSeekFileImages verifies
// Responses API file-backed images survive fallback conversion to Chat Completions.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when file image content is dropped.
func TestConvertResponseAPIToChatCompletionRequestPreservesDeepSeekFileImages(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]map[string]string{
		"file_id":   {"file_id": "file-api-123"},
		"file_data": {"file_data": "data:image/png;base64,AAAA", "filename": "image.png"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := &ResponseAPIRequest{
				Model: "deepseek-v4-flash-vision-exp",
				Input: ResponseAPIInput{
					map[string]any{
						"role": "user",
						"content": []any{
							map[string]any{"type": "input_text", "text": "describe"},
							map[string]any{"type": "input_image"},
						},
					},
				},
			}
			content := request.Input[0].(map[string]any)["content"].([]any)
			for key, value := range source {
				content[1].(map[string]any)[key] = value
			}

			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.NoError(t, err)
			require.Len(t, converted.Messages, 1)
			blocks, ok := converted.Messages[0].Content.([]model.MessageContent)
			require.True(t, ok)
			require.Len(t, blocks, 2)
			require.Equal(t, model.ContentTypeText, blocks[0].Type)
			require.Equal(t, model.ContentTypeFile, blocks[1].Type)
			require.Equal(t, source["file_id"], blocks[1].FileID)
			require.Equal(t, source["file_data"], blocks[1].FileData)
			require.Equal(t, source["filename"], blocks[1].Filename)
		})
	}
}

// TestConvertResponseAPIToChatCompletionRequestMergesTurnAcrossReasoningItem
// replays an assistant turn in the order this gateway's own response.output
// emits it (message, reasoning, function_call). A reasoning item is an
// intra-turn marker, so it must not split the turn into two assistant messages.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the turn is split.
func TestConvertResponseAPIToChatCompletionRequestMergesTurnAcrossReasoningItem(t *testing.T) {
	t.Parallel()
	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"role": "user", "content": "Summarize README.md"},
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": "Let me read it."},
				},
			},
			map[string]any{
				"type": "reasoning",
				"content": []any{
					map[string]any{"type": "text", "text": "Read the file first."},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        "fc_read",
				"call_id":   "call_read",
				"name":      "read_file",
				"arguments": `{"path":"README.md"}`,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_read",
				"output":  "file contents",
			},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3, "reasoning between message and function_call must not split the turn")

	assistant := chatReq.Messages[1]
	require.Equal(t, "assistant", assistant.Role)
	require.Equal(t, "Let me read it.", assistant.StringContent())
	require.Len(t, assistant.ToolCalls, 1)
	require.NotNil(t, assistant.ReasoningContent)
	require.Equal(t, "Read the file first.", *assistant.ReasoningContent)

	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, assistant.ToolCalls[0].Id, chatReq.Messages[2].ToolCallId)
}

// TestConvertResponseAPIToChatCompletionRequestKeepsMergedAssistantContentNonNull
// covers assistant message items whose content decodes to nil. Merging tool calls
// into such a message must not emit an assistant message without a content field,
// because `json:"content,omitempty"` drops nil and DeepSeek rejects tool-call
// history whose assistant content is absent.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when content is dropped.
func TestConvertResponseAPIToChatCompletionRequestKeepsMergedAssistantContentNonNull(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]any{
		"empty_content_array":     []any{},
		"unrecognized_content":    []any{map[string]any{"type": "refusal", "refusal": "no"}},
		"missing_content_entries": []any{map[string]any{"type": "output_text"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			responseReq := &ResponseAPIRequest{
				Model: "deepseek-v4-pro",
				Input: ResponseAPIInput{
					map[string]any{"type": "message", "role": "assistant", "content": content},
					map[string]any{
						"type":      "function_call",
						"id":        "fc_x",
						"call_id":   "call_x",
						"name":      "read_file",
						"arguments": `{}`,
					},
				},
			}

			chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
			require.NoError(t, err)
			require.Len(t, chatReq.Messages, 1)
			assistant := chatReq.Messages[0]
			require.Len(t, assistant.ToolCalls, 1)
			require.NotNil(t, assistant.Content,
				"an assistant message carrying tool_calls must serialize a content field")

			encoded, err := json.Marshal(assistant)
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"content"`)
		})
	}
}

// TestConvertResponseAPIToChatCompletionRequestAttachesTrailingReasoningToItsTurn
// covers the item order this gateway's own bridge produces. buildFinalResponse
// appends message, reasoning, function_call in that order, so a reasoning item
// arrives *after* the assistant message it belongs to. Staging it for the next
// message instead dropped the thinking of a text-only turn entirely, and DeepSeek
// answers a trailing assistant message with no reasoning_content with
// "The `reasoning_content` in the thinking mode must be passed back to the API"
// (verified against api.deepseek.com with deepseek-flash on 2026-09-18).
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the reasoning is lost.
func TestConvertResponseAPIToChatCompletionRequestAttachesTrailingReasoningToItsTurn(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "weather in Paris?"}}},
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "It is 18C."}}},
			map[string]any{"type": "reasoning", "status": "completed",
				"content": []any{map[string]any{"type": "text", "text": "Report the tool result."}}},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)

	assistant := chatReq.Messages[1]
	require.Equal(t, "assistant", assistant.Role)
	require.Equal(t, "It is 18C.", assistant.StringContent())
	require.NotNil(t, assistant.ReasoningContent,
		"a reasoning item following its assistant message must attach to that message")
	require.Equal(t, "Report the tool result.", *assistant.ReasoningContent)
}

// TestConvertResponseAPIToChatCompletionRequestKeepsReasoningWithinItsOwnTurn
// guards against cross-turn bleed. With the bridge's message-then-reasoning order,
// carrying reasoning forward stamped turn N's thinking onto turn N+1's assistant
// message and dropped the last turn's thinking; each turn must keep its own.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when thinking crosses a turn.
func TestConvertResponseAPIToChatCompletionRequestKeepsReasoningWithinItsOwnTurn(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "weather in Paris?"}}},
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Let me check."}}},
			map[string]any{"type": "reasoning", "status": "completed",
				"content": []any{map[string]any{"type": "text", "text": "first thinking"}}},
			map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1",
				"name": "get_weather", "arguments": `{"city":"Paris"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": `{"temp_c":18}`},
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "It is 18C."}}},
			map[string]any{"type": "reasoning", "status": "completed",
				"content": []any{map[string]any{"type": "text", "text": "second thinking"}}},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 4)

	firstTurn := chatReq.Messages[1]
	require.Len(t, firstTurn.ToolCalls, 1)
	require.NotNil(t, firstTurn.ReasoningContent)
	require.Equal(t, "first thinking", *firstTurn.ReasoningContent)

	require.Equal(t, "tool", chatReq.Messages[2].Role)

	secondTurn := chatReq.Messages[3]
	require.Equal(t, "It is 18C.", secondTurn.StringContent())
	require.NotNil(t, secondTurn.ReasoningContent)
	require.Equal(t, "second thinking", *secondTurn.ReasoningContent)
}

// TestConvertResponseAPIToChatCompletionRequestJoinsRepeatedReasoningItems covers a
// turn that emits several reasoning items: all of them belong to the same assistant
// message, so they are joined rather than overwriting one another.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when an item is lost.
func TestConvertResponseAPIToChatCompletionRequestJoinsRepeatedReasoningItems(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Working."}}},
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "text", "text": "step one"}}},
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "text", "text": "step two"}}},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 1)
	require.NotNil(t, chatReq.Messages[0].ReasoningContent)
	require.Equal(t, "step one\nstep two", *chatReq.Messages[0].ReasoningContent)

	// The same turn in the other producer order: both items precede the message
	// they belong to, so neither may overwrite the other while staged.
	staged := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "text", "text": "step one"}}},
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "text", "text": "step two"}}},
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Working."}}},
		},
	}

	stagedReq, err := ConvertResponseAPIToChatCompletionRequest(staged)
	require.NoError(t, err)
	require.Len(t, stagedReq.Messages, 1)
	require.NotNil(t, stagedReq.Messages[0].ReasoningContent)
	require.Equal(t, "step one\nstep two", *stagedReq.Messages[0].ReasoningContent)
}

// TestConvertResponseAPIToChatCompletionRequestKeepsStagedReasoningForOpenAIOrder
// pins the other producer order: OpenAI emits the reasoning item before the
// assistant message, so it must still be staged forward and land on that message.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the staged reasoning is lost.
func TestConvertResponseAPIToChatCompletionRequestKeepsStagedReasoningForOpenAIOrder(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "weather?"}}},
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "text", "text": "think first"}}},
			map[string]any{"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Let me check."}}},
			map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1",
				"name": "get_weather", "arguments": `{"city":"Paris"}`},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)

	assistant := chatReq.Messages[1]
	require.Len(t, assistant.ToolCalls, 1)
	require.NotNil(t, assistant.ReasoningContent)
	require.Equal(t, "think first", *assistant.ReasoningContent)
}

// TestConvertResponseAPIToChatCompletionRequestDisambiguatesRepeatedCallIDs covers
// the client bug this gateway triggered before function_call items carried call_id
// on response.output_item.added: a client that takes the call identity from the
// added event records every parallel call as the same placeholder. Repeating an id
// inside one tool_calls array is rejected upstream with "Duplicate value for
// 'tool_call_id' of undefined in message[1]", and the second output used to be
// silently downgraded to a user message (verified against api.deepseek.com with
// deepseek-flash on 2026-09-18).
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when ids collide or an output is lost.
func TestConvertResponseAPIToChatCompletionRequestDisambiguatesRepeatedCallIDs(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "weather?"}}},
			map[string]any{"type": "function_call", "call_id": "undefined",
				"name": "get_weather", "arguments": `{"city":"Paris"}`},
			map[string]any{"type": "function_call", "call_id": "undefined",
				"name": "get_weather", "arguments": `{"city":"Lyon"}`},
			map[string]any{"type": "function_call_output", "call_id": "undefined", "output": "18"},
			map[string]any{"type": "function_call_output", "call_id": "undefined", "output": "22"},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 4, "both outputs must survive as tool messages")

	assistant := chatReq.Messages[1]
	require.Len(t, assistant.ToolCalls, 2)
	require.NotEqual(t, assistant.ToolCalls[0].Id, assistant.ToolCalls[1].Id,
		"one assistant message must not repeat a tool_call_id")
	require.Equal(t, "undefined", assistant.ToolCalls[0].Id,
		"the first call keeps the id the client sent")

	// Outputs pair with calls in arrival order, so the Paris call keeps the Paris result.
	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, assistant.ToolCalls[0].Id, chatReq.Messages[2].ToolCallId)
	require.Equal(t, "18", chatReq.Messages[2].StringContent())
	require.Equal(t, "tool", chatReq.Messages[3].Role)
	require.Equal(t, assistant.ToolCalls[1].Id, chatReq.Messages[3].ToolCallId)
	require.Equal(t, "22", chatReq.Messages[3].StringContent())
}

// TestConvertResponseAPIToChatCompletionRequestKeepsToolCallAnswerableAcrossReasoning
// covers a producer that emits a reasoning item between a function_call and its
// output. A reasoning item lowers to no chat message, so the tool result is still
// adjacent to the assistant message carrying the call and must stay a `tool`
// message; downgrading it to a user message would leave the call unanswered, which
// upstream rejects with "An assistant message with 'tool_calls' must be followed by
// tool messages responding to each 'tool_call_id'".
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the answer is orphaned.
func TestConvertResponseAPIToChatCompletionRequestKeepsToolCallAnswerableAcrossReasoning(t *testing.T) {
	t.Parallel()

	responseReq := &ResponseAPIRequest{
		Model: "deepseek-v4-pro",
		Input: ResponseAPIInput{
			map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "weather?"}}},
			map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1",
				"name": "get_weather", "arguments": `{"city":"Paris"}`},
			map[string]any{"type": "reasoning",
				"content": []any{map[string]any{"type": "text", "text": "waiting for the tool"}}},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "18"},
		},
	}

	chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseReq)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)

	assistant := chatReq.Messages[1]
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, "tool", chatReq.Messages[2].Role,
		"a reasoning item lowers to no message, so it cannot orphan the tool result")
	require.Equal(t, assistant.ToolCalls[0].Id, chatReq.Messages[2].ToolCallId)
}
