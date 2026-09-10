package openai

import (
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewCapacityClosesRealWebSocket verifies that a full ledger cannot turn
// into an unmetered relay. The limit receipt is delivered and kept for settlement,
// and both real loopback pumps terminate without evicting replay protection.
func TestReviewCapacityClosesRealWebSocket(t *testing.T) {
	client, proxyClient := realtimeMeterPair(t)
	proxyUpstream, upstream := realtimeMeterPair(t)
	for _, conn := range []*websocket.Conn{client, proxyClient, proxyUpstream, upstream} {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(45*time.Second)))
		require.NoError(t, conn.SetWriteDeadline(time.Now().Add(45*time.Second)))
	}
	result := make(chan *rmodel.Usage, 1)
	go func() { result <- meteredRealtimePump(proxyClient, proxyUpstream, nil) }()
	for i := 0; i < realtime.MaxRecords; i++ {
		frame := fmt.Sprintf(`{"type":"response.done","response":{"id":"r%d","usage":{"input_tokens":1,"output_tokens":0}}}`, i)
		require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(frame)))
		kind, body, err := client.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, kind)
		require.Equal(t, frame, string(body))
	}
	_, _, err := client.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.ClosePolicyViolation), "expected a deliberate capacity close, got %v", err)
	select {
	case usage := <-result:
		require.NotNil(t, usage.Realtime)
		require.Len(t, usage.Realtime.Records, realtime.MaxRecords)
		require.Equal(t, realtime.MaxRecords, usage.PromptTokens)
		require.True(t, usage.Realtime.Audit().CapacityReached)
		require.False(t, usage.Realtime.HasUsageGap())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "metered pumps did not stop at ledger capacity")
	}
	_, _, err = upstream.ReadMessage()
	require.Error(t, err, "the upstream must not remain connected after the meter stops")
}
