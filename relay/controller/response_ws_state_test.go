package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/state"
)

// TestCommitWebSocketObservedResponses verifies a store=true response observed on
// a native Responses websocket is committed under its upstream id and is
// retrievable afterwards, idempotently (proposal ST-011, rows WS04/S05).
func TestCommitWebSocketObservedResponses(t *testing.T) {
	store := enableStateForTest(t)
	c, _ := newBridgeTestContext(t)

	meta := testMeta()
	meta.ActualModelName = "gpt-5"

	resp := &openai.ResponseAPIResponse{
		Id:     "resp_upstream_abc",
		Model:  "gpt-5",
		Status: "completed",
		Output: []openai.OutputItem{{
			Type:    "message",
			Role:    "assistant",
			Status:  "completed",
			Content: []openai.OutputContent{{Type: "output_text", Text: "hello ws"}},
		}},
		Usage: &openai.ResponseAPIUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
	}

	commitWebSocketObservedResponses(c, meta, []*openai.ResponseAPIResponse{resp})

	rec, err := store.GetResponse(context.Background(), testOwner(), "resp_upstream_abc")
	require.NoError(t, err)
	require.NotNil(t, rec.Binding)
	require.Equal(t, "resp_upstream_abc", rec.Binding.UpstreamResponseID)
	require.Equal(t, meta.ChannelId, rec.Binding.ChannelID)
	require.Len(t, rec.OutputItems, 1)
	require.True(t, rec.StoreMode)
	require.Equal(t, state.StatusCompleted, rec.Status)
	createdAt := rec.CreatedAt

	// Idempotent: a re-observed response does not create a second/altered node.
	commitWebSocketObservedResponses(c, meta, []*openai.ResponseAPIResponse{resp})
	rec2, err := store.GetResponse(context.Background(), testOwner(), "resp_upstream_abc")
	require.NoError(t, err)
	require.Equal(t, createdAt, rec2.CreatedAt)
}

// TestCommitWebSocketObservedResponses_DisabledIsNoOp verifies nothing is stored
// when the feature is disabled (row O01).
func TestCommitWebSocketObservedResponses_DisabledIsNoOp(t *testing.T) {
	require.False(t, state.Enabled())
	c, _ := newBridgeTestContext(t)
	meta := testMeta()
	// Must not panic and must not attempt any store access.
	commitWebSocketObservedResponses(c, meta, []*openai.ResponseAPIResponse{{Id: "resp_x"}})
}
