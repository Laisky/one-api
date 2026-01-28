package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestGetAndValidateClaudeMessagesRequest(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		requestBody string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			requestBody: `{
				"model": "claude-3-sonnet-20240229",
				"max_tokens": 1024,
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "missing model",
			requestBody: `{
				"max_tokens": 1024,
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "model is required",
		},
		{
			name: "missing max_tokens",
			requestBody: `{
				"model": "claude-3-sonnet-20240229",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "max_tokens must be greater than 0",
		},
		{
			name: "empty messages",
			requestBody: `{
				"model": "claude-3-sonnet-20240229",
				"max_tokens": 1024,
				"messages": []
			}`,
			expectError: true,
			errorMsg:    "messages array cannot be empty",
		},
		{
			name: "invalid message role",
			requestBody: `{
				"model": "claude-3-sonnet-20240229",
				"max_tokens": 1024,
				"messages": [
					{
						"role": "system",
						"content": "Hello"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "message[0].role must be 'user' or 'assistant'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set up request
			req, _ := http.NewRequest("POST", "/v1/messages", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			// Call the function
			result, err := getAndValidateClaudeMessagesRequest(c)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "claude-3-sonnet-20240229", result.Model)
				assert.Equal(t, 1024, result.MaxTokens)
				assert.Len(t, result.Messages, 1)
			}
		})
	}
}

func TestBuildClaudeToolsForMCP(t *testing.T) {
	req := &relaymodel.ClaudeRequest{
		Tools: []relaymodel.ClaudeTool{
			{Type: "web_search_20250305", Name: "web_search"},
			{
				Name:        "local_tool",
				Description: "Local tool",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	tools := buildClaudeToolsForMCP(req)
	require.Len(t, tools, 2)
	require.Equal(t, "web_search_20250305", tools[0].Type)
	require.Nil(t, tools[0].Function)
	require.Equal(t, "function", tools[1].Type)
	require.NotNil(t, tools[1].Function)
	require.Equal(t, "local_tool", tools[1].Function.Name)
}

func TestGetClaudeMessagesPromptTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		request  *ClaudeMessagesRequest
		expected int
	}{
		{
			name: "simple text message",
			request: &ClaudeMessagesRequest{
				Model: "gpt-3.5-turbo",
				Messages: []relaymodel.ClaudeMessage{
					{
						Role:    "user",
						Content: "Hello, how are you today?",
					},
				},
			},
			expected: 16,
		},
		{
			name: "multiple messages",
			request: &ClaudeMessagesRequest{
				Model: "gpt-3.5-turbo",
				Messages: []relaymodel.ClaudeMessage{
					{
						Role:    "user",
						Content: "Hello",
					},
					{
						Role:    "assistant",
						Content: "Hi there!",
					},
				},
			},
			expected: 17,
		},
		{
			name: "with system prompt",
			request: &ClaudeMessagesRequest{
				Model:  "gpt-3.5-turbo",
				System: "You are a helpful assistant.",
				Messages: []relaymodel.ClaudeMessage{
					{
						Role:    "user",
						Content: "Hello",
					},
				},
			},
			expected: 23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getClaudeMessagesPromptTokens(ctx, tt.request)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClaudeMessagesRelayMode(t *testing.T) {
	t.Parallel()
	// Test that the relay mode is correctly detected for /v1/messages
	mode := relaymode.GetByPath("/v1/messages")
	assert.Equal(t, relaymode.ClaudeMessages, mode)

	// Test other paths are not affected
	assert.Equal(t, relaymode.ChatCompletions, relaymode.GetByPath("/v1/chat/completions"))
	assert.Equal(t, relaymode.ResponseAPI, relaymode.GetByPath("/v1/responses"))
}

func TestClaudeMessagesRequestStructure(t *testing.T) {
	t.Parallel()
	// Test that the Claude Messages request structure can be properly marshaled/unmarshaled
	originalRequest := &ClaudeMessagesRequest{
		Model:     "claude-3-sonnet-20240229",
		MaxTokens: 1024,
		Messages: []relaymodel.ClaudeMessage{
			{
				Role:    "user",
				Content: "Hello, how are you?",
			},
			{
				Role:    "assistant",
				Content: "I'm doing well, thank you!",
			},
		},
		System:      "You are a helpful assistant.",
		Temperature: func() *float64 { f := 0.7; return &f }(),
		Stream:      func() *bool { b := true; return &b }(),
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "get_weather",
				Description: "Get the current weather",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The city and state",
						},
					},
				},
			},
		},
		ToolChoice: map[string]any{
			"type": "auto",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(originalRequest)
	require.NoError(t, err)

	// Unmarshal back
	var parsedRequest ClaudeMessagesRequest
	err = json.Unmarshal(jsonData, &parsedRequest)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, originalRequest.Model, parsedRequest.Model)
	assert.Equal(t, originalRequest.MaxTokens, parsedRequest.MaxTokens)
	assert.Len(t, parsedRequest.Messages, 2)
	assert.Equal(t, "user", parsedRequest.Messages[0].Role)
	assert.Equal(t, "assistant", parsedRequest.Messages[1].Role)
	assert.NotNil(t, parsedRequest.Temperature)
	assert.Equal(t, 0.7, *parsedRequest.Temperature)
	assert.NotNil(t, parsedRequest.Stream)
	assert.True(t, *parsedRequest.Stream)
	assert.Len(t, parsedRequest.Tools, 1)
	assert.Equal(t, "get_weather", parsedRequest.Tools[0].Name)
}
