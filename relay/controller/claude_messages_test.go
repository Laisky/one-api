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

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	awsclaude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	vertexclaude "github.com/Laisky/one-api/relay/adaptor/vertexai/claude"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMergeThenAnthropicConvert_NoSystemRoleLeaks proves the AWS Bedrock / Vertex
// path (which rebuilds the request via anthropic.ConvertClaudeRequest, NOT raw-body
// passthrough) is fixed by the central in-place merge: after merging, the typed
// Anthropic payload contains zero system-role messages, the head system is left
// UNCHANGED (cache-preserving), and the mid-array system text survives inside an
// adjacent turn.
func TestMergeThenAnthropicConvert_NoSystemRoleLeaks(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-8",
		MaxTokens: 256,
		System:    "HEAD",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "u1"},
			{Role: "system", Content: "MID1"},
			{Role: "assistant", Content: "a1"},
			{Role: "user", Content: "u2"},
			{Role: "system", Content: "MID2"},
		},
	}

	mergeMidArraySystemMessages(req)
	// Head system is untouched -> the cacheable prefix survives.
	require.Equal(t, "HEAD", req.System)

	converted, err := anthropic.ConvertClaudeRequest(c, *req)
	require.NoError(t, err)
	require.NotNil(t, converted)

	b, err := json.Marshal(converted)
	require.NoError(t, err)
	js := string(b)
	// Head system carries ONLY the original head text (not the mid-array text).
	require.Contains(t, js, `"system":"HEAD"`)
	// Zero system-role messages leak into the typed Bedrock/Vertex payload.
	require.NotContains(t, js, `"role":"system"`)
	// The mid-array system instructions survive inside adjacent turns.
	require.Contains(t, js, "MID1")
	require.Contains(t, js, "MID2")
}

// TestMergeThenNativeClaudeRebuiltPayload_PreservesSignedThinking proves that
// AWS Bedrock and Vertex-style rebuilt Claude payloads keep signed assistant
// thinking blocks untouched when a mid-array system message precedes them.
func TestMergeThenNativeClaudeRebuiltPayload_PreservesSignedThinking(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	signature := "ErQCsignedThinkingPayload=="
	req := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-8",
		MaxTokens: 256,
		System:    "HEAD",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "u1"},
			{Role: "system", Content: "MID-before-signed-assistant"},
			{Role: "assistant", Content: []any{
				map[string]any{"type": "thinking", "thinking": "private chain", "signature": signature},
				map[string]any{"type": "text", "text": "a1"},
			}},
		},
	}

	mergeMidArraySystemMessages(req)
	require.Len(t, req.Messages, 3)
	require.Equal(t, []string{"user", "user", "assistant"}, []string{req.Messages[0].Role, req.Messages[1].Role, req.Messages[2].Role})
	require.Equal(t, "MID-before-signed-assistant", req.Messages[1].Content)

	converted, err := anthropic.ConvertClaudeRequest(c, *req)
	require.NoError(t, err)
	require.NotNil(t, converted)
	require.Len(t, converted.Messages, 3)
	require.Equal(t, "assistant", converted.Messages[2].Role)
	require.Len(t, converted.Messages[2].Content, 2)
	require.Equal(t, "thinking", converted.Messages[2].Content[0].Type)
	require.NotNil(t, converted.Messages[2].Content[0].Thinking)
	require.NotNil(t, converted.Messages[2].Content[0].Signature)
	require.Equal(t, signature, *converted.Messages[2].Content[0].Signature)
	require.NotContains(t, *converted.Messages[2].Content[0].Thinking, "MID-before-signed-assistant")
	require.NotContains(t, converted.Messages[2].Content[1].Text, "MID-before-signed-assistant")

	awsPayload := awsclaude.Request{
		AnthropicVersion: "bedrock-2023-05-31",
		Messages:         converted.Messages,
		System:           converted.System,
		MaxTokens:        converted.MaxTokens,
		Thinking:         converted.Thinking,
	}
	vertexPayload := vertexclaude.Request{
		AnthropicVersion: "vertex-2023-10-16",
		Messages:         converted.Messages,
		System:           converted.System,
		MaxTokens:        converted.MaxTokens,
	}

	for name, payload := range map[string]any{"aws": awsPayload, "vertex": vertexPayload} {
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err, name)
		payloadJSON := string(payloadBytes)
		require.NotContains(t, payloadJSON, `"role":"system"`, name)
		require.Contains(t, payloadJSON, `"signature":"`+signature+`"`, name)
		require.Contains(t, payloadJSON, "MID-before-signed-assistant", name)
	}
}

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
						"role": "function",
						"content": "Hello"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "message[0].role must be 'user', 'assistant', or 'system'",
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

// TestGetAndValidateClaudeMessagesRequest_MidArraySystemAllowed reproduces
// one-api issue #350: Claude Code v2.1.154+ (e.g. "adaptive thinking") emits
// role:"system" messages INSIDE the messages array. one-api must tolerate them
// instead of rejecting the request with
// "message[i].role must be 'user' or 'assistant'".
func TestGetAndValidateClaudeMessagesRequest_MidArraySystemAllowed(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	requestBody := `{
		"model": "claude-3-sonnet-20240229",
		"max_tokens": 1024,
		"system": "You are Claude Code.",
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "system", "content": "Adaptive thinking guidance injected mid-conversation."},
			{"role": "assistant", "content": "Hi!"}
		]
	}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest("POST", "/v1/messages", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	result, err := getAndValidateClaudeMessagesRequest(c)
	require.NoError(t, err, "mid-array system message must be tolerated (issue #350)")
	require.NotNil(t, result)
	assert.Len(t, result.Messages, 3)
}

// TestGetAndValidateClaudeMessagesRequest_RejectsUnknownRole guards that
// genuinely invalid roles are still rejected after #350 relaxes the system role.
func TestGetAndValidateClaudeMessagesRequest_RejectsUnknownRole(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	requestBody := `{
		"model": "claude-3-sonnet-20240229",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "tool", "content": "nope"}
		]
	}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest("POST", "/v1/messages", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	result, err := getAndValidateClaudeMessagesRequest(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message[1].role")
	assert.Nil(t, result)
}

func TestBuildClaudeToolsForMCP(t *testing.T) {
	req := &relaymodel.ClaudeRequest{
		Tools: []relaymodel.ClaudeTool{
			{Type: "web_search_20260209", Name: "web_search"},
			{
				Name:        "local_tool",
				Description: "Local tool",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	tools := buildClaudeToolsForMCP(req)
	require.Len(t, tools, 2)
	require.Equal(t, "web_search_20260209", tools[0].Type)
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

// TestGetClaudeMessagesPromptTokens_FileImageFallback verifies file-based image blocks apply fallback tokens.
// Parameters: t is the test handler.
// Returns: nothing.
func TestGetClaudeMessagesPromptTokens_FileImageFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	request := &ClaudeMessagesRequest{
		Model: "gpt-3.5-turbo",
		Messages: []relaymodel.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":    "file",
							"file_id": "file-123",
						},
					},
				},
			},
		},
	}

	openaiRequest := convertClaudeToOpenAIForTokenCounting(request)
	baseTokens := openai.CountTokenMessages(ctx, openaiRequest.Messages, request.Model)
	result := getClaudeMessagesPromptTokens(ctx, request)

	require.Equal(t, baseTokens+claudeFileImageFallbackTokens, result)
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
