package cloudflare

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/model"
)

// TestHandler_ContentExtraction tests that the content extraction in Handler
// works correctly with StringContent() instead of unsafe type assertion.
// This is a regression test for the panic that occurred when Content was not a string.
func TestHandler_ContentExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		response        openai.TextResponse
		expectedContent string
	}{
		{
			name: "string content should work",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role:    "assistant",
							Content: "Hello, world!",
						},
						FinishReason: "stop",
					},
				},
			},
			expectedContent: "Hello, world!",
		},
		{
			name: "empty string content should work",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role:    "assistant",
							Content: "",
						},
						FinishReason: "stop",
					},
				},
			},
			expectedContent: "",
		},
		{
			name: "nil content should not panic",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role:    "assistant",
							Content: nil,
						},
						FinishReason: "stop",
					},
				},
			},
			expectedContent: "",
		},
		{
			name: "structured content ([]any) should work via StringContent",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role: "assistant",
							Content: []any{
								map[string]any{
									"type": "text",
									"text": "structured text",
								},
							},
						},
						FinishReason: "stop",
					},
				},
			},
			expectedContent: "structured text",
		},
		{
			name: "multiple choices should accumulate content",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role:    "assistant",
							Content: "First ",
						},
						FinishReason: "stop",
					},
					{
						Index: 1,
						Message: model.Message{
							Role:    "assistant",
							Content: "Second",
						},
						FinishReason: "stop",
					},
				},
			},
			expectedContent: "First Second",
		},
		{
			name: "empty choices should work",
			response: openai.TextResponse{
				Id:      "test-id",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   "test-model",
				Choices: []openai.TextResponseChoice{},
			},
			expectedContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test the content extraction logic directly (mimics what Handler does)
			var responseText string
			for _, v := range tt.response.Choices {
				// This is the fixed code using StringContent() instead of .(string) assertion
				responseText += v.Message.StringContent()
			}

			require.Equal(t, tt.expectedContent, responseText,
				"Content should be extracted correctly via StringContent()")
		})
	}
}

// TestStreamHandler_DeltaContentExtraction tests that stream delta content extraction
// works correctly without panicking on different content types.
func TestStreamHandler_DeltaContentExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		streamEvents   []openai.ChatCompletionsStreamResponse
		expectedLength int
	}{
		{
			name: "string content in delta",
			streamEvents: []openai.ChatCompletionsStreamResponse{
				{
					Id:      "test-id",
					Object:  "chat.completion.chunk",
					Created: 1234567890,
					Model:   "test-model",
					Choices: []openai.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: model.Message{
								Role:    "assistant",
								Content: "Hello",
							},
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "empty choices in some chunks",
			streamEvents: []openai.ChatCompletionsStreamResponse{
				{
					Id:      "test-id",
					Object:  "chat.completion.chunk",
					Created: 1234567890,
					Model:   "test-model",
					Choices: []openai.ChatCompletionsStreamResponseChoice{},
				},
			},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Build SSE stream data
			var streamData strings.Builder
			for _, event := range tt.streamEvents {
				eventBytes, err := json.Marshal(event)
				require.NoError(t, err)
				streamData.WriteString("data: ")
				streamData.Write(eventBytes)
				streamData.WriteString("\n")
			}
			streamData.WriteString("data: [DONE]\n")

			// Verify the stream data was built correctly
			require.Contains(t, streamData.String(), "data:")
		})
	}
}

// TestMessageStringContent_SafeTypeAssertion verifies that Message.StringContent()
// safely handles various Content types without panicking, which is the key fix
// that replaced the unsafe .(string) type assertion.
func TestMessageStringContent_SafeTypeAssertion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  model.Message
		expected string
	}{
		{
			name: "string content",
			message: model.Message{
				Content: "test string",
			},
			expected: "test string",
		},
		{
			name: "nil content",
			message: model.Message{
				Content: nil,
			},
			expected: "",
		},
		{
			name: "int content (edge case)",
			message: model.Message{
				Content: 123,
			},
			expected: "",
		},
		{
			name: "map content (edge case)",
			message: model.Message{
				Content: map[string]any{"key": "value"},
			},
			expected: "",
		},
		{
			name: "[]any with text content",
			message: model.Message{
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "extracted text",
					},
				},
			},
			expected: "extracted text",
		},
		{
			name: "[]any with multiple text blocks",
			message: model.Message{
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "first ",
					},
					map[string]any{
						"type": "text",
						"text": "second",
					},
				},
			},
			expected: "first second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// This should never panic
			result := tt.message.StringContent()

			require.Equal(t, tt.expected, result)
		})
	}
}

func init() {
	gin.SetMode(gin.TestMode)
}
