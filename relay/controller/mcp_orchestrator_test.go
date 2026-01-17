package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
)

// TestNormalizeChatToolChoiceForMCP verifies MCP tool choices are normalized to function.
func TestNormalizeChatToolChoiceForMCP(t *testing.T) {
	mcpNames := map[string]struct{}{ "web_search_20250305": {} }
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
	mcpNames := map[string]struct{}{ "web_search_20250305": {} }
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
		TotalCost: 10,
		Counts:     map[string]int{"web_search": 1},
		CostByTool: map[string]int64{"web_search": 10},
		Entries: []model.ToolUsageEntry{
			{Tool: "web_search", Source: "channel_builtin", ServerID: 0, Count: 1, Cost: 10},
		},
	}
	addition := &model.ToolUsageSummary{
		TotalCost: 5,
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
