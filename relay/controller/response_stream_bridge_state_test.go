package controller

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/state"
)

// TestChatToResponseStreamBridge_CommitsGatewayResponse verifies that with the
// state layer enabled, a streamed fallback response is committed exactly once at
// the terminal event under a gateway response ID that every event carries, and
// the record is retrievable. Closes B13 (and covers STR01/STR02/STR04).
func TestChatToResponseStreamBridge_CommitsGatewayResponse(t *testing.T) {
	store := enableStateForTest(t)
	c, w := newBridgeTestContext(t)

	meta := testMeta()
	meta.ActualModelName = "gpt-4o-test"
	req := &openai.ResponseAPIRequest{Model: "gpt-4o", Input: openai.ResponseAPIInput{"stream please"}}

	capturePendingStateCommit(c, meta, req)
	handler := newChatToResponseStreamBridge(c, meta, req).(*chatToResponseStreamBridge)

	handler.HandleChunk(c, bridgeTextChunk("Hello"))
	handler.HandleChunk(c, bridgeFinishChunk("stop"))
	handler.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	require.NotEmpty(t, events)

	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1, "exactly one terminal commit")
	var completedEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &completedEv)
	require.NotNil(t, completedEv.Response)
	gwID := completedEv.Response.Id
	require.True(t, state.LooksLikeGatewayResponseID(gwID), "final id %q must be a gateway id", gwID)
	require.NotNil(t, completedEv.Response.Store)
	require.True(t, *completedEv.Response.Store)

	// The response.created event carries the SAME gateway id (STR01).
	created := bridgeFindEvents(events, "response.created")
	require.Len(t, created, 1)
	var createdEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, created[0], &createdEv)
	require.NotNil(t, createdEv.Response)
	require.Equal(t, gwID, createdEv.Response.Id, "every emitted response id must match the committed id")

	// The committed record is retrievable and holds input + output.
	rec, err := store.GetResponse(context.Background(), testOwner(), gwID)
	require.NoError(t, err)
	require.Len(t, rec.InputItems, 1)
	require.GreaterOrEqual(t, len(rec.OutputItems), 1)
}

// TestChatToResponseStreamBridge_DisabledUsesSyntheticID verifies the disabled
// path still produces a synthetic (non-gateway) ID and commits nothing (O01).
func TestChatToResponseStreamBridge_DisabledUsesSyntheticID(t *testing.T) {
	require.False(t, state.Enabled())
	gin.SetMode(gin.TestMode)
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("Hi"))
	bridge.HandleChunk(c, bridgeFinishChunk("stop"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response)
	require.False(t, state.LooksLikeGatewayResponseID(ev.Response.Id))
}
