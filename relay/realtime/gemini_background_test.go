package realtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiBackgroundIdleWithoutSpokenCompletion reproduces a documented
// Extended Thinking lifecycle: IDLE, not another spoken turnComplete, finishes
// background work. Parameters: t is the test handle. Returns: none.
func TestGeminiBackgroundIdleWithoutSpokenCompletion(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(100, 10), `{"modelTurn":{},"turnComplete":true,"interactionStatus":"IN_PROGRESS"}`)))
	require.Empty(t, g.Ledger.Records)
	require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(100, 30), `{}`)))
	require.NoError(t, g.Observe([]byte(`{"interactionStatus":"IDLE"}`)))
	require.Len(t, g.Ledger.Records, 1, "IDLE seals the final background receipt")
	l := g.Finish(false)
	require.False(t, l.HasUsageGap(), "completed background work is not an estimated disconnect")
	require.EqualValues(t, 30, l.OutputTokens)
}
