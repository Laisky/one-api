package controller

import (
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestGeminiLiveBudgetPreservesFreeGroups verifies that a genuine free group
// (zero group ratio) is never reserved, never refused and settles at zero even
// with a zero balance. Parameters: t owns the test. Returns: none.
func TestGeminiLiveBudgetPreservesFreeGroups(t *testing.T) {
	var frames, bytes atomic.Int64
	env := newLiveBudgetEnv(t, liveBudgetOptions{Group: 0}, liveSinkProvider(&frames, &bytes))
	conn := env.connect()
	sendLiveBurst(conn, liveTextFrame(8192), 30)
	time.Sleep(300 * time.Millisecond)
	reason := awaitClose(conn, 200*time.Millisecond)
	env.finish()
	userQuota, tokenQuota := env.balances()
	require.Empty(t, reason, "free sessions are never closed for quota")
	require.EqualValues(t, 30, frames.Load())
	require.Zero(t, userQuota)
	require.Zero(t, tokenQuota)
	for _, rows := range env.consumeLogs() {
		require.Len(t, rows, 1)
		require.Zero(t, rows[0].Quota)
	}
}

// TestGeminiLiveBudgetUnlimitedTokenUsesUserBalance verifies an unlimited
// token is still bounded by its owner's balance and never debited itself.
// Parameters: t owns the test. Returns: none.
func TestGeminiLiveBudgetUnlimitedTokenUsesUserBalance(t *testing.T) {
	const balance = 40_000
	var frames, bytes atomic.Int64
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: 7, Unlimited: true, Group: 1},
		liveSinkProvider(&frames, &bytes))
	conn := env.connect()
	sendLiveBurst(conn, liveTextFrame(8192), 60)
	reason := awaitClose(conn, 3*time.Second)
	env.finish()
	userQuota, tokenQuota := env.balances()
	require.Equal(t, liveQuotaCloseReason, reason)
	require.Less(t, frames.Load(), int64(60))
	require.GreaterOrEqual(t, userQuota, int64(0))
	require.LessOrEqual(t, float64(bytes.Load())/4*liveBudgetTextQuotaPerToken, float64(balance-userQuota))
	require.EqualValues(t, 7, tokenQuota, "an unlimited token balance is never moved")
}

// TestGeminiLiveBudgetOperatorConfiguredModel verifies an administrator-priced
// Developer API model is admitted and bounded by its own channel prices.
// Parameters: t owns the test. Returns: none.
func TestGeminiLiveBudgetOperatorConfiguredModel(t *testing.T) {
	const balance = 100_000
	var frames, bytes atomic.Int64
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: balance, Group: 1,
		Model: "operator-configured-live-id"}, liveSinkProvider(&frames, &bytes))
	conn := env.connect()
	sendLiveBurst(conn, liveTextFrame(8192), 60)
	reason := awaitClose(conn, 3*time.Second)
	env.finish()
	userQuota, _ := env.balances()
	require.Equal(t, liveQuotaCloseReason, reason)
	require.Positive(t, frames.Load())
	require.Less(t, frames.Load(), int64(60))
	// Channel text price is 1 quota/token (vertexLivePrices).
	require.LessOrEqual(t, float64(bytes.Load())/4, float64(balance-userQuota))
	require.GreaterOrEqual(t, userQuota, int64(0))
}

// TestGeminiLiveBudgetDelayedDuplicateReceiptSettlesOnce verifies a receipt
// that arrives after the client disconnected, preceded by an identical usage
// snapshot, settles exactly once at the measured price and refunds every
// unused reservation. Parameters: t owns the test. Returns: none.
func TestGeminiLiveBudgetDelayedDuplicateReceiptSettlesOnce(t *testing.T) {
	const balance = 100_000
	receipt := liveReceiptFrame(120, 0, 3, 10, 100, 30)
	usageOnly := []byte(`{"usageMetadata":` + strings.SplitN(string(receipt), `"usageMetadata":`, 2)[1])
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: balance, Group: 1},
		func(conn *websocket.Conn) error {
			if _, _, err := conn.ReadMessage(); err != nil {
				return errors.Wrap(err, "read input")
			}
			if err := conn.WriteMessage(websocket.TextMessage, liveAudioOutputFrame(1)); err != nil {
				return errors.Wrap(err, "write audio")
			}
			time.Sleep(300 * time.Millisecond) // The client is gone; the gateway drains.
			for _, frame := range [][]byte{usageOnly, receipt} {
				if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					return errors.Wrap(err, "write receipt")
				}
			}
			_, _, _ = conn.ReadMessage()
			return nil
		})
	conn := env.connect()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(liveTextFrame(600))))
	_, _, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	env.finish()
	want := int64(math.Ceil(liveReceiptQuota(120, 0, 3, 10, 100, 30)))
	userQuota, tokenQuota := env.balances()
	require.Equal(t, balance-want, userQuota, "the measured receipt is charged once and the rest refunded")
	require.Equal(t, balance-want, tokenQuota)
	logs := env.consumeLogs()
	require.Len(t, logs, 1)
	for _, rows := range logs {
		require.Len(t, rows, 1)
		require.EqualValues(t, want, rows[0].Quota)
		require.Equal(t, true, rows[0].Metadata["realtime_billing_complete"])
		require.Nil(t, rows[0].Metadata[model.LogMetadataKeyEstimatedCharge])
		require.EqualValues(t, 22500, rows[0].Metadata["realtime_budget_reserved"])
	}
}
