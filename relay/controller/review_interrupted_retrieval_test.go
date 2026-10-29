package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/state"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type reviewRejectCompletedWriter struct{ gin.ResponseWriter }

func (w reviewRejectCompletedWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "event: response.completed") {
		return 0, errors.New("disconnected before completion")
	}
	return w.ResponseWriter.Write(p)
}

// TestInterruptedCompletionRetrievalDoesNotCharge checks the existing pre-delivery commit and read-only retrieval behavior.
func TestInterruptedCompletionRetrievalDoesNotCharge(t *testing.T) {
	xaiVideoSetup(t, 100000, false)
	store := enableStateForTest(t)
	c, rec := newBridgeTestContext(t)
	c.Writer = reviewRejectCompletedWriter{c.Writer}
	m := testMeta()
	m.ActualModelName = "gpt-4o-test"
	req := &openai.ResponseAPIRequest{Model: "gpt-4o", Input: openai.ResponseAPIInput{"stream please"}}
	capturePendingStateCommit(c, m, req)
	bridge := newChatToResponseStreamBridge(c, m, req).(*chatToResponseStreamBridge)
	bridge.HandleChunk(c, bridgeTextChunk("hello"))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_review", 0, "lookup", "{}"))
	bridge.HandleChunk(c, bridgeFinishChunk("stop"))
	bridge.HandleDone(c)
	bridge.HandleDone(c) // Repeated terminal handling must not commit or charge again.
	require.Error(t, bridge.StreamWriteError())
	require.Empty(t, bridgeFindEvents(parseBridgeSSE(rec.Body.String()), "response.completed"))
	created := bridgeFindEvents(parseBridgeSSE(rec.Body.String()), "response.created")
	require.Len(t, created, 1)
	var event openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, created[0], &event)
	require.NotNil(t, event.Response)
	id := event.Response.Id
	require.True(t, state.LooksLikeGatewayResponseID(id))
	before := reloadUserQuota(t)
	for i := 0; i < 2; i++ {
		record, err := store.GetResponse(context.Background(), testOwner(), id)
		require.NoError(t, err)
		require.NotEmpty(t, record.OutputItems)
		tools := 0
		for _, item := range record.OutputItems {
			var body struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal(item.Raw, &body))
			if body.Type == "function_call" {
				tools++
			}
		}
		require.Equal(t, 1, tools)
	}
	require.Equal(t, before, reloadUserQuota(t))
}
