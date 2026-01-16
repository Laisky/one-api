package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
)

func TestResolveTools_PolicyLayers(t *testing.T) {
	server := &model.MCPServer{
		Id:            1,
		Name:          "mcp",
		ToolWhitelist: model.JSONStringSlice{"tool_a", "tool_b"},
		ToolBlacklist: model.JSONStringSlice{"tool_blocked"},
		ToolPricing: model.MCPToolPricingMap{
			"tool_a": {UsdPerCall: 0.01},
		},
	}
	tools := []*model.MCPTool{
		{Name: "tool_a"},
		{Name: "tool_b"},
		{Name: "tool_blocked"},
	}

	resolved, err := ResolveTools(server, tools, []string{"tool_b"}, []string{"tool_x"}, nil)
	require.NoError(t, err)

	allowed := map[string]bool{}
	for _, entry := range resolved {
		allowed[entry.Tool.Name] = entry.Policy.Allowed
	}

	require.True(t, allowed["tool_a"], "tool_a should be allowed")
	require.False(t, allowed["tool_b"], "tool_b should be denied by channel blacklist")
	require.False(t, allowed["tool_blocked"], "tool_blocked should be denied by server blacklist")
}

func TestResolveTools_AllowedToolsFilter(t *testing.T) {
	server := &model.MCPServer{
		Id:            1,
		Name:          "mcp",
		ToolWhitelist: model.JSONStringSlice{"tool_a", "tool_b"},
	}
	tools := []*model.MCPTool{
		{Name: "tool_a"},
		{Name: "tool_b"},
	}

	resolved, err := ResolveTools(server, tools, nil, nil, []string{"tool_b"})
	require.NoError(t, err)

	allowed := map[string]bool{}
	for _, entry := range resolved {
		allowed[entry.Tool.Name] = entry.Policy.Allowed
	}

	require.False(t, allowed["tool_a"], "tool_a should be filtered out by allowed_tools")
	require.True(t, allowed["tool_b"], "tool_b should be allowed")
}

func TestResolveTools_EmptyWhitelistDeniesAll(t *testing.T) {
	server := &model.MCPServer{
		Id:   1,
		Name: "mcp",
	}
	tools := []*model.MCPTool{{Name: "tool_a"}}

	resolved, err := ResolveTools(server, tools, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	require.False(t, resolved[0].Policy.Allowed, "tool should be denied when whitelist is empty")
}

func TestBuildToolCandidates_PriorityOrdering(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo": map[string]any{"type": "string"},
		},
	}
	schemaBytes, err := json.Marshal(schema)
	require.NoError(t, err)

	servers := []*model.MCPServer{
		{Id: 1, Name: "mcp_high", Priority: 10, ToolWhitelist: model.JSONStringSlice{"tool_a"}},
		{Id: 2, Name: "mcp_low", Priority: 1, ToolWhitelist: model.JSONStringSlice{"tool_a"}},
	}
	toolsByServer := map[int][]*model.MCPTool{
		1: {{Name: "tool_a", InputSchema: string(schemaBytes)}},
		2: {{Name: "tool_a", InputSchema: string(schemaBytes)}},
	}

	candidates, err := BuildToolCandidates(servers, toolsByServer, nil, nil, []string{"tool_a"}, "tool_a", "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, 1, candidates[0].ServerID)
	require.Equal(t, 2, candidates[1].ServerID)
}

func TestBuildToolCandidates_SignatureDisambiguation(t *testing.T) {
	schemaA := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo": map[string]any{"type": "string"},
		},
	}
	schemaB := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bar": map[string]any{"type": "string"},
		},
	}
	schemaBytesA, err := json.Marshal(schemaA)
	require.NoError(t, err)
	schemaBytesB, err := json.Marshal(schemaB)
	require.NoError(t, err)
	signatureA, err := SignatureFromSchema(schemaA)
	require.NoError(t, err)

	servers := []*model.MCPServer{
		{Id: 1, Name: "mcp_a", Priority: 1, ToolWhitelist: model.JSONStringSlice{"tool_a"}},
		{Id: 2, Name: "mcp_b", Priority: 1, ToolWhitelist: model.JSONStringSlice{"tool_a"}},
	}
	toolsByServer := map[int][]*model.MCPTool{
		1: {{Name: "tool_a", InputSchema: string(schemaBytesA)}},
		2: {{Name: "tool_a", InputSchema: string(schemaBytesB)}},
	}

	_, err = BuildToolCandidates(servers, toolsByServer, nil, nil, []string{"tool_a"}, "tool_a", "")
	require.Error(t, err)

	candidates, err := BuildToolCandidates(servers, toolsByServer, nil, nil, []string{"tool_a"}, "tool_a", signatureA)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, 1, candidates[0].ServerID)
}
