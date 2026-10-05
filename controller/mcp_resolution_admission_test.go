package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// TestMCPResolutionIgnoresUnrelatedSecrets proves that a qualified tool never
// decrypts another server's credentials, even when that unrelated row is corrupt.
// Both the legacy and modern public call implementations must remain usable.
func TestMCPResolutionIgnoresUnrelatedSecrets(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			name := "legacy"
			if modern {
				name = "modern"
			}
			if enabled {
				name += "/enabled"
			} else {
				name += "/disabled"
			}
			t.Run(name, func(t *testing.T) {
				cleanup, fx := setupMCPProxyTest(t)
				defer cleanup()
				other := &model.MCPServer{Name: "unrelated", BaseURL: fx.upstream.URL, APIKey: "invalid-base64!", Status: model.MCPServerStatusEnabled}
				require.NoError(t, model.DB.Create(other).Error)
				if !enabled {
					require.NoError(t, model.DB.Model(other).Update("status", model.MCPServerStatusDisabled).Error)
				}
				c, _ := newMCPCallContext(t, fx.user.Id, "unrelated-secret")
				var err error
				if modern {
					_, err = executeModernMCPTool(context.Background(), c, modernMCPCallParams{Name: "fake-mcp.echo"})
				} else {
					_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: "fake-mcp.echo"})
				}
				require.NoError(t, err, "unrelated broken credentials must not disable a healthy selected tool")
				require.Equal(t, 1, fx.upstreamHits)
			})
		}
	}
}

// TestMCPResolutionAdmissionReadBudget counts real ORM reads, including admission,
// for both entrypoints. Raw oversized input must fail before even a user lookup;
// a dotted unknown name must not multiply SQL reads by its separator count.
func TestMCPResolutionAdmissionReadBudget(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, tc := range []struct {
			name, value string
			maxQueries  int
		}{
			{"oversized", strings.Repeat("a.", 513), 0},
			{"padded_oversized", strings.Repeat(" ", 1025) + "echo", 0},
			{"many_dots", strings.Repeat("a.", 128) + "unknown", 3},
		} {
			mode := "legacy/"
			if modern {
				mode = "modern/"
			}
			t.Run(mode+tc.name, func(t *testing.T) {
				cleanup, fx := setupMCPProxyTest(t)
				defer cleanup()
				queries := 0
				const callback = "review:mcp-query-budget"
				require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(*gorm.DB) { queries++ }))
				defer func() { require.NoError(t, model.DB.Callback().Query().Remove(callback)) }()
				c, _ := newMCPCallContext(t, fx.user.Id, "query-budget")
				var err error
				if modern {
					_, err = executeModernMCPTool(context.Background(), c, modernMCPCallParams{Name: tc.value})
				} else {
					_, err = callMCPToolForUser(context.Background(), c, mcpCallParams{Name: tc.value})
				}
				require.LessOrEqual(t, queries, tc.maxQueries)
				require.Error(t, err)
				require.Zero(t, fx.upstreamHits)
			})
		}
	}
}
