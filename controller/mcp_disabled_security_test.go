package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCPDisabledServersFailClosed checks both protocol paths, upstream isolation,
// and unchanged quota. t supplies assertions; the test returns nothing.
func TestMCPDisabledServersFailClosed(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "modern"}[modern], func(t *testing.T) {
			cleanup, fx := setupMCPProxyTest(t)
			defer cleanup()
			require.NoError(t, model.DB.Model(fx.server).Update("status", model.MCPServerStatusDisabled).Error)
			c, _ := newMCPCallContext(t, fx.user.Id, "disabled-server")
			var err error
			if modern {
				_, err = executeModernMCPTool(context.Background(), c, modernMCPCallParams{Name: "fake-mcp.echo"})
			} else {
				_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: "fake-mcp.echo"})
			}
			require.Error(t, err)
			require.Zero(t, fx.upstreamHits)
			var user model.User
			require.NoError(t, model.DB.First(&user, fx.user.Id).Error)
			require.Equal(t, fx.user.Quota, user.Quota)
		})
	}
}

// TestMCPDisabledLongestPrefixCannotFallBack verifies that a disabled dotted
// server cannot fall back to a shorter enabled server with a dotted tool name.
func TestMCPDisabledLongestPrefixCannotFallBack(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()
	require.NoError(t, model.DB.Model(fx.server).Updates(map[string]any{
		"name": "service", "tool_whitelist": model.JSONStringSlice{"sub.echo"},
	}).Error)
	require.NoError(t, model.DB.Model(fx.tool).Update("name", "sub.echo").Error)
	disabled := &model.MCPServer{Name: "service.sub", BaseURL: fx.upstream.URL}
	require.NoError(t, model.DB.Create(disabled).Error)
	require.NoError(t, model.DB.Model(disabled).Update("status", model.MCPServerStatusDisabled).Error)
	c, _ := newMCPCallContext(t, fx.user.Id, "disabled-longest-prefix")
	_, err := prepareModernMCPToolCall(c, modernMCPCallParams{Name: "service.sub.echo"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
	require.Zero(t, fx.upstreamHits)
}
