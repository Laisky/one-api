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

// TestApplyMCPToolCostDelta_NewUsage verifies MCP tool costs initialize usage when nil.
func TestApplyMCPToolCostDelta_NewUsage(t *testing.T) {
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{TotalCost: 12}}
	updated := applyMCPToolCostDelta(nil, 0, summary)
	require.NotNil(t, updated)
	require.Equal(t, int64(12), updated.ToolsCost)
}

// TestRecordMCPToolUsage_QualifiedName verifies MCP tool usage is recorded with server-qualified names.
func TestRecordMCPToolUsage_QualifiedName(t *testing.T) {
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{Counts: map[string]int{}, CostByTool: map[string]int64{}}}
	candidate := mcp.ToolCandidate{ResolvedTool: mcp.ResolvedTool{
		Tool:        &model.MCPTool{Name: "search"},
		Policy:      mcp.ToolPolicySnapshot{Pricing: model.ToolPricingLocal{QuotaPerCall: 7}},
		ServerID:    9,
		ServerLabel: "mcp",
	}}

	recordMCPToolUsage(summary, candidate, "search")

	require.Equal(t, int64(7), summary.summary.TotalCost)
	require.Equal(t, 1, summary.summary.Counts["mcp.search"])
	require.Equal(t, int64(7), summary.summary.CostByTool["mcp.search"])
	require.Len(t, summary.summary.Entries, 1)
	require.Equal(t, "mcp.search", summary.summary.Entries[0].Tool)
	require.Equal(t, "oneapi_builtin", summary.summary.Entries[0].Source)
	require.Equal(t, 9, summary.summary.Entries[0].ServerID)
}

// TestApplyMCPToolCostDelta_Accumulates verifies MCP tool cost deltas accumulate on existing usage.
func TestApplyMCPToolCostDelta_Accumulates(t *testing.T) {
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{TotalCost: 20}}
	usage := &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 4, ToolsCost: 5}
	updated := applyMCPToolCostDelta(usage, 12, summary)
	require.Equal(t, int64(13), updated.ToolsCost)
	require.Equal(t, 7, updated.TotalTokens)
}

// TestApplyMCPToolCostDelta_NoChange verifies zero or negative deltas do not mutate usage.
func TestApplyMCPToolCostDelta_NoChange(t *testing.T) {
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{TotalCost: 5}}
	usage := &relaymodel.Usage{ToolsCost: 7}
	updated := applyMCPToolCostDelta(usage, 5, summary)
	require.Equal(t, int64(7), updated.ToolsCost)
}

// TestEstimateMCPRoundPreConsumeQuota verifies per-round pre-consume estimation.
func TestEstimateMCPRoundPreConsumeQuota(t *testing.T) {
	maxTokens := 120
	request := &relaymodel.GeneralOpenAIRequest{MaxTokens: maxTokens}
	quota := estimateMCPRoundPreConsumeQuota(request, 80, 2.0)
	require.Equal(t, int64((80+maxTokens)*2), quota)
}

// TestEstimateMCPRoundPreConsumeQuota_UsesMaxCompletionTokens verifies max_completion_tokens take precedence.
func TestEstimateMCPRoundPreConsumeQuota_UsesMaxCompletionTokens(t *testing.T) {
	maxCompletion := 64
	request := &relaymodel.GeneralOpenAIRequest{MaxTokens: 120, MaxCompletionTokens: &maxCompletion}
	quota := estimateMCPRoundPreConsumeQuota(request, 10, 1.5)
	require.Equal(t, int64(float64(10+maxCompletion)*1.5), quota)
}

// TestMCPToolRegistry_RebuildRequestTools_UsesSelectedSchema verifies tool definitions rebuild from selected candidates.
func TestMCPToolRegistry_RebuildRequestTools_UsesSelectedSchema(t *testing.T) {
	schemaA := `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`
	schemaB := `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`
	toolA := &model.MCPTool{Name: "web_fetch", Description: "fetch", InputSchema: schemaA}
	toolB := &model.MCPTool{Name: "web_fetch", Description: "fetch", InputSchema: schemaB}

	registry := &mcpToolRegistry{
		candidatesByName: map[string][]mcp.ToolCandidate{
			"web_fetch": {
				{ResolvedTool: mcp.ResolvedTool{Tool: toolA}},
				{ResolvedTool: mcp.ResolvedTool{Tool: toolB}},
			},
		},
		originalTools:  []relaymodel.Tool{{Type: "web_fetch"}},
		toolNameByType: map[string]string{"web_fetch": "web_fetch"},
		selectedIndex:  map[string]int{"web_fetch": 1},
	}
	request := &relaymodel.GeneralOpenAIRequest{Tools: []relaymodel.Tool{{Type: "web_fetch"}}}

	err := registry.rebuildRequestTools(request)
	require.NoError(t, err)
	require.Len(t, request.Tools, 1)
	require.Equal(t, "function", request.Tools[0].Type)
	require.NotNil(t, request.Tools[0].Function)
	params, ok := request.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	properties, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, properties, "url")
}
