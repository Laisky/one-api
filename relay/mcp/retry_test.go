package mcp

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

func TestCallWithFallback_SucceedsOnSecond(t *testing.T) {
	ctx := context.Background()
	candidates := []ToolCandidate{{ResolvedTool: ResolvedTool{ServerID: 1}}, {ResolvedTool: ResolvedTool{ServerID: 2}}}
	calls := 0
	selected, result, err := CallWithFallback(ctx, candidates, func(_ context.Context, candidate ToolCandidate) (*CallToolResult, error) {
		calls++
		if candidate.ServerID == 1 {
			return nil, errors.New("first failure")
		}
		return &CallToolResult{Content: "ok"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, selected.ServerID)
	require.NotNil(t, result)
	require.Equal(t, "ok", result.Content)
	require.Equal(t, 2, calls)
}

func TestCallWithFallback_ContextCanceledStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	candidates := []ToolCandidate{{ResolvedTool: ResolvedTool{ServerID: 1}}, {ResolvedTool: ResolvedTool{ServerID: 2}}}
	calls := 0
	_, _, err := CallWithFallback(ctx, candidates, func(_ context.Context, _ ToolCandidate) (*CallToolResult, error) {
		calls++
		return nil, context.Canceled
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Equal(t, 1, calls)
}
