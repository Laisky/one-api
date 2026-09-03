package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestDottedServerNameToolsAreCallable pins that a tool on a server whose name
// contains a dot can actually be invoked.
//
// The catalog advertises tools as "<server>.<tool>", and the call path split that
// at the FIRST dot. A server named "github.com" therefore advertised
// "github.com.search_repos" and then looked up a server called "github", which
// does not exist — every tool on that server was listed but permanently
// uncallable, with no validation preventing the name in the first place.
func TestDottedServerNameToolsAreCallable(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	dotted := &model.MCPServer{
		Id:                      2,
		Name:                    "github.com",
		Status:                  model.MCPServerStatusEnabled,
		BaseURL:                 fx.upstream.URL,
		Protocol:                model.MCPProtocolStreamableHTTP,
		AuthType:                model.MCPAuthTypeNone,
		ToolWhitelist:           model.JSONStringSlice{"search_repos"},
		AutoSyncIntervalMinutes: 60,
	}
	require.NoError(t, model.DB.Create(dotted).Error)
	require.NoError(t, model.DB.Create(&model.MCPTool{
		Id:          2,
		ServerId:    dotted.Id,
		Name:        "search_repos",
		DisplayName: "Search repos",
		Description: "Searches repositories",
		InputSchema: `{"type":"object"}`,
		Status:      1,
	}).Error)

	t.Run("resolution prefers the longest matching server name", func(t *testing.T) {
		serverLabel, toolName := resolveQualifiedToolName("github.com.search_repos")
		require.Equal(t, "github.com", serverLabel)
		require.Equal(t, "search_repos", toolName)
	})

	t.Run("a server name without dots still resolves", func(t *testing.T) {
		serverLabel, toolName := resolveQualifiedToolName("fake-mcp.echo")
		require.Equal(t, "fake-mcp", serverLabel)
		require.Equal(t, "echo", toolName)
	})

	t.Run("an unknown qualifier keeps the previous behaviour", func(t *testing.T) {
		serverLabel, toolName := resolveQualifiedToolName("nosuch.tool")
		require.Equal(t, "nosuch", serverLabel)
		require.Equal(t, "tool", toolName)
	})

	t.Run("an unqualified name has no server label", func(t *testing.T) {
		serverLabel, toolName := resolveQualifiedToolName("echo")
		require.Empty(t, serverLabel)
		require.Equal(t, "echo", toolName)
	})

	t.Run("the dotted server's tool actually executes", func(t *testing.T) {
		c, _ := newMCPCallContext(t, fx.user.Id, "req-dotted")
		result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
			Name: "github.com.search_repos",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}
