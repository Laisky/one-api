package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// TestMCPResolutionBoundsDatabaseWork checks that separators and oversized raw
// names cannot amplify server queries. t supplies assertions; it returns nothing.
func TestMCPResolutionBoundsDatabaseWork(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()
	reads := 0
	const callback = "test:bounded-mcp-resolution"
	countServerRead := func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" {
			reads++
		}
	}
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, countServerRead))
	require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register(callback, countServerRead))
	defer func() {
		require.NoError(t, model.DB.Callback().Query().Remove(callback))
		require.NoError(t, model.DB.Callback().Row().Remove(callback))
	}()
	for _, dots := range []int{2, 20, 100} {
		for _, modern := range []bool{false, true} {
			reads = 0
			c, _ := newMCPCallContext(t, fx.user.Id, "bounded-resolution")
			name := strings.Repeat("unknown.", dots) + "tool"
			var err error
			if modern {
				_, err = prepareModernMCPToolCall(c, modernMCPCallParams{Name: name})
			} else {
				_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: name})
			}
			require.Error(t, err)
			require.LessOrEqual(t, reads, 2, "separators must not control query count")
		}
	}
	for _, modern := range []bool{false, true} {
		reads = 0
		c, _ := newMCPCallContext(t, fx.user.Id, "oversized-resolution")
		name := strings.Repeat(" ", 1025) + "fake-mcp.echo"
		var err error
		if modern {
			_, err = prepareModernMCPToolCall(c, modernMCPCallParams{Name: name})
		} else {
			_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: name})
		}
		require.Error(t, err)
		require.Zero(t, reads, "reject raw byte length before server reads")
	}
	require.Zero(t, fx.upstreamHits)
}

// TestMCPResolutionDoesNotDecryptUnrelatedServers proves that an unrelated
// disabled server's corrupt secret cannot poison a valid qualified tool call.
func TestMCPResolutionDoesNotDecryptUnrelatedServers(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()
	c, _ := newMCPCallContext(t, fx.user.Id, "resolution-control")
	_, err := prepareModernMCPToolCall(c, modernMCPCallParams{Name: "fake-mcp.echo"})
	require.NoError(t, err, "the control must resolve before fault injection")
	other := &model.MCPServer{Name: "unrelated", BaseURL: fx.upstream.URL, Status: model.MCPServerStatusEnabled}
	require.NoError(t, model.DB.Create(other).Error)
	require.NoError(t, model.DB.Model(other).Updates(map[string]any{
		"status": model.MCPServerStatusDisabled, "api_key": "not-valid-base64!",
	}).Error)
	c, _ = newMCPCallContext(t, fx.user.Id, "resolution-isolation")
	plan, err := prepareModernMCPToolCall(c, modernMCPCallParams{Name: "fake-mcp.echo"})
	require.NoError(t, err, "an unrelated disabled server must not poison resolution")
	require.Len(t, plan.candidates, 1)
	require.Contains(t, plan.serverByID, fx.server.Id)
}

// TestMCPToolNameByteLimit checks the raw byte limit, including UTF-8 and padding.
// t supplies assertions; no unqualified resolution needs a database lookup.
func TestMCPToolNameByteLimit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		rejected bool
	}{
		{"ASCII boundary", strings.Repeat("a", 1024), false},
		{"ASCII oversized", strings.Repeat("a", 1025), true},
		{"UTF8 boundary", strings.Repeat("é", 512), false},
		{"UTF8 oversized", strings.Repeat("é", 513), true},
		{"padding cannot bypass", strings.Repeat(" ", 1024) + "a", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			label, tool, err := resolveQualifiedToolName(context.Background(), tc.value)
			if tc.rejected {
				require.Error(t, err)
				require.Empty(t, label)
				require.Empty(t, tool)
				return
			}
			require.NoError(t, err)
			require.Empty(t, label)
			require.Equal(t, tc.value, tool)
		})
	}
}
