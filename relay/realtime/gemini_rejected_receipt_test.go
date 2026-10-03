package realtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiRejectedReceiptSurvivesLaterTurns verifies that a later valid turn
// cannot erase an earlier rejected final receipt. Parameters: t is the test
// handle. Returns: none; measured partial usage and the reconciliation gap survive.
func TestGeminiRejectedReceiptSurvivesLaterTurns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, initial, rejected string
		separateBoundary        bool
	}{
		{"same_frame_boundary", `{"modelTurn":{}}`, `{"turnComplete":true}`, false},
		{"separate_boundary", `{"modelTurn":{}}`, `{}`, true},
		{"background_idle", `{"modelTurn":{},"interactionStatus":"IN_PROGRESS"}`, `{"interactionStatus":"IDLE"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGeminiLedger()
			require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 2), tc.initial)))
			rejectedErr := g.Observe(geminiServerFixture(`{"totalTokenCount":-1}`, tc.rejected))
			if tc.separateBoundary {
				// The rejected frame has already returned its error. Closing that
				// turn must make the gap durable, even if a caller continues.
				_ = g.Observe(geminiServerFixture("", `{"turnComplete":true}`))
			}
			require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(20, 4), `{"modelTurn":{},"turnComplete":true}`)))
			ledger := g.Finish(false)
			t.Logf("records=%d input=%d output=%d gap=%t rejected_error=%v", len(ledger.Records), ledger.InputTokens, ledger.OutputTokens, ledger.HasUsageGap(), rejectedErr)
			require.Len(t, ledger.Records, 2, "keep measured work from both turns")
			require.EqualValues(t, 30, ledger.InputTokens)
			require.EqualValues(t, 6, ledger.OutputTokens)
			require.True(t, ledger.HasUsageGap(), "a different turn cannot repair a rejected final receipt")
			require.Error(t, rejectedErr, "a terminal boundary must not mask the decode error")
		})
	}
}

// TestGeminiRejectedReceiptCorrectedWithinTurnIsComplete is the false-positive
// control: a valid replacement in the same unfinished turn repairs its snapshot.
// Parameters: t is the test handle. Returns: none.
func TestGeminiRejectedReceiptCorrectedWithinTurnIsComplete(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 2), `{"modelTurn":{}}`)))
	require.Error(t, g.Observe(geminiServerFixture(`{"totalTokenCount":-1}`, `{}`)))
	require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(20, 4), `{"turnComplete":true}`)))
	ledger := g.Finish(false)
	require.False(t, ledger.HasUsageGap())
	require.Len(t, ledger.Records, 1)
	require.EqualValues(t, 20, ledger.InputTokens)
	require.EqualValues(t, 4, ledger.OutputTokens)
}
