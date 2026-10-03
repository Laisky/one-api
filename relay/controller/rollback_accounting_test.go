package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// These tests exercise the audio/video pre-consume ROLLBACK path (goRollbackPreConsumed,
// reached via goAudioRollbackPreConsumed / goVideoRollbackPreConsumed) against a real
// (sqlite) database and assert on the user's actual quota balance. The rollback runs from a
// defer AFTER the handler returns, on a detached, c-free, BillingTimeoutSec-bounded context;
// these tests prove the money actually moves correctly there:
//
//   - the rollback ALWAYS refunds (unlike the conservative refund it is not gated on the
//     forwarded marker — audio/video roll back the whole pre-consume on failure);
//   - the refund lands even after the request context is cancelled (detached context);
//   - many concurrent rollbacks net to an exact, lossless, non-double-counted balance.

// newRollbackContext builds a gin context wired to the fixture user/token, optionally with a
// cancellable request context (to simulate a client disconnect before the rollback runs).
func newRollbackContext(t *testing.T, requestID string, cancellable bool) (*gin.Context, context.CancelFunc) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/audio/transcriptions", nil)
	var cancel context.CancelFunc
	if cancellable {
		var reqCtx context.Context
		reqCtx, cancel = context.WithCancel(context.Background())
		req = req.WithContext(reqCtx)
	}
	c.Request = req
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.RequestId, requestID)
	return c, cancel
}

// TestAudioRollback_DBAccounting_RefundsExactlyOnce refunds exactly the pre-consumed quota.
func TestAudioRollback_DBAccounting_RefundsExactlyOnce(t *testing.T) {
	const K, Q = int64(10_000_000), int64(2468)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	require.Equal(t, start-Q, reloadUserQuota(t), "pre-consume should deduct Q")

	c, _ := newRollbackContext(t, "audio-rollback-"+t.Name(), false)
	goAudioRollbackPreConsumed(c, fallbackTokenID, Q)
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"audio rollback must refund the pre-consumed quota exactly once")
}

// TestVideoRollback_DBAccounting_RefundsExactlyOnce mirrors the audio case for the video path.
func TestVideoRollback_DBAccounting_RefundsExactlyOnce(t *testing.T) {
	const K, Q = int64(10_000_000), int64(1357)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	require.Equal(t, start-Q, reloadUserQuota(t), "pre-consume should deduct Q")

	c, _ := newRollbackContext(t, "video-rollback-"+t.Name(), false)
	goVideoRollbackPreConsumed(c, fallbackTokenID, Q)
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"video rollback must refund the pre-consumed quota exactly once")
}

// TestAudioRollback_DBAccounting_SurvivesClientDisconnect asserts the rollback refund lands
// even when the request context is cancelled before the rollback goroutine runs (it runs on
// a detached, non-cancelled context). See the NOTE on sqlite in billing_accounting_test.go:
// the discriminating ctx-observation reproduce-first lives in audio_rollback_ctx_test.go.
func TestAudioRollback_DBAccounting_SurvivesClientDisconnect(t *testing.T) {
	const K, Q = int64(10_000_000), int64(909)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	c, cancel := newRollbackContext(t, "audio-rollback-disconnect-"+t.Name(), true)
	goAudioRollbackPreConsumed(c, fallbackTokenID, Q)
	cancel() // client disconnects / handler returns: request context is cancelled
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"rollback refund must land on a detached context even after request-context cancellation")
}

// TestRollback_DBAccounting_ConcurrentMix fires many concurrent audio+video rollbacks and
// asserts the final balance returns to start: every rollback refunds exactly once, with no
// lost or double refunds under gin pool recycling + concurrent DB writes.
func TestRollback_DBAccounting_ConcurrentMix(t *testing.T) {
	const (
		K     = int64(50_000_000)
		Q     = int64(40)
		total = 200
	)
	start := billingAccountingSetup(t, K)

	// In-memory sqlite cannot service many concurrent writers; serialize the pool (the
	// goroutine spawning + per-request snapshotting concurrency is unaffected — production
	// runs MySQL/Postgres). See billing_accounting_test.go for the rationale.
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(config.SQLMaxOpenConns) })

	for i := 0; i < total; i++ {
		preConsume(t, Q)
	}
	require.Equal(t, start-int64(total)*Q, reloadUserQuota(t))

	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/audio/transcriptions", nil)
			c.Set(ctxkey.Id, fallbackUserID)
			c.Set(ctxkey.TokenId, fallbackTokenID)
			c.Set(ctxkey.RequestId, fmt.Sprintf("rollback-mix-%d", i))
			if i%2 == 0 {
				goAudioRollbackPreConsumed(c, fallbackTokenID, Q)
			} else {
				goVideoRollbackPreConsumed(c, fallbackTokenID, Q)
			}
		}(i)
	}
	wg.Wait()
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"every concurrent rollback must refund exactly once: final balance must return to start")
}
