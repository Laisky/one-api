package controller

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

// requireClaudePromptTokens returns a checked native quote for valid test inputs
// and fails the test if source decoding or arithmetic validation rejects it.
func requireClaudePromptTokens(t testing.TB, ctx context.Context, request *ClaudeMessagesRequest) int {
	t.Helper()
	tokens, err := getClaudeMessagesPromptTokens(ctx, request)
	require.NoError(t, err)
	return tokens
}

// requireClaudePromptEstimate returns a checked native quote at the supplied
// serialized-size compatibility boundary and fails on unexpected errors.
func requireClaudePromptEstimate(t testing.TB, ctx context.Context, request *ClaudeMessagesRequest, size int) int {
	t.Helper()
	tokens, err := estimateClaudeMessagesPromptTokens(ctx, request, size)
	require.NoError(t, err)
	return tokens
}

// requireClaudeDocumentTokens returns a checked document-only estimate for valid
// traversal fixtures and fails the test on an unexpected validation error.
func requireClaudeDocumentTokens(t testing.TB, request *ClaudeMessagesRequest) int {
	t.Helper()
	tokens, err := countClaudeNativeDocumentAllowance(request)
	require.NoError(t, err)
	return tokens
}
