package openai

import (
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
