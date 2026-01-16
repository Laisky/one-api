package mcp

import (
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
