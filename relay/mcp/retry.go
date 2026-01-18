package mcp

import (
	"context"

	"github.com/Laisky/errors/v2"
)

// CallWithFallback tries MCP tool calls in order and returns the first successful result.
func CallWithFallback(ctx context.Context, candidates []ToolCandidate, call func(context.Context, ToolCandidate) (*CallToolResult, error)) (ToolCandidate, *CallToolResult, error) {
	if len(candidates) == 0 {
		return ToolCandidate{}, nil, errors.New("no mcp tool candidates available")
	}
	if call == nil {
		return ToolCandidate{}, nil, errors.New("call function is nil")
	}

	var lastErr error
	for _, candidate := range candidates {
		result, err := call(ctx, candidate)
		if err == nil {
			return candidate, result, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}
	return ToolCandidate{}, nil, errors.Wrap(lastErr, "mcp tool call failed after retries")
}
