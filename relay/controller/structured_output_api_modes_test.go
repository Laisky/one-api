package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func boolPtr(value bool) *bool {
	return &value
}

func baseJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic": map[string]any{
				"type":        "string",
				"description": "Target topic to extract",
			},
			"confidence": map[string]any{
				"type": "number",
			},
		},
		"required": []any{"topic"},
	}
}

// TestStructuredOutputChatCompletionsVariants exercises structured output parsing for Chat Completions with both streaming modes.
func TestStructuredOutputChatCompletionsVariants(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	strict := true

	cases := map[string]bool{
		"non-stream": false,
		"stream":     true,
	}

	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload := &relaymodel.GeneralOpenAIRequest{
				Model: "gpt-4o-mini",
				Messages: []relaymodel.Message{
					{Role: "user", Content: "Extract the topic and confidence."},
				},
				Stream: stream,
				ResponseFormat: &relaymodel.ResponseFormat{
					Type: "json_schema",
					JsonSchema: &relaymodel.JSONSchema{
						Name:   "structured_result",
						Schema: baseJSONSchema(),
						Strict: &strict,
					},
				},
			}

			body, err := json.Marshal(payload)
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger.Logger)
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			parsed, err := getAndValidateTextRequest(c, relaymode.ChatCompletions)
			require.NoError(t, err)
			require.NotNil(t, parsed)
			require.Equal(t, stream, parsed.Stream)

			require.NotNil(t, parsed.ResponseFormat)
			require.Equal(t, "json_schema", parsed.ResponseFormat.Type)
			require.NotNil(t, parsed.ResponseFormat.JsonSchema)
			require.Equal(t, "structured_result", parsed.ResponseFormat.JsonSchema.Name)
			require.NotNil(t, parsed.ResponseFormat.JsonSchema.Strict)
			require.True(t, *parsed.ResponseFormat.JsonSchema.Strict)

			schema := parsed.ResponseFormat.JsonSchema.Schema
			require.NotNil(t, schema)

			properties, ok := schema["properties"].(map[string]any)
			require.True(t, ok)
			topic, ok := properties["topic"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "string", topic["type"])

			requiredField, ok := schema["required"].([]any)
			require.True(t, ok)
			require.Len(t, requiredField, 1)
			nameValue, ok := requiredField[0].(string)
			require.True(t, ok)
			require.Equal(t, "topic", nameValue)
		})
	}
}

// TestStructuredOutputResponseAPIVariants verifies structured output conversion for the Response API fallback across stream modes.
func TestStructuredOutputResponseAPIVariants(t *testing.T) {
	t.Parallel()
	strict := true

	cases := map[string]bool{
		"non-stream": false,
		"stream":     true,
	}

	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := &openai.ResponseAPIRequest{
				Model:  "gpt-4o-mini",
				Stream: boolPtr(stream),
				Input:  openai.ResponseAPIInput{"Summarize the provided article."},
				Text: &openai.ResponseTextConfig{
					Format: &openai.ResponseTextFormat{
						Type:        "json_schema",
						Name:        "structured_result",
						Description: "Normalized summary payload",
						Schema:      baseJSONSchema(),
						Strict:      &strict,
					},
				},
			}

			chatReq, err := openai.ConvertResponseAPIToChatCompletionRequest(req)
			require.NoError(t, err)
			require.NotNil(t, chatReq)
			require.Equal(t, stream, chatReq.Stream)

			require.NotNil(t, chatReq.ResponseFormat)
			require.Equal(t, "json_schema", chatReq.ResponseFormat.Type)
			require.NotNil(t, chatReq.ResponseFormat.JsonSchema)
			require.Equal(t, "structured_result", chatReq.ResponseFormat.JsonSchema.Name)
			require.Nil(t, chatReq.ResponseFormat.JsonSchema.Strict)

			schema := chatReq.ResponseFormat.JsonSchema.Schema
			require.NotNil(t, schema)
			properties, ok := schema["properties"].(map[string]any)
			require.True(t, ok)
			confidence, ok := properties["confidence"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "number", confidence["type"])

			requiredField, ok := schema["required"].([]any)
			require.True(t, ok)
			require.Len(t, requiredField, 1)
			first, ok := requiredField[0].(string)
			require.True(t, ok)
			require.Equal(t, "topic", first)

			require.NotNil(t, req.Text)
			require.NotNil(t, req.Text.Format)
			require.NotNil(t, req.Text.Format.Strict)
			require.True(t, *req.Text.Format.Strict)
		})
	}
}

// TestStructuredOutputClaudeVariants ensures structured Claude requests preserve tool schemas for both streaming options.
func TestStructuredOutputClaudeVariants(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	toolSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic":     map[string]any{"type": "string"},
				"sentiment": map[string]any{"type": "string"},
			},
			"required": []any{"topic"},
		}
	}

	cases := map[string]bool{
		"non-stream": false,
		"stream":     true,
	}

	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := &ClaudeMessagesRequest{
				Model:     "claude-3-sonnet-20240229",
				MaxTokens: 256,
				Stream:    boolPtr(stream),
				Messages: []relaymodel.ClaudeMessage{
					{Role: "user", Content: "Extract key facts."},
				},
				Tools: []relaymodel.ClaudeTool{
					{
						Name:        "structured_extractor",
						Description: "Return normalized insights",
						InputSchema: toolSchema(),
					},
				},
			}

			body, err := json.Marshal(request)
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger.Logger)
			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			parsed, err := getAndValidateClaudeMessagesRequest(c)
			require.NoError(t, err)
			require.NotNil(t, parsed)
			require.NotNil(t, parsed.Stream)
			require.Equal(t, stream, *parsed.Stream)
			require.Len(t, parsed.Tools, 1)

			rawSchema, ok := parsed.Tools[0].InputSchema.(map[string]any)
			require.True(t, ok)
			props, ok := rawSchema["properties"].(map[string]any)
			require.True(t, ok)
			_, ok = props["topic"].(map[string]any)
			require.True(t, ok)

			converted := convertClaudeToolsToOpenAI(parsed.Tools)
			require.Len(t, converted, 1)
			require.NotNil(t, converted[0].Function)
			parameters, ok := converted[0].Function.Parameters.(map[string]any)
			require.True(t, ok)
			toolProps, ok := parameters["properties"].(map[string]any)
			require.True(t, ok)
			require.Contains(t, toolProps, "topic")
		})
	}
}
