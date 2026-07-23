package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
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
