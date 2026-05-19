package openai_compatible

import (
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// validToolNameRE mirrors `^[a-zA-Z0-9_-]+$` enforced by strict provider
// validators (OpenAI, DeepSeek, Anthropic).
var validToolNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// TestConvertClaudeRequest_SanitizesDottedToolName ensures the Claude→OpenAI
// shared converter sanitizes Claude tool definitions whose names violate
// `^[a-zA-Z0-9_-]+$` before they are handed to OpenAI-compatible adapters.
func TestConvertClaudeRequest_SanitizesDottedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &model.ClaudeRequest{
		Model:     "deepseek-chat",
		MaxTokens: 128,
		Messages: []model.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.ClaudeTool{
			{Name: "mcp.search.web", Description: "search", InputSchema: map[string]any{"type": "object"}},
		},
	}

	out, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)

	converted, ok := out.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "expected *model.GeneralOpenAIRequest, got %T", out)
	require.Len(t, converted.Tools, 1)
	require.NotNil(t, converted.Tools[0].Function)
	require.Equal(t, "mcp_search_web", converted.Tools[0].Function.Name)
	require.True(t, validToolNameRE.MatchString(converted.Tools[0].Function.Name))

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists, "expected rename map on context")
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "mcp.search.web", mp["mcp_search_web"])
}
