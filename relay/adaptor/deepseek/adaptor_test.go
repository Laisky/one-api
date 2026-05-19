package deepseek

import (
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// deepseekValidToolName mirrors DeepSeek's strict `^[a-zA-Z0-9_-]+$` validator.
var deepseekValidToolName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestConvertRequest_NormalizesToolArrayContentToString(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "user", Content: "hello"},
			{
				Role:       "tool",
				ToolCallId: "call_1",
				Content: []any{
					map[string]any{"type": "text", "text": "README.md\n"},
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "README.md\n", converted.Messages[1].Content)
}

func TestConvertRequest_NormalizesToolMapContentByJSONFallback(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{
				Role:       "tool",
				ToolCallId: "call_2",
				Content: map[string]any{
					"stdout":    "ok",
					"exit_code": 0,
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	contentStr, ok := converted.Messages[0].Content.(string)
	require.True(t, ok)
	require.Contains(t, contentStr, `"stdout":"ok"`)
	require.Contains(t, contentStr, `"exit_code":0`)
}

func TestConvertRequest_NormalizesNilToolContentToEmptyString(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "tool", ToolCallId: "call_3", Content: nil},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "", converted.Messages[0].Content)
}

func TestConvertRequest_DoesNotChangeNonToolArrayContent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	originalContent := []any{map[string]any{"type": "text", "text": "hello"}}
	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "user", Content: originalContent},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, originalContent, converted.Messages[0].Content)
}

// TestConvertRequest_SanitizesDottedToolName guards the DeepSeek adapter's
// integration with toolnamesafe: an OpenAI request with `tools[].function.name`
// containing characters outside `^[a-zA-Z0-9_-]+$` must be rewritten and a
// reverse-lookup map stashed on the gin context for restoration on the
// response path.
func TestConvertRequest_SanitizesDottedToolName(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "mcp.server.search",
					Description: "search MCP server",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Tools, 1)
	require.NotNil(t, converted.Tools[0].Function)
	require.Equal(t, "mcp_server_search", converted.Tools[0].Function.Name)
	require.True(t, deepseekValidToolName.MatchString(converted.Tools[0].Function.Name))

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists, "expected toolname rename map on context")
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "mcp.server.search", mp["mcp_server_search"])
}

// TestConvertClaudeRequest_SanitizesDottedToolName covers the Claude→DeepSeek
// path: the shared openai_compatible.ConvertClaudeRequest pipeline must
// sanitize Claude tool definitions before they cross the DeepSeek boundary.
func TestConvertClaudeRequest_SanitizesDottedToolName(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.ClaudeRequest{
		Model:     "deepseek-chat",
		MaxTokens: 64,
		Messages: []model.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.ClaudeTool{
			{Name: "mcp__a.b", Description: "namespaced", InputSchema: map[string]any{"type": "object"}},
		},
	}

	out, err := adaptor.ConvertClaudeRequest(c, request)
	require.NoError(t, err)
	converted, ok := out.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "expected *model.GeneralOpenAIRequest, got %T", out)
	require.Len(t, converted.Tools, 1)
	require.NotNil(t, converted.Tools[0].Function)
	require.Equal(t, "mcp__a_b", converted.Tools[0].Function.Name)
	require.True(t, deepseekValidToolName.MatchString(converted.Tools[0].Function.Name))

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists)
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "mcp__a.b", mp["mcp__a_b"])
}
