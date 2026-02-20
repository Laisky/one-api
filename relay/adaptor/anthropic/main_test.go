package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
)

func float64Ptr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func newThinkingContext(t *testing.T, rawURL string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, rawURL, nil)
	c.Request = req
	return c
}

func TestConvertRequest_ThinkingEnabledViaQuery(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions?thinking=enabled")

	request := model.GeneralOpenAIRequest{
		Model:     "claude-3-opus-latest",
		MaxTokens: 4096,
		Messages:  []model.Message{{Role: "user", Content: "hello"}},
	}

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)

	if assert.NotNil(t, converted.Thinking) {
		assert.Equal(t, "enabled", converted.Thinking.Type)
		assert.Equal(t, 1024, converted.Thinking.BudgetTokens)
	}
}

func TestConvertRequest_RejectsThinkingWhenMaxTokensTooSmall(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	request := model.GeneralOpenAIRequest{
		Model:     "claude-3-opus-latest",
		MaxTokens: 1024,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
	}

	_, err := ConvertRequest(c, request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_tokens must be greater than 1024")
}

func TestConvertRequest_ClearsMutuallyExclusiveParametersWhenThinkingEnabled(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	request := model.GeneralOpenAIRequest{
		Model:     "claude-3-opus-latest",
		MaxTokens: 4096,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 2048,
		},
		Temperature: float64Ptr(0.7),
		TopP:        float64Ptr(0.5),
		TopK:        intPtr(32),
	}

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)

	assert.Nil(t, converted.Temperature, "temperature should be nil when thinking is enabled")
	assert.Nil(t, converted.TopP, "top_p should be nil when thinking is enabled")
	assert.Nil(t, converted.TopK, "top_k should be nil when thinking is enabled")
}

func TestConvertRequest_ClearsTopPWhenTemperatureSpecified(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	request := model.GeneralOpenAIRequest{
		Model:       "claude-3-opus-latest",
		MaxTokens:   4096,
		Temperature: float64Ptr(0.8),
		TopP:        float64Ptr(0.6),
	}

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)

	assert.Nil(t, converted.TopP, "top_p should be nil when temperature is provided")
	if assert.NotNil(t, converted.Temperature) {
		assert.InEpsilon(t, 0.8, *converted.Temperature, 1e-9)
	}
}

func TestConvertRequest_DefaultsMissingFunctionSchema(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	request := model.GeneralOpenAIRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 2048,
		Messages:  []model.Message{{Role: "user", Content: "hello"}},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "how-to-subscribe",
					Description: "explain subscription",
				},
			},
		},
	}

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)
	require.Len(t, converted.Tools, 1)
	assert.Equal(t, "object", converted.Tools[0].InputSchema.Type)
}

// TestConvertRequest_MergesAllSystemMessagesToTopLevel verifies all OpenAI system messages are consolidated into Anthropic top-level system.
func TestConvertRequest_MergesAllSystemMessagesToTopLevel(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	request := model.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 300,
		Messages: []model.Message{
			{Role: "system", Content: "You are a roleplay assistant."},
			{Role: "user", Content: "hi"},
			{Role: "system", Content: "Keep the tone warm and short."},
			{Role: "assistant", Content: "Hello!"},
		},
	}

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)

	require.Equal(t, "You are a roleplay assistant.\n\nKeep the tone warm and short.", converted.System)
	require.Len(t, converted.Messages, 2)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "assistant", converted.Messages[1].Role)

	for _, message := range converted.Messages {
		require.NotEqual(t, "system", message.Role)
	}
}

// TestConvertRequest_UserProvidedPayload_SystemMovedOutOfMessages verifies the user-provided request shape does not leak system role into Anthropic messages.
func TestConvertRequest_UserProvidedPayload_SystemMovedOutOfMessages(t *testing.T) {
	c := newThinkingContext(t, "/v1/chat/completions")

	const rawRequest = `{
		"frequency_penalty": 0,
		"model": "claude-sonnet-4-5",
		"max_tokens": 300,
		"messages": [
			{
				"content": "Write Seraphina's next reply in a fictional chat between Seraphina and 林舒白.",
				"role": "system"
			},
			{
				"content": "林舒白: \"What is the glade?\"\\nSeraphina: *Seraphina smiles softly.*",
				"role": "user"
			}
		]
	}`

	var request model.GeneralOpenAIRequest
	err := json.Unmarshal([]byte(rawRequest), &request)
	require.NoError(t, err)

	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.NotNil(t, converted)

	require.Equal(t, "Write Seraphina's next reply in a fictional chat between Seraphina and 林舒白.", converted.System)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.NotContains(t, strings.ToLower(mustMarshalString(t, converted.Messages)), `"role":"system"`)
}

// mustMarshalString marshals value to JSON text and fails the test on error.
func mustMarshalString(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)

	return string(raw)
}

func TestConvertRequest_EmptyAssistantMessageWithToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test case: assistant message with empty content but with tool calls
	temp := 0.7
	openaiRequest := model.GeneralOpenAIRequest{
		Model:       "claude-3.5-sonnet",
		MaxTokens:   4096,
		Temperature: &temp,
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "What's the weather like?",
			},
			{
				Role:    "assistant",
				Content: "", // Empty content
				ToolCalls: []model.Tool{
					{
						Id:   "call_123",
						Type: "function",
						Function: &model.Function{
							Name:      "get_weather",
							Arguments: `{"location": "San Francisco"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    "It's sunny and 72F",
				ToolCallId: "call_123",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_weather",
					Description: "Get weather information",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Find the assistant message (should be the one with tool_use)
	var assistantMessage *Message
	var toolMessage *Message

	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" && len(claudeRequest.Messages[i].Content) > 0 {
			for _, content := range claudeRequest.Messages[i].Content {
				if content.Type == "tool_use" {
					assistantMessage = &claudeRequest.Messages[i]
					break
				}
			}
		}
		if claudeRequest.Messages[i].Role == "user" && len(claudeRequest.Messages[i].Content) > 0 {
			for _, content := range claudeRequest.Messages[i].Content {
				if content.Type == "tool_result" {
					toolMessage = &claudeRequest.Messages[i]
					break
				}
			}
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message with tool_use")

	// Find the tool_use content in the assistant message
	var toolUseContent *Content
	for i := range assistantMessage.Content {
		if assistantMessage.Content[i].Type == "tool_use" {
			toolUseContent = &assistantMessage.Content[i]
			break
		}
	}

	require.NotNil(t, toolUseContent, "Should have tool_use content")

	// Verify it's a tool_use content
	assert.Equal(t, "tool_use", toolUseContent.Type)
	assert.Equal(t, "call_123", toolUseContent.Id)
	assert.Equal(t, "get_weather", toolUseContent.Name)

	// Verify there's no empty text content in the assistant message
	for _, content := range assistantMessage.Content {
		if content.Type == "text" {
			assert.NotEmpty(t, content.Text, "Text content should not be empty")
		}
	}

	// Verify tool result is properly formatted
	require.NotNil(t, toolMessage, "Should have tool result message")
	require.Len(t, toolMessage.Content, 1)
	assert.Equal(t, "tool_result", toolMessage.Content[0].Type)
	assert.Equal(t, "It's sunny and 72F", toolMessage.Content[0].Content)
	assert.Equal(t, "call_123", toolMessage.Content[0].ToolUseId)
}

func TestConvertRequest_ToolRoleWithStructuredContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 4096,
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Create a markdown file with an introduction.",
			},
			{
				Role:    "assistant",
				Content: "I'll create the file for you.",
				ToolCalls: []model.Tool{
					{
						Id:   "call_001",
						Type: "function",
						Function: &model.Function{
							Name:      "write_file",
							Arguments: `{"file_path":"/tmp/a.md","content":"hello"}`,
						},
					},
				},
			},
			{
				Role: "tool",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "Successfully created and wrote to new file: /tmp/a.md.",
					},
				},
				ToolCallId: "call_001",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "write_file",
					Description: "Write content to a file",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"file_path": map[string]any{"type": "string"},
							"content":   map[string]any{"type": "string"},
						},
						"required": []string{"file_path", "content"},
					},
				},
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	for _, message := range claudeRequest.Messages {
		assert.NotEqual(t, "tool", message.Role)
	}

	var toolResultMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role != "user" {
			continue
		}
		if len(claudeRequest.Messages[i].Content) == 0 {
			continue
		}
		if claudeRequest.Messages[i].Content[0].Type == "tool_result" {
			toolResultMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, toolResultMessage)
	require.Len(t, toolResultMessage.Content, 1)
	assert.Equal(t, "tool_result", toolResultMessage.Content[0].Type)
	assert.Equal(t, "call_001", toolResultMessage.Content[0].ToolUseId)
	assert.Equal(t, "Successfully created and wrote to new file: /tmp/a.md.", toolResultMessage.Content[0].Content)
}

func TestConvertRequest_AssistantMessageWithTextAndToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test case: assistant message with both text content and tool calls
	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 4096,
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "What's the weather like?",
			},
			{
				Role:    "assistant",
				Content: "I'll check the weather for you.",
				ToolCalls: []model.Tool{
					{
						Id:   "call_123",
						Type: "function",
						Function: &model.Function{
							Name:      "get_weather",
							Arguments: `{"location": "San Francisco"}`,
						},
					},
				},
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_weather",
					Description: "Get weather information",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Find the assistant message
	var assistantMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")
	require.Len(t, assistantMessage.Content, 2) // Should have both text and tool_use

	// Verify text content
	assert.Equal(t, "text", assistantMessage.Content[0].Type)
	assert.Equal(t, "I'll check the weather for you.", assistantMessage.Content[0].Text)

	// Verify tool_use content
	assert.Equal(t, "tool_use", assistantMessage.Content[1].Type)
	assert.Equal(t, "call_123", assistantMessage.Content[1].Id)
	assert.Equal(t, "get_weather", assistantMessage.Content[1].Name)
}

func TestConvertRequest_EmptyTextContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test case: message with empty text content (using ParseContent path)
	// This should result in an error since the message has no valid content
	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 4096,
		Messages: []model.Message{
			{
				Role: "user",
				Content: []map[string]any{
					{
						"type": "text",
						"text": "", // Empty text
					},
				},
			},
		},
	}

	_, err := ConvertRequest(c, openaiRequest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message must have at least one content block")
}

func TestConvertRequest_ValidJSONGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test the original failing case from the bug report
	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 4096,
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "[User Context: Current time is 2025-06-17 10:22]\n\nUser Request: 今天早上要刷牙",
			},
			{
				Role:    "assistant",
				Content: "", // Empty content
				ToolCalls: []model.Tool{
					{
						Id:   "initial_datetime_call",
						Type: "function",
						Function: &model.Function{
							Name:      "get_current_datetime",
							Arguments: "{}",
						},
					},
				},
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_current_datetime",
					Description: "Get current date and time",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Verify the JSON can be marshaled without issues
	jsonData, err := json.Marshal(claudeRequest)
	require.NoError(t, err)

	// Find the assistant message
	var assistantMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")
	require.Len(t, assistantMessage.Content, 1) // Should only have tool_use

	// Verify it's a tool_use content
	assert.Equal(t, "tool_use", assistantMessage.Content[0].Type)
	assert.Equal(t, "initial_datetime_call", assistantMessage.Content[0].Id)
	assert.Equal(t, "get_current_datetime", assistantMessage.Content[0].Name)

	// Verify the JSON doesn't contain empty text fields
	var jsonObj map[string]any
	err = json.Unmarshal(jsonData, &jsonObj)
	require.NoError(t, err)

	// Check that no message content has empty text type
	messages := jsonObj["messages"].([]any)
	for _, msg := range messages {
		msgMap := msg.(map[string]any)
		if content, ok := msgMap["content"].([]any); ok {
			for _, c := range content {
				contentMap := c.(map[string]any)
				if contentMap["type"] == "text" {
					// If it's a text type, it must have non-empty text
					text, exists := contentMap["text"]
					assert.True(t, exists, "text field must exist for text type")
					assert.NotEmpty(t, text, "text field must not be empty for text type")
				}
			}
		}
	}
}

func TestConvertRequest_MessageWithNoContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test case: message with no content at all
	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 4096,
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:    "assistant",
				Content: "", // Empty content and no tool calls
			},
		},
	}

	_, err := ConvertRequest(c, openaiRequest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message must have at least one content block")
}

func TestConvertRequest_ThinkingBlocksConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Initialize signature cache
	InitSignatureCache(time.Hour)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions?thinking=true", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)

	// Test case: assistant message with reasoning content that should be converted to thinking blocks
	// This simulates the bug scenario where the first request works but the second fails
	reasoningContent := "用户想要读取vite.config.ts文件。我需要使用fileOperation函数来读取这个文件。\n\n需要的参数：\n- operation: \"read\"\n- filePath: \"vite.config.ts\"\n\n这是一个读取操作，所以不需要codeEdit和instruction参数。"

	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3-7-sonnet-20250219", // Model that supports thinking
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []model.Message{
			{
				Role:    "system",
				Content: "你是一个专业的代码编辑助手，请帮助用户优化和改进代码。请用中文回答，并提供具体的代码示例和详细的解释。",
			},
			{
				Role:    "user",
				Content: "能否读取vite.config.ts呢",
			},
			{
				Role:      "assistant",
				Content:   "我来帮您读取vite.config.ts文件。",
				Reasoning: &reasoningContent, // This should be converted to thinking block
				ToolCalls: []model.Tool{
					{
						Id:   "toolu_01Aihmmh3xCfqxLmzCLdepNi",
						Type: "function",
						Function: &model.Function{
							Name:      "fileOperation",
							Arguments: `{"operation": "read", "filePath": "vite.config.ts"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"status":"success","result":"import { defineConfig } from 'vite';\\nexport default defineConfig({\\n    plugins: [],\\n    build: {\\n        outDir: 'dist',\\n        assetsDir: 'assets',\\n    },\\n})\\n","lintError":""}`,
				ToolCallId: "toolu_01Aihmmh3xCfqxLmzCLdepNi",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "fileOperation",
					Description: "Perform file operations",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"operation": map[string]any{
								"type": "string",
								"enum": []string{"read", "write"},
							},
							"filePath": map[string]any{
								"type": "string",
							},
						},
						"required": []string{"operation", "filePath"},
					},
				},
			},
		},
	}

	// Pre-cache a signature to ensure thinking block is created (not fallback)
	tokenIDStr := getTokenIDFromRequest(12345)
	conversationID := generateConversationID(openaiRequest.Messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 (based on len(claudeRequest.Messages) at processing time)
	testSignature := "test_signature_123"

	GetSignatureCache().Store(cacheKey, testSignature)

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Verify thinking is enabled in the request
	require.NotNil(t, claudeRequest.Thinking)
	assert.Equal(t, "enabled", claudeRequest.Thinking.Type)
	assert.Equal(t, 1024, claudeRequest.Thinking.BudgetTokens)

	// Find the assistant message
	var assistantMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")

	// The assistant message should have thinking block, text content, and tool_use
	// Expected order: thinking, text, tool_use
	require.Len(t, assistantMessage.Content, 3)

	// Verify thinking block comes first
	assert.Equal(t, "thinking", assistantMessage.Content[0].Type)
	require.NotNil(t, assistantMessage.Content[0].Thinking)
	assert.Equal(t, reasoningContent, *assistantMessage.Content[0].Thinking)

	// Verify text content comes second
	assert.Equal(t, "text", assistantMessage.Content[1].Type)
	assert.Equal(t, "我来帮您读取vite.config.ts文件。", assistantMessage.Content[1].Text)

	// Verify tool_use comes third
	assert.Equal(t, "tool_use", assistantMessage.Content[2].Type)
	assert.Equal(t, "toolu_01Aihmmh3xCfqxLmzCLdepNi", assistantMessage.Content[2].Id)
	assert.Equal(t, "fileOperation", assistantMessage.Content[2].Name)

	// Verify the JSON can be marshaled without issues
	jsonData, err := json.Marshal(claudeRequest)
	require.NoError(t, err)

	// Verify the JSON structure is correct for Claude API
	var jsonObj map[string]any
	err = json.Unmarshal(jsonData, &jsonObj)
	require.NoError(t, err)

	// Check that thinking is properly set
	thinking := jsonObj["thinking"].(map[string]any)
	assert.Equal(t, "enabled", thinking["type"])
	assert.Equal(t, float64(1024), thinking["budget_tokens"])
}

func TestConvertRequest_ThinkingBlocksWithComplexContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Initialize signature cache
	InitSignatureCache(time.Hour)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/chat/completions?thinking=true", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)

	// Test case: assistant message with complex content structure and reasoning
	// This tests the non-string content path in the conversion logic
	reasoningContent := "用户想要读取vite.config.ts文件。我需要使用fileOperation函数来读取这个文件。"

	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-20250514", // Claude 4 model that supports thinking
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []model.Message{
			{
				Role:    "system",
				Content: "你是一个专业的代码编辑助手。",
			},
			{
				Role:    "user",
				Content: "能否读取vite.config.ts呢",
			},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "我来帮您读取vite.config.ts文件。",
					},
				},
				Reasoning: &reasoningContent, // This should be converted to thinking block
				ToolCalls: []model.Tool{
					{
						Id:   "toolu_01Aihmmh3xCfqxLmzCLdepNi",
						Type: "function",
						Function: &model.Function{
							Name:      "fileOperation",
							Arguments: `{"operation": "read", "filePath": "vite.config.ts"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"status":"success","result":"file content"}`,
				ToolCallId: "toolu_01Aihmmh3xCfqxLmzCLdepNi",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "fileOperation",
					Description: "Perform file operations",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"operation": map[string]any{
								"type": "string",
							},
							"filePath": map[string]any{
								"type": "string",
							},
						},
						"required": []string{"operation", "filePath"},
					},
				},
			},
		},
	}

	// Pre-cache a signature to ensure thinking block is created (not fallback)
	tokenIDStr := getTokenIDFromRequest(12345)
	conversationID := generateConversationID(openaiRequest.Messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 (based on len(claudeRequest.Messages) at processing time)
	testSignature := "test_signature_123"

	GetSignatureCache().Store(cacheKey, testSignature)

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Verify thinking is enabled in the request
	require.NotNil(t, claudeRequest.Thinking)
	assert.Equal(t, "enabled", claudeRequest.Thinking.Type)

	// Find the assistant message
	var assistantMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")

	// The assistant message should have thinking block, text content, and tool_use
	require.Len(t, assistantMessage.Content, 3)

	// Verify thinking block comes first
	assert.Equal(t, "thinking", assistantMessage.Content[0].Type)
	require.NotNil(t, assistantMessage.Content[0].Thinking)
	assert.Equal(t, reasoningContent, *assistantMessage.Content[0].Thinking)

	// Verify text content comes second
	assert.Equal(t, "text", assistantMessage.Content[1].Type)
	assert.Equal(t, "我来帮您读取vite.config.ts文件。", assistantMessage.Content[1].Text)

	// Verify tool_use comes third
	assert.Equal(t, "tool_use", assistantMessage.Content[2].Type)
	assert.Equal(t, "toolu_01Aihmmh3xCfqxLmzCLdepNi", assistantMessage.Content[2].Id)
	assert.Equal(t, "fileOperation", assistantMessage.Content[2].Name)

	// Verify the JSON can be marshaled without issues
	jsonData, err := json.Marshal(claudeRequest)
	require.NoError(t, err)

	// Verify the JSON structure is correct for Claude API
	var jsonObj map[string]any
	err = json.Unmarshal(jsonData, &jsonObj)
	require.NoError(t, err)
}

func TestConvertRequest_BackwardCompatibility_NoThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test context without thinking parameter
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Test case: assistant message with reasoning content but thinking NOT enabled
	// This should work as before - reasoning content should be ignored
	reasoningContent := "Some reasoning content that should be ignored"

	openaiRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3.5-sonnet", // Model that doesn't support thinking
		MaxTokens: 2048,
		// No Thinking field - thinking is disabled
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:      "assistant",
				Content:   "Hello! How can I help you?",
				Reasoning: &reasoningContent, // This should be ignored when thinking is disabled
			},
		},
	}

	claudeRequest, err := ConvertRequest(c, openaiRequest)
	require.NoError(t, err)

	// Verify thinking is NOT enabled in the request
	assert.Nil(t, claudeRequest.Thinking)

	// Find the assistant message
	var assistantMessage *Message
	for i := range claudeRequest.Messages {
		if claudeRequest.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")

	// The assistant message should only have text content (no thinking block)
	require.Len(t, assistantMessage.Content, 1)

	// Verify only text content is present
	assert.Equal(t, "text", assistantMessage.Content[0].Type)
	assert.Equal(t, "Hello! How can I help you?", assistantMessage.Content[0].Text)
	assert.Nil(t, assistantMessage.Content[0].Thinking)

	// Verify the JSON can be marshaled without issues
	jsonData, err := json.Marshal(claudeRequest)
	require.NoError(t, err)

	// Verify no thinking blocks are present in the JSON
	var jsonObj map[string]any
	err = json.Unmarshal(jsonData, &jsonObj)
	require.NoError(t, err)

	// Check that thinking is not set
	_, hasThinking := jsonObj["thinking"]
	assert.False(t, hasThinking, "Should not have thinking field when thinking is disabled")
}

func TestConvertRequest_BugScenario_FullWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Initialize signature cache
	InitSignatureCache(time.Hour)

	// Create a test context with thinking parameter
	req := httptest.NewRequest("POST", "/v1/chat/completions?thinking=true", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(ctxkey.TokenId, 12345)

	// Test the exact scenario from the bug report
	// First request: user asks to read vite.config.ts
	firstRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3-7-sonnet-20250219",
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []model.Message{
			{
				Role:    "system",
				Content: "你是一个专业的代码编辑助手，请帮助用户优化和改进代码。请用中文回答，并提供具体的代码示例和详细的解释。",
			},
			{
				Role:    "user",
				Content: "能否读取vite.config.ts呢",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "fileOperation",
					Description: "Perform file operations",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"operation": map[string]any{
								"type": "string",
								"enum": []string{"read", "write"},
							},
							"filePath": map[string]any{
								"type": "string",
							},
						},
						"required": []string{"operation", "filePath"},
					},
				},
			},
		},
	}

	// Pre-cache a signature to ensure thinking block is created (not fallback)
	tokenIDStr := getTokenIDFromRequest(12345)
	conversationID := generateConversationID(firstRequest.Messages)
	cacheKey := generateSignatureKey(tokenIDStr, conversationID, 1, 0) // messageIndex=1 (based on len(claudeRequest.Messages) at processing time)
	testSignature := "test_signature_123"

	GetSignatureCache().Store(cacheKey, testSignature)

	// First request should work (this was working in the bug report)
	claudeRequest1, err := ConvertRequest(c, firstRequest)
	require.NoError(t, err)
	assert.NotNil(t, claudeRequest1.Thinking)

	// Simulate the response from the first request (with reasoning and tool call)
	reasoningContent := "用户想要读取vite.config.ts文件。我需要使用fileOperation函数来读取这个文件。\n\n需要的参数：\n- operation: \"read\"\n- filePath: \"vite.config.ts\"\n\n这是一个读取操作，所以不需要codeEdit和instruction参数。"

	// Second request: includes the assistant's response with reasoning and tool result
	// This is the scenario that was failing in the bug report
	secondRequest := model.GeneralOpenAIRequest{
		Model:     "claude-3-7-sonnet-20250219",
		MaxTokens: 2048,
		Thinking: &model.Thinking{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []model.Message{
			{
				Role:    "system",
				Content: "你是一个专业的代码编辑助手，请帮助用户优化和改进代码。请用中文回答，并提供具体的代码示例和详细的解释。",
			},
			{
				Role:    "user",
				Content: "能否读取vite.config.ts呢",
			},
			{
				Role:      "assistant",
				Content:   "我来帮您读取vite.config.ts文件。",
				Reasoning: &reasoningContent, // This should be converted to thinking block
				ToolCalls: []model.Tool{
					{
						Id:   "toolu_01Aihmmh3xCfqxLmzCLdepNi",
						Type: "function",
						Function: &model.Function{
							Name:      "fileOperation",
							Arguments: `{"operation": "read", "filePath": "vite.config.ts"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"status":"success","result":"import { defineConfig } from 'vite';\\nexport default defineConfig({\\n    plugins: [],\\n    build: {\\n        outDir: 'dist',\\n        assetsDir: 'assets',\\n    },\\n})\\n","lintError":""}`,
				ToolCallId: "toolu_01Aihmmh3xCfqxLmzCLdepNi",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "fileOperation",
					Description: "Perform file operations",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"operation": map[string]any{
								"type": "string",
								"enum": []string{"read", "write"},
							},
							"filePath": map[string]any{
								"type": "string",
							},
						},
						"required": []string{"operation", "filePath"},
					},
				},
			},
		},
	}

	// Pre-cache a signature for the second request too
	conversationID2 := generateConversationID(secondRequest.Messages)
	cacheKey2 := generateSignatureKey(tokenIDStr, conversationID2, 1, 0) // messageIndex=1 for assistant message
	testSignature2 := "test_signature_456"

	GetSignatureCache().Store(cacheKey2, testSignature2)

	// Second request should now work (this was failing before the fix)
	claudeRequest2, err := ConvertRequest(c, secondRequest)
	require.NoError(t, err, "Second request with tool results should work after the fix")

	// Verify thinking is enabled
	require.NotNil(t, claudeRequest2.Thinking)
	assert.Equal(t, "enabled", claudeRequest2.Thinking.Type)

	// Find the assistant message in the second request
	var assistantMessage *Message
	for i := range claudeRequest2.Messages {
		if claudeRequest2.Messages[i].Role == "assistant" {
			assistantMessage = &claudeRequest2.Messages[i]
			break
		}
	}

	require.NotNil(t, assistantMessage, "Should have assistant message")

	// The assistant message should have thinking block, text content, and tool_use
	// This is the key fix - the thinking block should be properly formatted
	require.Len(t, assistantMessage.Content, 3)

	// Verify thinking block comes first (this is required by Claude API)
	assert.Equal(t, "thinking", assistantMessage.Content[0].Type)
	require.NotNil(t, assistantMessage.Content[0].Thinking)
	assert.Equal(t, reasoningContent, *assistantMessage.Content[0].Thinking)

	// Verify text content comes second
	assert.Equal(t, "text", assistantMessage.Content[1].Type)
	assert.Equal(t, "我来帮您读取vite.config.ts文件。", assistantMessage.Content[1].Text)

	// Verify tool_use comes third
	assert.Equal(t, "tool_use", assistantMessage.Content[2].Type)
	assert.Equal(t, "toolu_01Aihmmh3xCfqxLmzCLdepNi", assistantMessage.Content[2].Id)
	assert.Equal(t, "fileOperation", assistantMessage.Content[2].Name)

	// Verify the JSON can be marshaled and is valid for Claude API
	jsonData, err := json.Marshal(claudeRequest2)
	require.NoError(t, err)

	// This should not produce the error from the bug report:
	// "messages.1.content.0.type: Expected `thinking` or `redacted_thinking`, but found `text`"
	var jsonObj map[string]any
	err = json.Unmarshal(jsonData, &jsonObj)
	require.NoError(t, err)

	// Verify the messages structure is correct for Claude API
	messages := jsonObj["messages"].([]any)
	for _, msg := range messages {
		msgMap := msg.(map[string]any)
		if msgMap["role"] == "assistant" {
			content := msgMap["content"].([]any)
			if len(content) > 0 {
				firstContent := content[0].(map[string]any)
				// The first content block should be thinking when thinking is enabled
				// and the message has reasoning content
				if len(content) > 1 { // If there are multiple content blocks
					assert.Equal(t, "thinking", firstContent["type"],
						"First content block should be thinking when reasoning is present")
				}
			}
		}
	}
}

func TestConvertClaudeRequest_DirectPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a proper test context with a request
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Create an Anthropic adaptor instance
	adaptor := &Adaptor{}

	// Test Claude Messages request
	claudeRequest := &model.ClaudeRequest{
		Model:     "claude-3.5-sonnet",
		MaxTokens: 1000,
		Messages: []model.ClaudeMessage{
			{
				Role:    "user",
				Content: "Hello, how are you?",
			},
		},
		Temperature: func() *float64 { f := 0.7; return &f }(),
		Stream:      func() *bool { b := false; return &b }(),
	}

	// Call ConvertClaudeRequest
	result, err := adaptor.ConvertClaudeRequest(c, claudeRequest)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify that the direct pass-through flag is set
	directPassthrough, exists := c.Get(ctxkey.ClaudeDirectPassthrough)
	assert.True(t, exists, "claude_direct_passthrough flag should be set")
	assert.True(t, directPassthrough.(bool), "claude_direct_passthrough should be true")

	// Verify that claude_messages_native flag is set
	nativeFlag, exists := c.Get(ctxkey.ClaudeMessagesNative)
	assert.True(t, exists, "claude_messages_native flag should be set")
	assert.True(t, nativeFlag.(bool), "claude_messages_native should be true")

	// Verify that claude_model is set
	model, exists := c.Get(ctxkey.ClaudeModel)
	assert.True(t, exists, "claude_model should be set")
	assert.Equal(t, "claude-3.5-sonnet", model.(string), "claude_model should match request model")

	// The result should be the original request (though it won't be used for marshaling)
	// Since the interface returns any, we just verify it's not nil and the flags are set correctly
	assert.NotNil(t, result, "result should not be nil")
}

func TestConvertClaudeRequest_ClearsTopPWhenTemperatureProvided(t *testing.T) {
	c := newThinkingContext(t, "/v1/messages")

	claudeRequest := model.ClaudeRequest{
		Model:       "claude-3.5-sonnet",
		MaxTokens:   2048,
		Messages:    []model.ClaudeMessage{{Role: "user", Content: "hello"}},
		Temperature: float64Ptr(0.6),
		TopP:        float64Ptr(0.5),
	}

	converted, err := ConvertClaudeRequest(c, claudeRequest)
	require.NoError(t, err)
	require.NotNil(t, converted)

	assert.NotNil(t, converted.Temperature, "temperature should be preserved")
	assert.Nil(t, converted.TopP, "top_p should be cleared when temperature is provided")
}
