package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/state"
)

// TestResolveNativePreviousResponseSameProviderRewrite covers ST-021 rows M05/PERF02:
// a gateway previous_response_id bound to the same native provider is rewritten to
// the upstream handle so only incremental input is sent, and the request stays
// native.
func TestResolveNativePreviousResponseSameProviderRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta() // ChannelId:1, APIType:0

	parentGw := mustNewResponseID(t)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: parentGw,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		StoreMode:         true,
		Binding: &state.ProviderBinding{
			ChannelID:          meta.ChannelId,
			APIType:            meta.APIType,
			ActualModel:        "gpt-5",
			UpstreamResponseID: "resp_upstream_parent",
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	prev := parentGw
	req := &openai.ResponseAPIRequest{Model: "gpt-5", PreviousResponseId: &prev, Input: openai.ResponseAPIInput{"hi"}}

	divert, gwErr := resolveNativePreviousResponse(c, meta, req)
	require.Nil(t, gwErr)
	require.False(t, divert)
	require.NotNil(t, req.PreviousResponseId)
	require.Equal(t, "resp_upstream_parent", *req.PreviousResponseId, "must rewrite to the upstream handle")
	require.Equal(t, parentGw, c.GetString(ctxNativeGatewayParent), "original gateway parent is preserved for chaining")
}

// TestResolveNativePreviousResponseDifferentProviderDiverts covers row C08: a
// gateway parent bound to a different provider cannot be honored natively, so the
// request diverts to the hydrating fallback and the selector is left intact.
func TestResolveNativePreviousResponseDifferentProviderDiverts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	parentGw := mustNewResponseID(t)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: parentGw,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		StoreMode:         true,
		Binding: &state.ProviderBinding{
			ChannelID:          meta.ChannelId + 999,
			APIType:            meta.APIType,
			UpstreamResponseID: "resp_other",
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	prev := parentGw
	req := &openai.ResponseAPIRequest{Model: "gpt-5", PreviousResponseId: &prev, Input: openai.ResponseAPIInput{"hi"}}

	divert, gwErr := resolveNativePreviousResponse(c, meta, req)
	require.Nil(t, gwErr)
	require.True(t, divert)
	require.Equal(t, parentGw, *req.PreviousResponseId, "selector unchanged so the fallback hydrator can resolve it")
}

// TestResolveNativePreviousResponseRawIDUntouched covers row B07: a non-gateway
// (raw/legacy) previous_response_id is forwarded verbatim.
func TestResolveNativePreviousResponseRawIDUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)
	meta := testMeta()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	prev := "resp_raw_from_before_the_feature"
	req := &openai.ResponseAPIRequest{Model: "gpt-5", PreviousResponseId: &prev, Input: openai.ResponseAPIInput{"hi"}}

	divert, gwErr := resolveNativePreviousResponse(c, meta, req)
	require.Nil(t, gwErr)
	require.False(t, divert)
	require.Equal(t, "resp_raw_from_before_the_feature", *req.PreviousResponseId)
}

// TestCommitNativeResponseState covers ST-021 rows STR01/M05: a native Responses
// result is committed under its upstream id with an upstream-handle binding, so it
// is retrievable over HTTP and can back a same-provider continuation.
func TestCommitNativeResponseState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &openai.ResponseAPIRequest{Model: "gpt-5", Input: openai.ResponseAPIInput{"hello"}}
	capturePendingStateCommit(c, meta, req)

	c.Set(ctxkey.ConvertedResponse, openai.ResponseAPIResponse{
		Id:     "resp_native_commit_1",
		Status: "completed",
		Model:  "gpt-5",
		Output: []openai.OutputItem{{Type: "message", Role: "assistant"}},
		Usage:  &openai.ResponseAPIUsage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8},
	})

	commitNativeResponseState(c, meta)

	got, err := store.GetResponse(context.Background(), testOwner(), "resp_native_commit_1")
	require.NoError(t, err)
	require.Equal(t, "resp_native_commit_1", got.GatewayResponseID)
	require.NotNil(t, got.Binding)
	require.Equal(t, "resp_native_commit_1", got.Binding.UpstreamResponseID)
	require.Equal(t, meta.ChannelId, got.Binding.ChannelID)
	require.Len(t, got.OutputItems, 1)
	require.Len(t, got.InputItems, 1)

	// Idempotent on the upstream id: a re-observation does not double-write.
	commitNativeResponseState(c, meta)
	again, err := store.GetResponse(context.Background(), testOwner(), "resp_native_commit_1")
	require.NoError(t, err)
	require.Equal(t, got.GatewayResponseID, again.GatewayResponseID)
}
