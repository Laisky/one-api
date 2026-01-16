package mcp

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/model"
)

// ToolPolicySnapshot captures policy and pricing decisions for a MCP tool.
type ToolPolicySnapshot struct {
	Allowed bool
	Pricing model.ToolPricingLocal
}

// ResolvedTool represents an MCP tool with policy applied.
type ResolvedTool struct {
	Tool        *model.MCPTool
	Policy      ToolPolicySnapshot
	ServerID    int
	ServerLabel string
	ServerURL   string
	DisplayName string
}

// ResolveTools applies layered policies (server/channel/user/allowed list) to MCP tools.
func ResolveTools(server *model.MCPServer, tools []*model.MCPTool, channelBlacklist []string, userBlacklist []string, allowedTools []string) ([]ResolvedTool, error) {
	if server == nil {
		return nil, errors.New("mcp server is nil")
	}

	allowList := normalizeToolSet(allowedTools)
	serverWhitelist := normalizeToolSet(server.ToolWhitelist)
	serverBlacklist := normalizeToolSet(server.ToolBlacklist)
	channelBlacklistSet := normalizeToolSet(channelBlacklist)
	userBlacklistSet := normalizeToolSet(userBlacklist)

	resolved := make([]ResolvedTool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := normalizeToolName(tool.Name)
		serverKey := normalizeToolName(server.Name + "." + tool.Name)
		if name == "" {
			continue
		}

		allowed := true
		if len(serverWhitelist) == 0 {
			allowed = false
		}
		if _, ok := serverWhitelist[name]; !ok {
			allowed = false
		}
		if _, ok := serverBlacklist[name]; ok {
			allowed = false
		}
		if toolInSet(channelBlacklistSet, name) || toolInSet(channelBlacklistSet, serverKey) {
			allowed = false
		}
		if toolInSet(userBlacklistSet, name) || toolInSet(userBlacklistSet, serverKey) {
			allowed = false
		}
		if len(allowList) > 0 {
			if !toolInSet(allowList, name) && !toolInSet(allowList, serverKey) {
				allowed = false
			}
		}

		pricing := server.ToolPricing[name]
		resolved = append(resolved, ResolvedTool{
			Tool:        tool,
			Policy:      ToolPolicySnapshot{Allowed: allowed, Pricing: pricing},
			ServerID:    server.Id,
			ServerLabel: server.Name,
			ServerURL:   server.BaseURL,
			DisplayName: tool.DisplayName,
		})
	}

	return resolved, nil
}

// normalizeToolSet builds a canonical set of tool names.
func normalizeToolSet(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(list))
	for _, raw := range list {
		name := normalizeToolName(raw)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

// normalizeToolName standardizes a tool name for policy comparisons.
func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// toolInSet reports whether a normalized tool name exists in a set.
func toolInSet(set map[string]struct{}, name string) bool {
	if len(set) == 0 {
		return false
	}
	if name == "" {
		return false
	}
	_, ok := set[name]
	return ok
}
