package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseAPIInputUnmarshaling(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected ResponseAPIInput
	}{
		{
			name:     "String input",
			jsonData: `{"input": "Write a one-sentence bedtime story about a unicorn."}`,
			expected: ResponseAPIInput{"Write a one-sentence bedtime story about a unicorn."},
		},
		{
			name:     "Array input with single string",
			jsonData: `{"input": ["Hello world"]}`,
			expected: ResponseAPIInput{"Hello world"},
		},
		{
			name:     "Array input with message object",
			jsonData: `{"input": [{"role": "user", "content": "Hello"}]}`,
			expected: ResponseAPIInput{map[string]any{
				"role":    "user",
				"content": "Hello",
			}},
		},
		{
			name:     "Array input with multiple items",
			jsonData: `{"input": ["Hello", {"role": "user", "content": "World"}]}`,
			expected: ResponseAPIInput{
				"Hello",
				map[string]any{
					"role":    "user",
					"content": "World",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request struct {
				Input ResponseAPIInput `json:"input"`
			}

			err := json.Unmarshal([]byte(tt.jsonData), &request)
			require.NoError(t, err, "Failed to unmarshal JSON")
			require.Len(t, request.Input, len(tt.expected), "Input length mismatch")

			for i, expectedItem := range tt.expected {
				actualItem := request.Input[i]

				// For string comparison
				if expectedStr, ok := expectedItem.(string); ok {
					actualStr, ok := actualItem.(string)
					require.True(t, ok, "Expected input[%d] to be string, got %T", i, actualItem)
					require.Equal(t, expectedStr, actualStr, "input[%d] mismatch", i)
					continue
				}

				// For map comparison (simplified)
				if expectedMap, ok := expectedItem.(map[string]any); ok {
					actualMap, ok := actualItem.(map[string]any)
					require.True(t, ok, "Expected input[%d] to be map, got %T", i, actualItem)
					for key, expectedValue := range expectedMap {
						actualValue, exists := actualMap[key]
						require.True(t, exists, "Expected key %s to exist in input[%d]", key, i)
						require.Equal(t, expectedValue, actualValue, "input[%d][%s] mismatch", i, key)
					}
				}
			}
		})
	}
}

func TestResponseAPIRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid string input",
			jsonData: `{
				"model": "gpt-4o-mini",
				"input": "Write a story"
			}`,
			expectError: false,
		},
		{
			name: "Valid array input",
			jsonData: `{
				"model": "gpt-4o-mini",
				"input": [{"role": "user", "content": "Write a story"}]
			}`,
			expectError: false,
		},
		{
			name: "Valid prompt input",
			jsonData: `{
				"model": "gpt-4o-mini",
				"prompt": {
					"id": "pmpt_123",
					"variables": {"name": "John"}
				}
			}`,
			expectError: false,
		},
		{
			name: "Invalid - both input and prompt",
			jsonData: `{
				"model": "gpt-4o-mini",
				"input": "Write a story",
				"prompt": {"id": "pmpt_123"}
			}`,
			expectError: false, // JSON unmarshaling will succeed, validation happens later
		},
		{
			name: "Invalid - neither input nor prompt",
			jsonData: `{
				"model": "gpt-4o-mini"
			}`,
			expectError: false, // JSON unmarshaling will succeed, validation happens later
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request ResponseAPIRequest
			err := json.Unmarshal([]byte(tt.jsonData), &request)

			if tt.expectError {
				require.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
				// Basic validation that the struct was populated correctly
				require.NotEmpty(t, request.Model, "Model should not be empty")
			}
		})
	}
}
