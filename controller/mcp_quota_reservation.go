package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// callMCPWithQuotaReservation applies the same durable admission and settlement
// boundary to both downstream protocols. A paid attempt reserves its exact
// server/tool price from both balances before external work. Once admitted,
// client disconnects cannot cancel the bounded execution or its settlement.
func callMCPWithQuotaReservation(
	ctx context.Context,
	c *gin.Context,
	userID int,
	servers map[int]*model.MCPServer,
	candidates []mcp.ToolCandidate,
	call func(context.Context, mcp.ToolCandidate) (*mcp.CallToolResult, error),
) (mcp.ToolCandidate, *mcp.CallToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var settlementErr error
	return mcp.CallWithFallback(ctx, candidates, func(attemptCtx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
		// A failed refund must never be followed by another paid attempt.
		if settlementErr != nil {
			return nil, settlementErr
		}
		if err := attemptCtx.Err(); err != nil {
			return nil, errors.Wrap(err, "mcp call canceled before admission")
		}
		server := servers[candidate.ServerID]
		if server == nil || candidate.Tool == nil {
			return nil, errors.New("mcp quota admission requires a loaded server and tool")
		}
		cost := resolveToolCost(server, candidate.Tool.Name)
		if cost <= 0 {
			return call(attemptCtx, candidate)
		}
		tokenID := c.GetInt(ctxkey.TokenId)
		if tokenID <= 0 {
			return nil, errors.New("authenticated token required for paid mcp tool")
		}
		token, err := model.GetTokenById(tokenID)
		if err != nil {
			return nil, errors.Wrap(err, "load token for mcp quota admission")
		}
		if token.UserId != userID {
			return nil, errors.New("mcp token owner does not match authenticated user")
		}
		if err := model.PreConsumeTokenQuota(attemptCtx, tokenID, cost); err != nil {
			return nil, errors.Wrap(err, "reserve quota before mcp tool execution")
		}

		timeout := time.Duration(config.MCPToolCallTimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = time.Minute
		}
		executionCtx, cancel := context.WithTimeout(context.WithoutCancel(attemptCtx), timeout)
		result, callErr := call(executionCtx, candidate)
		cancel()
		result = mcp.NormalizeCallToolResult(result)
		if callErr == nil && shouldBillMCPToolResult(result) {
			// The reservation is the final fixed-price debit, not a second charge.
			return result, nil
		}

		var uncertain *mcp.ToolExecutionUncertainError
		if callErr != nil && (errors.As(callErr, &uncertain) || errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded)) {
			// Preserve the client's existing no-replay uncertainty boundary. A
			// possibly executed side effect cannot be made free by losing its reply.
			settlementErr = callErr
			recordMCPRetainedReservation(attemptCtx, c, userID, candidate, cost, callErr)
			return nil, callErr
		}

		// Explicit tool errors, intermediate input_required results and known
		// transport rejection paths release both balances before any fallback.
		refundCtx, refundCancel := context.WithTimeout(context.WithoutCancel(attemptCtx), 5*time.Second)
		refundErr := model.PostConsumeTokenQuota(refundCtx, tokenID, -cost)
		refundCancel()
		if refundErr != nil {
			settlementErr = errors.Wrap(refundErr, "refund mcp quota reservation; fallback stopped")
			recordMCPRetainedReservation(attemptCtx, c, userID, candidate, cost, settlementErr)
			return nil, settlementErr
		}
		return result, callErr
	})
}

// recordMCPRetainedReservation leaves a durable audit row for a reservation
// whose outcome requires reconciliation, without calling it successful usage.
func recordMCPRetainedReservation(ctx context.Context, c *gin.Context, userID int, candidate mcp.ToolCandidate, cost int64, cause error) {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	name := candidate.ServerLabel + "." + candidate.Tool.Name
	gmw.GetLogger(c).Error("mcp reservation retained; reconciliation required",
		zap.Int("user_id", userID), zap.Int("token_id", c.GetInt(ctxkey.TokenId)),
		zap.Int64("quota", cost), zap.Error(cause))
	model.RecordToolLog(auditCtx, &model.Log{
		UserId:    userID,
		UserUUID:  model.StringPtrIfNotEmpty(c.GetString(ctxkey.UserUUID)),
		TokenUUID: model.StringPtrIfNotEmpty(c.GetString(ctxkey.TokenUUID)),
		ModelName: name,
		Quota:     int(cost),
		Content:   fmt.Sprintf("MCP reservation retained; reconciliation required: %s", name),
		RequestId: c.GetString(ctxkey.RequestId),
	})
}
