package controller

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCPReservationFallbackPricesEachAttempt verifies rollback before fallback
// and the final selected server's exact price, including with batch mode enabled.
func TestMCPReservationFallbackPricesEachAttempt(t *testing.T) {
	fx, token := setupMCPBillingBoundary(t)
	config.BatchUpdateEnabled = true
	c, _ := newMCPCallContext(t, fx.user.Id, "fallback-pricing")
	c.Set(ctxkey.TokenId, token.Id)
	second := *fx.server
	second.Id = 2
	second.Name = "second-mcp"
	second.ToolPricing = model.MCPToolPricingMap{"echo": {QuotaPerCall: 120}}
	secondTool := *fx.tool
	secondTool.ServerId = second.Id
	servers := map[int]*model.MCPServer{fx.server.Id: fx.server, second.Id: &second}
	candidates := []mcp.ToolCandidate{
		{ServerID: fx.server.Id, ServerLabel: fx.server.Name, Tool: fx.tool},
		{ServerID: second.Id, ServerLabel: second.Name, Tool: &secondTool},
	}
	calls := 0
	selected, result, err := callMCPWithQuotaReservation(context.Background(), c, fx.user.Id, servers, candidates,
		func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
			calls++
			require.NoError(t, ctx.Err())
			_, bounded := ctx.Deadline()
			require.True(t, bounded)
			user, err := model.GetUserById(fx.user.Id, true)
			require.NoError(t, err)
			stored, err := model.GetTokenById(token.Id)
			require.NoError(t, err)
			expected := int64(1000) - resolveToolCost(servers[candidate.ServerID], candidate.Tool.Name)
			require.Equal(t, expected, user.Quota)
			require.Equal(t, expected, stored.RemainQuota, "prior attempt must be refunded before reserving next")
			if calls == 1 {
				return nil, errors.New("known rejection before tool execution")
			}
			return &mcp.CallToolResult{ResultType: "complete"}, nil
		})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, second.Id, selected.ServerID)
	require.Equal(t, 2, calls)
	user, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	stored, err := model.GetTokenById(token.Id)
	require.NoError(t, err)
	require.Equal(t, int64(880), user.Quota)
	require.Equal(t, int64(880), stored.RemainQuota)
	require.Equal(t, int64(120), stored.UsedQuota)
}

// TestMCPReservationTerminalStates checks bounded execution, refunds, uncertain
// results, and refund failures against persistent balances and audit records.
func TestMCPReservationTerminalStates(t *testing.T) {
	for _, outcome := range []string{"input_required", "uncertain", "refund_failure", "pre_canceled", "unlimited"} {
		t.Run(outcome, func(t *testing.T) {
			fx, token := setupMCPBillingBoundary(t)
			c, _ := newMCPCallContext(t, fx.user.Id, "terminal-"+outcome)
			c.Set(ctxkey.TokenId, token.Id)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if outcome == "pre_canceled" {
				cancel()
			}
			if outcome == "unlimited" {
				require.NoError(t, model.DB.Model(token).Update("unlimited_quota", true).Error)
			}
			candidate := mcp.ToolCandidate{ServerID: fx.server.Id, ServerLabel: fx.server.Name, Tool: fx.tool}
			calls := 0
			_, result, err := callMCPWithQuotaReservation(ctx, c, fx.user.Id,
				map[int]*model.MCPServer{fx.server.Id: fx.server}, []mcp.ToolCandidate{candidate, candidate},
				func(executionCtx context.Context, _ mcp.ToolCandidate) (*mcp.CallToolResult, error) {
					calls++
					require.NoError(t, executionCtx.Err())
					switch outcome {
					case "input_required":
						return &mcp.CallToolResult{ResultType: "input_required"}, nil
					case "uncertain":
						return nil, context.DeadlineExceeded
					case "refund_failure":
						require.NoError(t, model.DB.Delete(token).Error)
						return nil, errors.New("known rejected execution")
					default:
						return &mcp.CallToolResult{ResultType: "complete"}, nil
					}
				})
			user, loadErr := model.GetUserById(fx.user.Id, true)
				require.NoError(t, loadErr)
			if outcome == "pre_canceled" {
				require.Error(t, err)
				require.Zero(t, calls)
				require.Equal(t, int64(1000), user.Quota)
				return
			}
			require.Equal(t, 1, calls, "uncertainty or refund failure must not execute a fallback")
			if outcome == "uncertain" || outcome == "refund_failure" {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, int64(925), user.Quota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ?", "terminal-"+outcome).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, 75, logs[0].Quota)
				require.Contains(t, logs[0].Content, "reconciliation required")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			stored, loadErr := model.GetTokenById(token.Id)
			require.NoError(t, loadErr)
			require.Equal(t, int64(1000), stored.RemainQuota)
			require.Zero(t, stored.UsedQuota)
			if outcome == "unlimited" {
				require.Equal(t, int64(925), user.Quota, "unlimited token does not mean unlimited user funds")
			} else {
				require.Equal(t, int64(1000), user.Quota)
				require.Equal(t, "input_required", result.ResultType)
			}
		})
	}
}
