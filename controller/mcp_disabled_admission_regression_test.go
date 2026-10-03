package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCPDisabledServerAdmission checks both public call implementations against
// real HTTP and SQLite fixtures. Disabled or invalid-status servers must cause
// neither an upstream handshake nor a tool invocation, quota change, or log.
func TestMCPDisabledServerAdmission(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			status int
			deny   bool
		}{
			{"enabled", model.MCPServerStatusEnabled, false},
			{"disabled", model.MCPServerStatusDisabled, true},
			{"invalid_status", 7, true},
		} {
			mode := "legacy/"
			if modern {
				mode = "modern/"
			}
			t.Run(mode+tc.name, func(t *testing.T) {
				cleanup, fx := setupMCPProxyTest(t)
				defer cleanup()
				var requests atomic.Int64
				counted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					fx.upstream.Config.Handler.ServeHTTP(w, r)
				}))
				defer counted.Close()
				require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("id = ?", fx.server.Id).
					Updates(map[string]any{"status": tc.status, "base_url": counted.URL}).Error)
				c, _ := newMCPCallContext(t, fx.user.Id, "disabled-server-admission")
				var err error
				if modern {
					_, err = executeModernMCPTool(context.Background(), c, modernMCPCallParams{Name: "fake-mcp.echo"})
				} else {
					_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: "fake-mcp.echo"})
				}
				if tc.deny {
					require.Zero(t, requests.Load(), "denied server must not receive even initialize")
					require.Error(t, err)
					require.Zero(t, fx.upstreamHits)
					var logs int64
					require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logs).Error)
					require.Zero(t, logs)
				} else {
					require.NoError(t, err)
					require.Equal(t, 1, fx.upstreamHits)
					require.Positive(t, requests.Load(), "positive control must contact the real HTTP fixture")
				}
				var owner model.User
				require.NoError(t, model.DB.First(&owner, fx.user.Id).Error)
				require.Equal(t, fx.user.Quota, owner.Quota)
			})
		}
	}
}
