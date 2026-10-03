package controller

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCPReservationConcurrentAdmission releases competing calls together and
// verifies that only funded work executes, with batch mode both off and on.
// t owns the isolated database and reports persistent balance mismatches.
func TestMCPReservationConcurrentAdmission(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batch), func(t *testing.T) {
			fx, token := setupMCPBillingBoundary(t)
			config.BatchUpdateEnabled = batch
			require.NoError(t, model.DB.Model(fx.user).Update("quota", 300).Error)
			require.NoError(t, model.DB.Model(token).Update("remain_quota", 300).Error)
			candidate := mcp.ToolCandidate{ServerID: fx.server.Id, ServerLabel: fx.server.Name, Tool: fx.tool}
			servers := map[int]*model.MCPServer{fx.server.Id: fx.server}
			start := make(chan struct{})
			results := make(chan error, 32)
			var executed atomic.Int64
			var workers sync.WaitGroup
			for i := 0; i < cap(results); i++ {
				c, _ := newMCPCallContext(t, fx.user.Id, fmt.Sprintf("concurrent-%d", i))
				c.Set(ctxkey.TokenId, token.Id)
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					_, _, err := callMCPWithQuotaReservation(context.Background(), c, fx.user.Id, servers,
						[]mcp.ToolCandidate{candidate}, func(context.Context, mcp.ToolCandidate) (*mcp.CallToolResult, error) {
							executed.Add(1)
							return &mcp.CallToolResult{ResultType: mcp.ResultTypeComplete}, nil
						})
					results <- err
				}()
			}
			close(start)
			workers.Wait()
			close(results)
			succeeded := 0
			for err := range results {
				if err == nil {
					succeeded++
				}
			}
			require.Equal(t, 4, succeeded, "300 quota funds exactly four 75-quota attempts")
			require.Equal(t, int64(4), executed.Load(), "unfunded competitors must not run upstream work")
			user, err := model.GetUserById(fx.user.Id, true)
			require.NoError(t, err)
			stored, err := model.GetTokenById(token.Id)
			require.NoError(t, err)
			require.Zero(t, user.Quota)
			require.Zero(t, stored.RemainQuota)
			require.Equal(t, int64(300), stored.UsedQuota)
		})
	}
}
