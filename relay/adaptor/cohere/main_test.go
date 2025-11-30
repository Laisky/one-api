package cohere

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// TestConvertRequest_ContentTypeHandling tests that ConvertRequest correctly handles
// different Content types without panicking. This is a regression test for the unsafe
// type assertion that used message.Content.(string) which would panic if Content was
// not a string.
func TestConvertRequest_ContentTypeHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		request       model.GeneralOpenAIRequest
		expectMessage string
		expectPanic   bool
	}{
		{
			name: "string content should work",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "user",
						Content: "Hello, world!",
					},
				},
			},
			expectMessage: "Hello, world!",
			expectPanic:   false,
		},
		{
			name: "empty string content should work",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "user",
						Content: "",
					},
				},
			},
			expectMessage: "",
			expectPanic:   false,
		},
		{
			name: "nil content should not panic via StringContent",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "user",
						Content: nil,
					},
				},
			},
			expectMessage: "",
			expectPanic:   false,
		},
		{
			name: "structured content array should not panic via StringContent",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role: "user",
						// Content as []any (mimics JSON parsing) with text type
						Content: []any{
							map[string]any{
								"type": "text",
								"text": "structured text",
							},
						},
					},
				},
			},
			// StringContent() handles []any and extracts text from "text" type content
			expectMessage: "structured text",
			expectPanic:   false,
		},
		{
			name: "assistant message with string content",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "user",
						Content: "Hello",
					},
					{
						Role:    "assistant",
						Content: "Hi there!",
					},
					{
						Role:    "user",
						Content: "How are you?",
					},
				},
			},
			expectMessage: "How are you?",
			expectPanic:   false,
		},
		{
			name: "system message with string content",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "system",
						Content: "You are a helpful assistant.",
					},
					{
						Role:    "user",
						Content: "Hello",
					},
				},
			},
			expectMessage: "Hello",
			expectPanic:   false,
		},
		{
			name: "multiple messages with mixed roles",
			request: model.GeneralOpenAIRequest{
				Model: "command-r",
				Messages: []model.Message{
					{
						Role:    "system",
						Content: "Be helpful",
					},
					{
						Role:    "user",
						Content: "Question 1",
					},
					{
						Role:    "assistant",
						Content: "Answer 1",
					},
					{
						Role:    "user",
						Content: "Final question",
					},
				},
			},
			expectMessage: "Final question",
			expectPanic:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use defer/recover to catch any panics
			defer func() {
				if r := recover(); r != nil {
					if !tt.expectPanic {
						require.Failf(t, "ConvertRequest panicked unexpectedly", "%v", r)
					}
				}
			}()

			// This should not panic with the fix applied
			result := ConvertRequest(tt.request)

			if tt.expectPanic {
				require.Fail(t, "Expected panic but none occurred")
				return
			}

			require.NotNil(t, result, "Result should not be nil")
			require.Equal(t, tt.expectMessage, result.Message,
				"Message should be extracted correctly via StringContent")
		})
	}
}

// TestConvertRequest_ChatHistory tests that chat history is correctly populated
// for non-user messages. Note: The last user message becomes the main "Message"
// field, and all non-last messages go to ChatHistory.
func TestConvertRequest_ChatHistory(t *testing.T) {
	t.Parallel()

	request := model.GeneralOpenAIRequest{
		Model: "command-r",
		Messages: []model.Message{
			{
				Role:    "system",
				Content: "You are helpful",
			},
			{
				Role:    "user",
				Content: "First message",
			},
			{
				Role:    "assistant",
				Content: "First response",
			},
			{
				Role:    "user",
				Content: "Second message",
			},
		},
	}

	result := ConvertRequest(request)

	require.NotNil(t, result)
	require.Equal(t, "Second message", result.Message, "Last user message should be the main message")
	// Cohere's ConvertRequest puts system, first user, and assistant messages in ChatHistory
	// The last user message becomes the main Message field
	require.Len(t, result.ChatHistory, 2, "Should have 2 items in chat history (system + assistant)")

	// Verify chat history contents (excluding non-final user messages based on Cohere logic)
	require.Equal(t, "SYSTEM", result.ChatHistory[0].Role)
	require.Equal(t, "You are helpful", result.ChatHistory[0].Message)

	require.Equal(t, "CHATBOT", result.ChatHistory[1].Role)
	require.Equal(t, "First response", result.ChatHistory[1].Message)
}

// TestConvertRequest_InternetSuffix tests that the -internet suffix is handled correctly.
func TestConvertRequest_InternetSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		model           string
		expectedModel   string
		expectConnector bool
	}{
		{
			name:            "model without internet suffix",
			model:           "command-r",
			expectedModel:   "command-r",
			expectConnector: false,
		},
		{
			name:            "model with internet suffix",
			model:           "command-r-internet",
			expectedModel:   "command-r",
			expectConnector: true,
		},
		{
			name:            "another model with internet suffix",
			model:           "command-light-internet",
			expectedModel:   "command-light",
			expectConnector: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := model.GeneralOpenAIRequest{
				Model: tt.model,
				Messages: []model.Message{
					{
						Role:    "user",
						Content: "test",
					},
				},
			}

			result := ConvertRequest(request)

			require.Equal(t, tt.expectedModel, result.Model)

			if tt.expectConnector {
				require.Len(t, result.Connectors, 1)
				require.Equal(t, "web-search", result.Connectors[0].ID)
			} else {
				require.Empty(t, result.Connectors)
			}
		})
	}
}

// TestConvertRequest_EmptyMessages tests edge case with no messages.
func TestConvertRequest_EmptyMessages(t *testing.T) {
	t.Parallel()

	request := model.GeneralOpenAIRequest{
		Model:    "command-r",
		Messages: []model.Message{},
	}

	// This should not panic
	result := ConvertRequest(request)

	require.NotNil(t, result)
	require.Equal(t, "", result.Message)
	require.Empty(t, result.ChatHistory)
}
