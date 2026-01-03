package openai_compatible

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestConvertClaudeRequest_ToOpenAI(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &relaymodel.ClaudeRequest{
		Model:     "claude-3",
		MaxTokens: 128,
		System:    []any{map[string]any{"type": "text", "text": "sys"}},
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "hi"},
				map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://a"}},
			}},
			{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "c1", "name": "get_weather", "input": map[string]any{"city": "SF"}}}},
			{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_call_id": "c1", "content": []any{map[string]any{"type": "text", "text": "ok"}}}}},
		},
		Tools:      []relaymodel.ClaudeTool{{Name: "get_weather", Description: "Get weather", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: map[string]any{"type": "tool", "name": "get_weather"},
	}

	out, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	// Ensure context flags are set for conversion path
	val, exists := c.Get(ctxkey.ClaudeMessagesConversion)
	assert.True(t, exists)
	assert.Equal(t, true, val)

	// Marshal to ensure it's valid JSON
	b, merr := json.Marshal(out)
	require.NoError(t, merr)
	// Basic sanity checks
	var goReq relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(b, &goReq))
	assert.Equal(t, "claude-3", goReq.Model)
	require.NotNil(t, goReq.MaxCompletionTokens)
	assert.Equal(t, 128, *goReq.MaxCompletionTokens)
	assert.GreaterOrEqual(t, len(goReq.Messages), 4)
	assert.NotNil(t, goReq.Tools)
	assert.NotNil(t, goReq.ToolChoice)
	choiceMap, ok := goReq.ToolChoice.(map[string]any)
	require.True(t, ok, "expected map tool_choice, got %T", goReq.ToolChoice)
	assert.Equal(t, "function", choiceMap["type"])
	fn, _ := choiceMap["function"].(map[string]any)
	assert.Equal(t, "get_weather", fn["name"])
	_, hasName := choiceMap["name"]
	assert.False(t, hasName)

	var (
		assistantSeen bool
		toolSeen      bool
	)
	for _, msg := range goReq.Messages {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				assistantSeen = true
				assert.Equal(t, "get_weather", msg.ToolCalls[0].Function.Name)
			}
		case "tool":
			toolSeen = true
			assert.Equal(t, "c1", msg.ToolCallId)
			assert.Equal(t, "ok", msg.StringContent())
		}
	}
	assert.True(t, assistantSeen, "expected assistant message with tool call")
	assert.True(t, toolSeen, "expected tool result message")
}

func TestConvertClaudeRequest_ToolResultWithFollowupText(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &relaymodel.ClaudeRequest{
		Model: "claude-3",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "use_123", "name": "get_weather", "input": map[string]any{"location": "Paris"}}}},
			{Role: "user", Content: []any{
				map[string]any{"type": "tool_result", "tool_use_id": "use_123", "content": []any{map[string]any{"type": "text", "text": "{\"temperature\":20}"}}},
				map[string]any{"type": "text", "text": "Great, please summarise the forecast."},
			}},
		},
	}

	converted, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)

	body, err := json.Marshal(converted)
	require.NoError(t, err)

	var goReq relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(body, &goReq))

	var toolMsgs int
	var followupSeen bool
	for _, msg := range goReq.Messages {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) == 1 && msg.ToolCalls[0].Id == "use_123" {
				toolMsgs++
			}
		case "tool":
			toolMsgs++
			assert.Equal(t, "use_123", msg.ToolCallId)
			assert.Equal(t, "{\"temperature\":20}", msg.StringContent())
		case "user":
			if strings.Contains(msg.StringContent(), "summarise the forecast") {
				followupSeen = true
			}
		}
	}

	assert.Equal(t, 2, toolMsgs, "expected tool call and tool result messages to be preserved")
	assert.True(t, followupSeen, "expected follow-up user text after tool result")
}

func TestHandleClaudeMessagesResponse_NonStream_ConvertedResponse(t *testing.T) {
	t.Parallel()
	// Validate the handler path where the adaptor provides a converted response (stored in context)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Mark as Claude conversion
	c.Set(ctxkey.ClaudeMessagesConversion, true)

	// Prepare meta
	m := &meta.Meta{ActualModelName: "gpt-x", PromptTokens: 11, IsStream: false}
	meta.Set2Context(c, m)

	// Prepare a converted Claude JSON response
	cr := relaymodel.ClaudeResponse{
		ID:         "id1",
		Type:       "message",
		Role:       "assistant",
		Model:      "gpt-x",
		Content:    []relaymodel.ClaudeContent{{Type: "text", Text: "hello"}},
		StopReason: "end_turn",
		Usage:      relaymodel.ClaudeUsage{InputTokens: 11, OutputTokens: 5},
	}
	b, _ := json.Marshal(cr)
	conv := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(b))}
	c.Set(ctxkey.ConvertedResponse, conv)

	// Call
	fallbackCalled := false
	usage, errResp := HandleClaudeMessagesResponse(c, conv, m, func(*gin.Context, *http.Response, int, string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
		// Should not be called in this path
		fallbackCalled = true
		return nil, nil
	})
	require.False(t, fallbackCalled, "fallback handler should not be invoked")
	require.Nil(t, errResp)
	// Non-stream path returns nil usage and stores converted response in context for controller
	assert.Nil(t, usage)
	v, ok := c.Get(ctxkey.ConvertedResponse)
	require.True(t, ok)
	resp, _ := v.(*http.Response)
	require.NotNil(t, resp)
}

func TestHandler_NonStream_ComputeUsageFromContent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Build OpenAI-compatible JSON with zero usage to trigger computation
	text := `{"choices":[{"index":0,"message":{"role":"assistant","content":"Hello","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":{"x":1}}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(text))}

	errResp, usage := Handler(c, resp, 9, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Computation with simple estimator: "Hello" (5/4=1) + {"x":1} (7/4=1) = 2; prompt=9; total=11
	assert.Equal(t, 9, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 11, usage.TotalTokens)
}

func TestConvertClaudeRequest_StructuredToolPromoted(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "number"},
		},
		"required": []any{"topic", "confidence"},
	}
	schema["additionalProperties"] = false

	req := &relaymodel.ClaudeRequest{
		Model:     "claude-structured",
		MaxTokens: 512,
		Messages: []relaymodel.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Provide topic and confidence JSON."},
				},
			},
		},
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "topic_classifier",
				Description: "Return structured topic and confidence data",
				InputSchema: schema,
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}

	convertedAny, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)

	require.NotNil(t, converted.ResponseFormat)
	assert.Equal(t, "json_schema", converted.ResponseFormat.Type)
	require.NotNil(t, converted.ResponseFormat.JsonSchema)
	assert.Equal(t, "topic_classifier", converted.ResponseFormat.JsonSchema.Name)
	assert.Equal(t, "Return structured topic and confidence data", converted.ResponseFormat.JsonSchema.Description)
	require.NotNil(t, converted.ResponseFormat.JsonSchema.Strict)
	assert.True(t, *converted.ResponseFormat.JsonSchema.Strict)
	assert.Equal(t, schema, converted.ResponseFormat.JsonSchema.Schema)
	assert.Nil(t, converted.ToolChoice)
	assert.Empty(t, converted.Tools)
	require.NotNil(t, converted.MaxCompletionTokens)
	assert.Equal(t, 512, *converted.MaxCompletionTokens)

	// Ensure original request remains unchanged
	require.Len(t, req.Tools, 1)
	assert.NotNil(t, req.ToolChoice)
}

func TestConvertClaudeRequest_StructuredPromotionDisabledForDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-chat"})

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "number"},
		},
		"required": []any{"topic", "confidence"},
	}
	schema["additionalProperties"] = false

	req := &relaymodel.ClaudeRequest{
		Model:     "deepseek-chat",
		MaxTokens: 256,
		Messages: []relaymodel.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Provide structured topic and confidence."},
				},
			},
		},
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "topic_classifier",
				Description: "Return structured topic and confidence data",
				InputSchema: schema,
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}

	convertedAny, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Nil(t, converted.ResponseFormat)
	require.NotNil(t, converted.ToolChoice)
	require.NotEmpty(t, converted.Tools)
}

func TestConvertClaudeRequest_StructuredPromotionEnabledForAzureGPT5(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.Azure, ActualModelName: "gpt-5-nano"})

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "number"},
		},
		"required": []any{"topic", "confidence"},
	}
	schema["additionalProperties"] = false

	req := &relaymodel.ClaudeRequest{
		Model:     "gpt-5-nano",
		MaxTokens: 256,
		Messages: []relaymodel.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Provide structured topic and confidence JSON."},
				},
			},
		},
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "topic_classifier",
				Description: "Return structured topic and confidence data",
				InputSchema: schema,
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}

	convertedAny, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)

	require.NotNil(t, converted.ResponseFormat)
	require.NotNil(t, converted.ResponseFormat.JsonSchema)
	assert.Equal(t, "json_schema", converted.ResponseFormat.Type)
	assert.Equal(t, "topic_classifier", converted.ResponseFormat.JsonSchema.Name)
	assert.Equal(t, schema, converted.ResponseFormat.JsonSchema.Schema)
	assert.Nil(t, converted.ToolChoice)
	assert.Empty(t, converted.Tools)
}

func TestConvertClaudeRequest_ToolNotPromoted(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &relaymodel.ClaudeRequest{
		Model:     "gpt-tool",
		MaxTokens: 2048,
		Messages: []relaymodel.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Use the get_weather tool to retrieve today's weather in San Francisco, CA."},
				},
			},
		},
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "get_weather",
				Description: "Get the current weather for a location",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City and region to look up (example: San Francisco, CA)",
						},
						"unit": map[string]any{
							"type":        "string",
							"description": "Temperature unit to use",
							"enum":        []any{"celsius", "fahrenheit"},
						},
					},
					"required": []any{"location"},
				},
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "get_weather"},
	}

	convertedAny, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Nil(t, converted.ResponseFormat)
	require.NotNil(t, converted.ToolChoice)
	assert.NotEmpty(t, converted.Tools)
	require.NotNil(t, converted.MaxCompletionTokens)
	assert.Equal(t, 2048, *converted.MaxCompletionTokens)
}
