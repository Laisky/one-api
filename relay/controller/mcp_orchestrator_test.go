package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/mcp"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// TestNormalizeChatToolChoiceForMCP verifies MCP tool choices are normalized to function.
func TestNormalizeChatToolChoiceForMCP(t *testing.T) {
	mcpNames := map[string]struct{}{"web_search_20250305": {}}
	choice := map[string]any{"type": "web_search_20250305"}

	normalized := normalizeChatToolChoiceForMCP(choice, mcpNames)
	result, ok := normalized.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", result["type"])
	function, ok := result["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "web_search_20250305", function["name"])
}

// TestNormalizeMCPToolChoiceForResponse verifies Response API tool_choice normalization for MCP tools.
func TestNormalizeMCPToolChoiceForResponse(t *testing.T) {
	mcpNames := map[string]struct{}{"web_search_20250305": {}}
	choice := map[string]any{"type": "web_search_20250305"}

	normalized := normalizeMCPToolChoiceForResponse(choice, mcpNames)
	result, ok := normalized.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", result["type"])
	require.Equal(t, "web_search_20250305", result["name"])
}

// TestMergeToolUsageSummaries merges MCP usage entries into existing summaries.
func TestMergeToolUsageSummaries(t *testing.T) {
	base := &model.ToolUsageSummary{
		TotalCost:  10,
		Counts:     map[string]int{"web_search": 1},
		CostByTool: map[string]int64{"web_search": 10},
		Entries: []model.ToolUsageEntry{
			{Tool: "web_search", Source: "channel_builtin", ServerID: 0, Count: 1, Cost: 10},
		},
	}
	addition := &model.ToolUsageSummary{
		TotalCost:  5,
		Counts:     map[string]int{"mcp.search": 1},
		CostByTool: map[string]int64{"mcp.search": 5},
		Entries: []model.ToolUsageEntry{
			{Tool: "mcp.search", Source: "oneapi_builtin", ServerID: 2, Count: 1, Cost: 5},
		},
	}

	merged := mergeToolUsageSummaries(base, addition)
	require.Equal(t, int64(15), merged.TotalCost)
	require.Equal(t, 2, len(merged.Counts))
	require.Equal(t, 2, len(merged.Entries))
}

// TestExpandMCPBuiltinsInChatRequest_MCPPrecedence ensures MCP tools are preferred over upstream built-ins.
func TestExpandMCPBuiltinsInChatRequest_MCPPrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	err := model.DB.Where("name = ?", "mcp-search").Delete(&model.MCPServer{}).Error
	require.NoError(t, err, "failed to clean mcp server fixture")
	err = model.DB.Where("name = ?", "web_search").Delete(&model.MCPTool{}).Error
	require.NoError(t, err, "failed to clean mcp tool fixture")

	server := &model.MCPServer{
		Name:          "mcp-search",
		Status:        model.MCPServerStatusEnabled,
		BaseURL:       "http://mcp.example.com",
		ToolWhitelist: model.JSONStringSlice{"web_search"},
	}
	err = model.DB.Create(server).Error
	require.NoError(t, err, "failed to create mcp server fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", server.Id).Delete(&model.MCPServer{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp server fixture")
	})

	tool := &model.MCPTool{
		ServerId:    server.Id,
		Name:        "web_search",
		Description: "Search the web",
		InputSchema: `{"type":"object","properties":{}}`,
	}
	err = model.DB.Create(tool).Error
	require.NoError(t, err, "failed to create mcp tool fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", tool.Id).Delete(&model.MCPTool{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp tool fixture")
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(ctxkey.Id, fallbackUserID)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Tools: []relaymodel.Tool{{Type: "web_search"}},
	}
	meta := &metalib.Meta{ActualModelName: "gpt-4o", ChannelType: channeltype.OpenAI}
	channel := &model.Channel{Type: channeltype.OpenAI}

	registry, mcpNames, err := expandMCPBuiltinsInChatRequest(c, meta, channel, nil, request)
	require.NoError(t, err, "unexpected error expanding mcp builtins")
	require.NotNil(t, registry, "expected mcp registry to be created")
	require.Contains(t, mcpNames, "web_search")
	require.Len(t, request.Tools, 1)
	require.Equal(t, "function", request.Tools[0].Type)
	require.NotNil(t, request.Tools[0].Function)
	require.Equal(t, "web_search", request.Tools[0].Function.Name)
	require.True(t, registry.isMCPTool("web_search"))
}

// TestExpandMCPBuiltinsInChatRequest_PreviewBuiltinAlias ensures preview tools add normalized names.
func TestExpandMCPBuiltinsInChatRequest_PreviewBuiltinAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	err := model.DB.Where("name = ?", "mcp-preview").Delete(&model.MCPServer{}).Error
	require.NoError(t, err, "failed to clean mcp server fixture")
	err = model.DB.Where("name = ?", "web_search_preview").Delete(&model.MCPTool{}).Error
	require.NoError(t, err, "failed to clean mcp tool fixture")

	server := &model.MCPServer{
		Name:          "mcp-preview",
		Status:        model.MCPServerStatusEnabled,
		BaseURL:       "http://mcp.preview.example.com",
		ToolWhitelist: model.JSONStringSlice{"web_search_preview"},
	}
	err = model.DB.Create(server).Error
	require.NoError(t, err, "failed to create mcp server fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", server.Id).Delete(&model.MCPServer{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp server fixture")
	})

	tool := &model.MCPTool{
		ServerId:    server.Id,
		Name:        "web_search_preview",
		Description: "Preview search",
		InputSchema: `{"type":"object","properties":{}}`,
	}
	err = model.DB.Create(tool).Error
	require.NoError(t, err, "failed to create mcp tool fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", tool.Id).Delete(&model.MCPTool{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp tool fixture")
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(ctxkey.Id, fallbackUserID)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Tools: []relaymodel.Tool{{Type: "web_search_preview"}},
	}
	meta := &metalib.Meta{ActualModelName: "gpt-4o", ChannelType: channeltype.OpenAI}
	channel := &model.Channel{Type: channeltype.OpenAI}

	_, mcpNames, err := expandMCPBuiltinsInChatRequest(c, meta, channel, nil, request)
	require.NoError(t, err, "unexpected error expanding mcp preview builtins")
	require.Contains(t, mcpNames, "web_search_preview")
	require.Contains(t, mcpNames, "web_search")
}

// TestBuildFunctionToolFromMCP_DefaultSchema ensures empty schemas still produce valid parameters.
func TestBuildFunctionToolFromMCP_DefaultSchema(t *testing.T) {
	tool := &model.MCPTool{Name: "empty_schema", Description: "Empty schema"}
	candidate := mcp.ToolCandidate{ResolvedTool: mcp.ResolvedTool{Tool: tool}}

	converted, err := buildFunctionToolFromMCP(candidate)
	require.NoError(t, err)
	require.NotNil(t, converted.Function)
	require.NotNil(t, converted.Function.Parameters)
	params, ok := converted.Function.Parameters.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", params["type"])
}

// TestBuildToolResultMessage_UsesRawPayload verifies full MCP payload forwarding.
func TestBuildToolResultMessage_UsesRawPayload(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"ok"}],"is_error":false,"results":[{"url":"https://example.com","title":"Example"}]}`
	var result mcp.CallToolResult
	err := json.Unmarshal([]byte(raw), &result)
	require.NoError(t, err)

	msg, err := buildToolResultMessage("call-1", &result)
	require.NoError(t, err)

	var payload map[string]any
	err = json.Unmarshal([]byte(msg.Content.(string)), &payload)
	require.NoError(t, err)
	require.Contains(t, payload, "results")
	require.Contains(t, payload, "content")
}
