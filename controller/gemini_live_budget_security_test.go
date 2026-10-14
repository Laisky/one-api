package controller

import (
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// liveQuotaCloseReason is the documented close reason for unfunded Live work.
const liveQuotaCloseReason = "gemini_live_quota_exhausted"

// liveSinkProvider counts every client frame reaching the provider and never
// answers. Parameters: frames and bytes receive the counts. Returns: a script.
func liveSinkProvider(frames, bytes *atomic.Int64) liveBudgetServe {
	return func(conn *websocket.Conn) error {
		for {
			if _, data, err := conn.ReadMessage(); err != nil {
				return nil // Gateway teardown ends the provider session.
			} else {
				frames.Add(1)
				bytes.Add(int64(len(data)))
			}
		}
	}
}

// sendLiveBurst writes count frames without waiting for the provider.
// Parameters: conn is the client socket, frame the payload and count the
// burst length. Returns: none; a closed gateway may reject trailing writes.
func sendLiveBurst(conn *websocket.Conn, frame string, count int) {
	for range count {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			return
		}
	}
}

// requireLiveBudgetWarning asserts the payload-free exhaustion diagnostic.
// Parameters: t owns assertions and env holds observed logs. Returns: none.
func requireLiveBudgetWarning(t *testing.T, env *liveBudgetEnv) {
	t.Helper()
	entries := env.logs.FilterMessage("Gemini Live session budget exhausted").All()
	require.NotEmpty(t, entries, "budget exhaustion must be diagnosable")
	for _, entry := range entries {
		require.Equal(t, zapcore.WarnLevel, entry.Level, "insufficient caller quota is a 4xx-class condition")
		for _, field := range entry.Context {
			require.NotContains(t, field.String, "live-budget-provider-key")
			require.NotContains(t, field.String, "aaaa", "logs must not contain client payloads")
		}
	}
}

// TestSecurityGeminiLiveLowBalanceInputBurst reproduces #462: a low-balance
// session forwards billable input far beyond its prepaid reservation. Each
// frame is valid and below every per-message limit; the burst is independent
// of session duration. Parameters: t owns the test. Returns: none.
func TestSecurityGeminiLiveLowBalanceInputBurst(t *testing.T) {
	for _, backend := range []struct {
		name     string
		vertex   bool
		balance  int64
		textRate float64
	}{
		{"developer", false, 40_000, liveBudgetTextQuotaPerToken},
		{"vertex", true, 100_000, 1},
	} {
		t.Run(backend.name, func(t *testing.T) {
			var frames, bytes atomic.Int64
			env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: backend.balance, TokenQuota: backend.balance,
				Group: 1, Vertex: backend.vertex}, liveSinkProvider(&frames, &bytes))
			conn := env.connect()
			const sent = 60
			sendLiveBurst(conn, liveTextFrame(8192), sent)
			reason := awaitClose(conn, 3*time.Second)
			env.finish()

			userQuota, tokenQuota := env.balances()
			charged := backend.balance - userQuota
			// Independent lower bound: four UTF-8 bytes per token at the
			// published text input price. Real tokenizers never do better.
			minimumProviderCost := float64(bytes.Load()) / 4 * backend.textRate
			t.Logf("provider_frames=%d provider_bytes=%d charged=%d user_quota=%d close=%q",
				frames.Load(), bytes.Load(), charged, userQuota, reason)
			require.LessOrEqual(t, minimumProviderCost, float64(charged),
				"provider received input worth more than the user's entire debit")
			require.Less(t, frames.Load(), int64(sent), "unfunded input must not reach the provider")
			require.Positive(t, frames.Load(), "funded input must still be forwarded")
			require.GreaterOrEqual(t, userQuota, int64(0), "a prepaid session must not create debt from estimates")
			require.GreaterOrEqual(t, tokenQuota, int64(0))
			require.Equal(t, liveQuotaCloseReason, reason)
			logs := env.consumeLogs()
			require.Len(t, logs, 1)
			for _, rows := range logs {
				require.Len(t, rows, 1, "a session settles exactly once")
				require.EqualValues(t, charged, rows[0].Quota)
				require.Equal(t, true, rows[0].Metadata[model.LogMetadataKeyEstimatedCharge], "missing receipts stay an explicit estimate")
				require.Equal(t, true, rows[0].Metadata["realtime_budget_exhausted"])
				reserved, ok := rows[0].Metadata["realtime_budget_reserved"].(float64)
				require.True(t, ok)
				require.Greater(t, reserved, float64(charged), "headroom for turns that never started is refunded")
			}
			requireLiveBudgetWarning(t, env)
		})
	}
}

// liveTurnProvider answers each client text frame with streamed audio and one
// realistic receipt whose prompt re-bills the growing context. Parameters:
// turns counts completed provider turns and quota accumulates exact receipt
// quota. Returns: a script that stops when the gateway closes the session.
func liveTurnProvider(turns *atomic.Int64, quota *liveQuotaSum) liveBudgetServe {
	return func(conn *websocket.Conn) error {
		const textPerTurn, audioPerTurn, remainderPerTurn = 100, 100, 7
		for k := int64(1); ; k++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				return nil
			}
			for range 4 {
				if err := conn.WriteMessage(websocket.TextMessage, liveAudioOutputFrame(1)); err != nil {
					return nil
				}
			}
			textIn, audioIn, remainder := 50+textPerTurn*k, audioPerTurn*(k-1), remainderPerTurn*k
			if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(textIn, audioIn, remainder, 10, audioPerTurn, 30)); err != nil {
				return nil
			}
			quota.add(liveReceiptQuota(textIn, audioIn, remainder, 10, audioPerTurn, 30))
			turns.Add(1)
		}
	}
}

// liveQuotaSum accumulates exact receipt quota across provider goroutines.
type liveQuotaSum struct {
	mu    sync.Mutex
	value float64
}

// add records one receipt. Parameters: v is its unrounded quota. Returns: none.
func (s *liveQuotaSum) add(v float64) { s.mu.Lock(); s.value += v; s.mu.Unlock() }

// total returns the session charge rounded once. Parameters: none. Returns: quota.
func (s *liveQuotaSum) total() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(math.Ceil(s.value - 1e-9))
}

// TestSecurityGeminiLiveReceiptsCannotOutrunReservation reproduces billed
// turns beyond the reservation: every turn re-bills the whole context, so
// sequential cheap inputs accumulate debt. Parameters: t owns the test.
func TestSecurityGeminiLiveReceiptsCannotOutrunReservation(t *testing.T) {
	const balance = 60_000
	var turns atomic.Int64
	receipts := &liveQuotaSum{}
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: balance, Group: 1},
		liveTurnProvider(&turns, receipts))
	conn := env.connect()
	completed, reason := 0, ""
	for range 40 {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(liveTextFrame(400))); err != nil {
			break
		}
		turnDone := false
		for !turnDone {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) {
					reason = closeErr.Text
				}
				break
			}
			turnDone = strings.Contains(string(frame), "usageMetadata")
		}
		if !turnDone {
			break
		}
		completed++
	}
	if reason == "" {
		reason = awaitClose(conn, time.Second)
	}
	env.finish()

	userQuota, tokenQuota := env.balances()
	t.Logf("completed_turns=%d provider_turns=%d receipts_quota=%d user_quota=%d close=%q",
		completed, turns.Load(), receipts.total(), userQuota, reason)
	require.Less(t, completed, 40, "the session must stop before unfunded turns")
	require.Positive(t, completed)
	require.EqualValues(t, completed, turns.Load(), "an unfunded turn trigger must not reach the provider")
	require.GreaterOrEqual(t, userQuota, int64(0), "authoritative receipts must not exceed the prepaid budget")
	require.GreaterOrEqual(t, tokenQuota, int64(0))
	require.Equal(t, receipts.total(), balance-userQuota, "complete receipts settle exactly")
	require.Equal(t, liveQuotaCloseReason, reason)
	for _, rows := range env.consumeLogs() {
		require.Len(t, rows, 1)
		require.EqualValues(t, receipts.total(), rows[0].Quota)
		require.Equal(t, true, rows[0].Metadata["realtime_billing_complete"])
	}
	requireLiveBudgetWarning(t, env)
}

// TestSecurityGeminiLiveConcurrentSessionsCannotOversubscribe reproduces two
// sessions of one finite token each spending the same prepaid balance.
// Parameters: t owns the test. Returns: none.
func TestSecurityGeminiLiveConcurrentSessionsCannotOversubscribe(t *testing.T) {
	const tokenBalance, userBalance = 60_000, 1_000_000
	var frames, bytes atomic.Int64
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: userBalance, TokenQuota: tokenBalance, Group: 1},
		liveSinkProvider(&frames, &bytes))
	first, second := env.connect(), env.connect()
	var wg sync.WaitGroup
	reasons := make([]string, 2)
	for i, conn := range []*websocket.Conn{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendLiveBurst(conn, liveTextFrame(8192), 60)
			reasons[i] = awaitClose(conn, 3*time.Second)
		}()
	}
	wg.Wait()
	env.finish()

	userQuota, tokenQuota := env.balances()
	charged := tokenBalance - tokenQuota
	t.Logf("provider_frames=%d provider_bytes=%d charged=%d token_quota=%d close=%q",
		frames.Load(), bytes.Load(), charged, tokenQuota, reasons)
	require.Equal(t, int64(userBalance)-userQuota, charged, "user and token ledgers move together")
	require.GreaterOrEqual(t, tokenQuota, int64(0), "concurrent sessions must not oversubscribe a finite token")
	require.LessOrEqual(t, float64(bytes.Load())/4*liveBudgetTextQuotaPerToken, float64(charged),
		"provider received input worth more than both sessions' combined debit")
	require.Less(t, frames.Load(), int64(120))
	require.Equal(t, []string{liveQuotaCloseReason, liveQuotaCloseReason}, reasons)
	var settled int64
	for _, rows := range env.consumeLogs() {
		require.Len(t, rows, 1)
		settled += int64(rows[0].Quota)
	}
	require.Equal(t, charged, settled, "two sessions settle exactly once each")
}

// TestSecurityGeminiLiveOutputStreamCannotOutrunReservation verifies that the
// gateway stops a funded response before another acknowledged frame can outrun
// its reservation. Parameters: t owns the test. Returns: none.
func TestSecurityGeminiLiveOutputStreamCannotOutrunReservation(t *testing.T) {
	runLiveOutputReservation(t, true)
}

// TestSecurityGeminiLiveBufferedOutputPreservesReservation exercises an unpaced
// provider burst without confusing buffered socket writes with gateway delivery.
// Parameters: t owns the test. Returns: none.
func TestSecurityGeminiLiveBufferedOutputPreservesReservation(t *testing.T) {
	runLiveOutputReservation(t, false)
}

// runLiveOutputReservation checks output cutoff, upstream closure and exact
// durable settlement using the real relay. Parameters: acknowledged bounds the
// synthetic producer to one unacknowledged frame; false retains an unpaced burst.
// Returns: none. The acknowledgement is fixture-only and never changes a socket
// payload, production flow control, quota policy or timeout.
func runLiveOutputReservation(t *testing.T, acknowledged bool) {
	t.Helper()
	const balance, chunks = 30_000, 100
	var written atomic.Int64
	delivered := make(chan struct{}, 1)
	providerClosed := make(chan struct{})
	providerReadResult := make(chan error, 1)
	providerDone := make(chan struct{})
	env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: balance, Group: 1},
		func(conn *websocket.Conn) error {
			defer close(providerDone)
			if _, _, err := conn.ReadMessage(); err != nil {
				return errors.Wrap(err, "read funded input")
			}
			// A websocket writer must also read to observe peer close controls.
			// Successful writes can otherwise be queued after the gateway stops.
			go func() {
				defer close(providerClosed)
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						providerReadResult <- err
						return
					}
				}
			}()
			deadline := time.NewTimer(30 * time.Second)
			defer deadline.Stop()
			frame := liveAudioOutputFrame(2)
			for range chunks {
				if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					return nil // The real gateway closed the provider connection.
				}
				written.Add(1)
				if acknowledged {
					select {
					case <-delivered:
					case <-providerClosed:
						return nil
					case <-deadline.C:
						return errors.New("fixture received neither delivery acknowledgement nor upstream closure")
					}
				}
			}
			if err := conn.WriteMessage(websocket.TextMessage, liveReceiptFrame(60, 0, 0, 0, 25*2*chunks, 0)); err != nil {
				return nil // Closing the exhausted session can reject its final receipt.
			}
			select {
			case <-providerClosed:
				return nil
			case <-deadline.C:
				return errors.New("fixture upstream did not close")
			}
		})
	conn := env.connect()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(liveTextFrame(200))))
	received, reason := 0, ""
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(30*time.Second)))
	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				reason = closeErr.Text
			}
			break
		}
		if strings.Contains(string(frame), "usageMetadata") {
			reason = awaitClose(conn, time.Second)
			break
		}
		received++
		if acknowledged {
			delivered <- struct{}{}
		}
	}
	if received >= chunks {
		t.Logf("acknowledged=%t client_chunks=%d provider_chunks=%d close=%q", acknowledged, received, written.Load(), reason)
		require.Less(t, received, chunks, "unfunded streamed output reached the client")
	}
	// Inspect the actual read result before harness cleanup can close the client.
	select {
	case err := <-providerReadResult:
		require.True(t, liveOutputReadEndedWithoutTimeout(err), "a provider read deadline is not evidence of peer closure")
	case <-time.After(30 * time.Second):
		t.Fatal("provider did not observe closure before fixture cleanup")
	}
	env.finish()
	for _, done := range []<-chan struct{}{providerClosed, providerDone} {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("provider did not observe gateway closure and terminate")
		}
	}

	userQuota, tokenQuota := env.balances()
	t.Logf("acknowledged=%t client_chunks=%d provider_chunks=%d user_quota=%d close=%q", acknowledged, received, written.Load(), userQuota, reason)
	require.Positive(t, received, "a dropped session is not a successful funding control")
	require.Less(t, received, chunks, "unfunded streamed output must stop the provider session")
	if acknowledged {
		// The original stop-before-producer-completion assertion now observes
		// delivery progress, independent of the kernel's socket-buffer capacity.
		require.Less(t, written.Load(), int64(chunks))
		require.EqualValues(t, received+1, written.Load(), "only the next withheld frame may be unacknowledged")
	} else {
		require.GreaterOrEqual(t, written.Load(), int64(received))
	}
	require.GreaterOrEqual(t, userQuota, int64(0))
	require.GreaterOrEqual(t, tokenQuota, int64(0))
	require.Equal(t, userQuota, tokenQuota, "user and token reservations must settle together")
	require.Equal(t, liveQuotaCloseReason, reason)
	// The fixture's independently specified audio price is 6 quota per token,
	// with 25 tokens per second. Delivered output alone must fit its debit.
	require.LessOrEqual(t, int64(received*2*25*6), balance-userQuota)
	rowsByRequest := env.consumeLogs()
	require.Len(t, rowsByRequest, 1, "the session must leave one durable settlement")
	for _, rows := range rowsByRequest {
		require.Len(t, rows, 1)
		require.EqualValues(t, balance-userQuota, rows[0].Quota)
		require.Equal(t, true, rows[0].Metadata[model.LogMetadataKeyEstimatedCharge])
		require.Equal(t, true, rows[0].Metadata["realtime_budget_exhausted"])
	}
	requireLiveBudgetWarning(t, env)
}

// liveOutputReadEndedWithoutTimeout distinguishes a non-timeout read termination
// from a missing result or expired fixture deadline, including wrapped errors.
// Parameters: err is the provider reader's result. Returns: non-timeout termination.
func liveOutputReadEndedWithoutTimeout(err error) bool {
	if err == nil {
		return false
	}
	var timeout interface{ Timeout() bool }
	return !errors.As(err, &timeout) || !timeout.Timeout()
}

// TestSecurityGeminiLiveProviderReadTimeoutIsNotClosure preserves deadline
// rejection while accepting WebSocket closes and connection teardown.
// Parameters: t owns synthetic error controls. Returns: none.
func TestSecurityGeminiLiveProviderReadTimeoutIsNotClosure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		closed bool
	}{
		{name: "missing"},
		{name: "deadline", err: os.ErrDeadlineExceeded},
		{name: "wrapped_deadline", err: errors.Wrap(os.ErrDeadlineExceeded, "fixture read")},
		{name: "websocket_close", err: &websocket.CloseError{Code: websocket.CloseNormalClosure}, closed: true},
		{name: "connection_eof", err: io.EOF, closed: true},
		{name: "wrapped_connection_eof", err: errors.Wrap(io.EOF, "fixture read"), closed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.closed, liveOutputReadEndedWithoutTimeout(tc.err))
		})
	}
}
