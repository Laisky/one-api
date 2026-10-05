package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// TestSecurityResponseFallbackRejectsBeforeStateAccess supplies nil state/context
// holders intentionally: malformed or excessive input must return before any
// snapshot, hydration or upstream access. t receives failures; no value returns.
func TestSecurityResponseFallbackRejectsBeforeStateAccess(t *testing.T) {
	request := &openai.ResponseAPIRequest{Input: make([]any, openai.MaxResponseAPIFallbackInputItems+1)}
	_, failure := prepareResponseFallbackInput(context.Background(), nil, nil, request)
	if failure == nil {
		require.FailNow(t, "oversized input reached state handling")
	}
	_, failure = prepareResponseFallbackInput(context.Background(), nil, nil, nil)
	if failure == nil {
		require.FailNow(t, "nil input reached state handling")
	}
}

// TestSecurityResponseFallbackCancelledBeforeStateAccess ensures cancellation is
// observed before optional snapshot or state hydration work begins.
func TestSecurityResponseFallbackCancelledBeforeStateAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, failure := prepareResponseFallbackInput(ctx, nil, nil, &openai.ResponseAPIRequest{})
	require.NotNil(t, failure)
	require.Equal(t, 408, failure.StatusCode)
}
