package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestCallMCPToolForUser_InsufficientTokenQuotaDoesNotInvokeUpstream ensures
// paid MCP calls enforce token quota before executing side-effecting tools.
func TestCallMCPToolForUser_InsufficientTokenQuotaDoesNotInvokeUpstream(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	fx.setToolPricing(t, "echo", model.ToolPricingLocal{QuotaPerCall: 75})
	require.NoError(t, model.DB.Model(&model.Token{}).
		Where("id = ?", fx.token.Id).
		Updates(map[string]any{"remain_quota": int64(1), "used_quota": int64(0)}).Error)

	c, _ := newMCPCallContext(t, fx.user.Id, "req-token-low")
	result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name:      "fake-mcp.echo",
		Arguments: map[string]any{"input": "blocked"},
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "insufficient token quota")
	require.Equal(t, 0, fx.upstreamHits, "quota rejection must happen before upstream execution")

	refreshed, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(1000), refreshed.Quota)
	require.Equal(t, int64(0), refreshed.UsedQuota)
	require.Equal(t, 0, refreshed.RequestCount)

	chargedToken, err := model.GetTokenById(fx.token.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1), chargedToken.RemainQuota)
	require.Equal(t, int64(0), chargedToken.UsedQuota)
}

// TestCallMCPToolForUser_InsufficientUserQuotaDoesNotInvokeUpstream ensures
// paid MCP calls enforce user quota before executing side-effecting tools.
func TestCallMCPToolForUser_InsufficientUserQuotaDoesNotInvokeUpstream(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	fx.setToolPricing(t, "echo", model.ToolPricingLocal{QuotaPerCall: 75})
	require.NoError(t, model.DB.Model(&model.User{}).
		Where("id = ?", fx.user.Id).
		Update("quota", int64(10)).Error)

	c, _ := newMCPCallContext(t, fx.user.Id, "req-user-low")
	result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name:      "fake-mcp.echo",
		Arguments: map[string]any{"input": "blocked"},
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "insufficient user quota")
	require.Equal(t, 0, fx.upstreamHits, "quota rejection must happen before upstream execution")

	refreshed, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(10), refreshed.Quota)
	require.Equal(t, int64(0), refreshed.UsedQuota)
	require.Equal(t, 0, refreshed.RequestCount)

	chargedToken, err := model.GetTokenById(fx.token.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1000), chargedToken.RemainQuota)
	require.Equal(t, int64(0), chargedToken.UsedQuota)
}

// TestMCPProxy_ToolsList_NoWhitelistReturnsAllSyncedTools is the regression
// guard for the second leg of issue #340: a user configures an MCP server,
// runs sync (which never populates ToolWhitelist), then connects MCP
// Inspector. Before the fix, an empty whitelist denied every tool and the
// Inspector saw `{"tools":[]}` despite the DB containing many tool rows.
// After the fix, an empty whitelist applies no whitelist filter and tools
// flow through subject only to blacklists.
func TestMCPProxy_ToolsList_NoWhitelistReturnsAllSyncedTools(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	// Clear the fixture-default whitelist to mirror a freshly synced server.
	require.NoError(t, model.DB.Model(&model.MCPServer{}).
		Where("id = ?", fx.server.Id).
		Update("tool_whitelist", nil).Error)
	reloadedServer, err := model.GetMCPServerByName(fx.server.Name)
	require.NoError(t, err)
	require.Empty(t, reloadedServer.ToolWhitelist,
		"precondition: server must have empty whitelist to reproduce issue #340")
	fx.server = reloadedServer

	// Seed two more tools to mimic a real sync result. Combined with the
	// fixture's seeded `echo` row, the registry now holds three tools.
	for _, extra := range []*model.MCPTool{
		{Id: 11, ServerId: fx.server.Id, Name: "search", DisplayName: "Search",
			Description: "Search the web", InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`, Status: 1},
		{Id: 12, ServerId: fx.server.Id, Name: "fetch", DisplayName: "Fetch",
			Description: "Fetch a URL", InputSchema: `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`, Status: 1},
	} {
		require.NoError(t, model.DB.Create(extra).Error)
	}

	recorder := invokeMCPProxy(t, fx, http.MethodPost, "req-no-whitelist",
		`{"jsonrpc":"2.0","id":99,"method":"tools/list"}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	raw := recorder.Body.String()
	require.NotContains(t, raw, `"tools":[]`,
		"empty tools array means the empty-whitelist deny-all bug is back — issue #340")
	require.NotContains(t, raw, `"tools":null`)

	var resp struct {
		Result struct {
			Tools []mcp.ToolDescriptor `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	names := make([]string, 0, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t,
		[]string{"fake-mcp.echo", "fake-mcp.search", "fake-mcp.fetch"},
		names,
		"all synced tools must be returned when whitelist is empty")
}

// TestMCPProxy_ToolsList_NoWhitelistStillRespectsBlacklists asserts the
// empty-whitelist relaxation does not bypass the user MCPToolBlacklist —
// privacy/policy filters must still apply.
func TestMCPProxy_ToolsList_NoWhitelistStillRespectsBlacklists(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	require.NoError(t, model.DB.Model(&model.MCPServer{}).
		Where("id = ?", fx.server.Id).
		Update("tool_whitelist", nil).Error)
	reloadedServer, err := model.GetMCPServerByName(fx.server.Name)
	require.NoError(t, err)
	fx.server = reloadedServer

	require.NoError(t, model.DB.Create(&model.MCPTool{
		Id: 21, ServerId: fx.server.Id, Name: "dangerous", DisplayName: "Dangerous",
		Description: "Should be blocked", InputSchema: `{"type":"object"}`, Status: 1,
	}).Error)

	fx.user.MCPToolBlacklist = model.JSONStringSlice{"dangerous"}
	require.NoError(t, model.DB.Save(fx.user).Error)
	reloadedUser, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	fx.user = reloadedUser

	recorder := invokeMCPProxy(t, fx, http.MethodPost, "req-blacklist-stillworks",
		`{"jsonrpc":"2.0","id":100,"method":"tools/list"}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Result struct {
			Tools []mcp.ToolDescriptor `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	names := make([]string, 0, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "fake-mcp.echo")
	require.NotContains(t, names, "fake-mcp.dangerous",
		"user blacklist must still filter even when whitelist is empty")
}
