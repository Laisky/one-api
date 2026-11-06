package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// TestChatToResponseStreamBridge_ReasoningDeltasKeepWhitespace verifies that
// streamed reasoning deltas are forwarded and accumulated verbatim, so word
// boundaries that fall on chunk edges survive, while a whitespace-only first
// delta still does not open an empty reasoning item.
func TestChatToResponseStreamBridge_ReasoningDeltasKeepWhitespace(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	for _, delta := range []string{"\n", "Let me", " think", " about\n", "it."} {
		bridge.HandleChunk(c, bridgeReasoningContentChunk(delta))
	}
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	var streamed strings.Builder
	for _, raw := range bridgeFindEvents(events, "response.reasoning_summary_text.delta") {
		var ev openai.ResponseAPIStreamEvent
		bridgeUnmarshal(t, raw, &ev)
		var text string
		require.NoError(t, json.Unmarshal(ev.Delta, &text))
		streamed.WriteString(text)
	}
	require.Equal(t, "Let me think about\nit.", streamed.String(), "streamed deltas must not drop chunk-boundary whitespace")

	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response)
	found := false
	for _, item := range ev.Response.Output {
		if item.Type != "reasoning" {
			continue
		}
		found = true
		require.Len(t, item.Summary, 1)
		require.Equal(t, "Let me think about\nit.", item.Summary[0].Text)
	}
	require.True(t, found, "a reasoning item must be emitted")
}
