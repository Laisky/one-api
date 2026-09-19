package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestExpectedLiveTokensDoesNotDoubleCountUnallocatedThinking verifies that the
// settlement oracle does not count thoughts twice when response details are missing.
// Parameters: t owns the test. Returns: none.
func TestExpectedLiveTokensDoesNotDoubleCountUnallocatedThinking(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"promptTokenCount":100,"responseTokenCount":100,"totalTokenCount":200,"thoughtsTokenCount":50}`,
		`{"promptTokenCount":100,"responseTokenCount":100,"totalTokenCount":200,"thoughtsTokenCount":50,"responseTokensDetails":[{"modality":"TEXT","tokenCount":20},{"modality":"AUDIO","tokenCount":30}]}`,
		`{"promptTokenCount":100,"responseTokenCount":100,"totalTokenCount":200,"thoughtsTokenCount":50,"responseTokensDetails":[{"modality":"FUTURE_MODALITY","tokenCount":100}]}`,
	} {
		var usage map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &usage))
		prompt, completion, err := expectedLiveTokens([]map[string]any{usage})
		require.NoError(t, err)
		require.EqualValues(t, 100, prompt)
		require.EqualValues(t, 100, completion)
	}
}

// TestAssertLiveReceiptRejectsMalformedCounters verifies that the diagnostic
// cannot declare a receipt valid when production accounting must reject it.
// Parameters: t owns the test. Returns: none.
func TestAssertLiveReceiptRejectsMalformedCounters(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"promptTokenCount":100,"responseTokenCount":-1,"totalTokenCount":99}`,
		`{"promptTokenCount":100,"responseTokenCount":1.5,"totalTokenCount":101.5}`,
		`{"promptTokenCount":100,"responseTokenCount":1,"totalTokenCount":101,"thoughtsTokenCount":-1}`,
		`{"promptTokenCount":100,"responseTokenCount":1,"totalTokenCount":101,"responseTokensDetails":[{"modality":"AUDIO","tokenCount":2}]}`,
	} {
		var usage map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &usage))
		require.Error(t, assertLiveReceipt(usage), raw)
	}
}

// TestReadLiveTurnAcceptsIdleWithoutSpokenTurnComplete verifies the same
// IN_PROGRESS to IDLE terminal boundary recognized by the production collector.
// Parameters: t owns the test. Returns: none.
func TestReadLiveTurnAcceptsIdleWithoutSpokenTurnComplete(t *testing.T) {
	t.Parallel()
	for _, frames := range [][]string{
		{`{"interactionStatus":"IN_PROGRESS"}`, `{"usageMetadata":{"promptTokenCount":1,"responseTokenCount":2,"totalTokenCount":3}}`, `{"interactionStatus":"IDLE"}`},
		{`{"serverContent":{"interactionStatus":"IN_PROGRESS"}}`, `{"serverContent":{"interactionStatus":"IDLE"}}`, `{"usageMetadata":{"promptTokenCount":1,"responseTokenCount":2,"totalTokenCount":3}}`},
	} {
		conn := liveTestConnection(t, func(conn *websocket.Conn) {
			for _, frame := range frames {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
					t.Errorf("write frame: %v", err)
					return
				}
			}
		})
		turn, err := readLiveTurn(conn, time.Second)
		require.NoError(t, err)
		require.True(t, turn.interactionIdle)
		require.False(t, turn.turnComplete)
		require.NotNil(t, turn.usage)
	}
}
