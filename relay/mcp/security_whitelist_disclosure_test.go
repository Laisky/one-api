package mcp

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestSecurityWhitelistPolicyDisclosure verifies translated effective-policy
// wording rather than treating intentional empty-list allow-all as a bypass.
func TestSecurityWhitelistPolicyDisclosure(t *testing.T) {
	expected := map[string][]string{
		"en": {"All otherwise-permitted tools", "future syncs", "Blacklists"},
		"zh": {"允许所有未被其他策略禁止的工具", "后续同步", "黑名单"},
		"ja": {"他のポリシーで許可されたすべてのツール", "今後の同期", "ブラックリスト"},
		"fr": {"Tous les outils autorisés par les autres règles", "futures synchronisations", "blocage"},
		"es": {"Todas las herramientas permitidas por otras políticas", "futuras sincronizaciones", "bloqueo"},
	}
	for locale, words := range expected {
		raw, err := os.ReadFile("../../web/modern/src/i18n/locales/" + locale + "/mcp.json")
		require.NoError(t, err)
		var root map[string]any
		require.NoError(t, json.Unmarshal(raw, &root))
		fields := root["mcp"].(map[string]any)["edit"].(map[string]any)["fields"].(map[string]any)
		require.Equal(t, words[0], fields["tool_whitelist_empty"])
		require.Contains(t, fields["tool_whitelist_help"], words[1])
		require.Contains(t, fields["tool_whitelist_help"], words[2])
	}
}

// TestSecurityWhitelistFutureSyncCompatibility preserves allow-all and every
// blacklist layer when synchronization adds previously unknown tools.
func TestSecurityWhitelistFutureSyncCompatibility(t *testing.T) {
	server := &model.MCPServer{Id: 1, Name: "fixture", ToolBlacklist: model.JSONStringSlice{"server-blocked"}}
	tools := []*model.MCPTool{{Name: "existing"}, {Name: "future"}, {Name: "server-blocked"}, {Name: "channel-blocked"}, {Name: "user-blocked"}}
	resolved, err := ResolveTools(server, tools, []string{"channel-blocked"}, []string{"user-blocked"}, nil)
	require.NoError(t, err)
	for _, entry := range resolved {
		require.Equal(t, entry.Tool.Name == "existing" || entry.Tool.Name == "future", entry.Policy.Allowed)
	}
	server.ToolWhitelist = model.JSONStringSlice{"existing", "server-blocked"}
	resolved, err = ResolveTools(server, tools, []string{"channel-blocked"}, []string{"user-blocked"}, nil)
	require.NoError(t, err)
	for _, entry := range resolved {
		require.Equal(t, entry.Tool.Name == "existing", entry.Policy.Allowed)
	}
}
