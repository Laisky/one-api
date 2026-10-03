package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
)

// These tests exercise the real refund path against a real (sqlite) database and
// assert on the user's actual quota balance. They validate BILLING DATA
// CORRECTNESS — the property the async/sync refund fix must preserve:
//
//   - not-forwarded failure  => pre-consumed quota is refunded EXACTLY once;
//   - forwarded failure       => pre-consumed quota is NOT refunded (conservative
//                                no-underbilling policy);
//   - client disconnect       => the refund still lands, because it runs on a
//                                detached (non-cancelled) context — this is the
//                                refund-loss regression the fix closes;
//   - concurrent mix          => the final balance equals the exact algebraic sum,
//                                with no lost or double refunds.

// billingAccountingSetup prepares the shared fixtures, disables redis / consume
// logging, sets a generous sqlite busy timeout (so concurrent quota UPDATEs wait
// instead of erroring SQLITE_BUSY), resets the user's quota to a known value and
// returns it.
func billingAccountingSetup(t *testing.T, startQuota int64) int64 {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLog := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLog) })

	// in-memory sqlite serializes writes; wait rather than fail under concurrency.
	require.NoError(t, model.DB.Exec("PRAGMA busy_timeout = 10000").Error)

	require.NoError(t, model.DB.Model(&model.User{}).
		Where("id = ?", fallbackUserID).
		Update("quota", startQuota).Error, "failed to reset user quota")
	return reloadUserQuota(t)
}

// preConsume simulates a pre-consume deduction on the request goroutine.
func preConsume(t *testing.T, quota int64) {
	t.Helper()
	require.NoError(t, model.PostConsumeTokenQuota(context.Background(), fallbackTokenID, quota),
		"failed to simulate pre-consume deduction")
}

// newRefundContext builds a gin context wired to the fixture user/token. When
// cancellable is true the request carries a cancellable context whose cancel func
// is returned so the test can simulate a client disconnect.
func newRefundContext(t *testing.T, forwarded bool, cancellable bool) (*gin.Context, context.CancelFunc) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	var cancel context.CancelFunc
	if cancellable {
		var reqCtx context.Context
		reqCtx, cancel = context.WithCancel(context.Background())
		req = req.WithContext(reqCtx)
	}
	c.Request = req
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.RequestId, "acct-"+t.Name())
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, forwarded)
	return c, cancel
}

func drainBilling(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// TestConservativeRefund_DBAccounting_NotForwarded refunds exactly the pre-consumed
// quota for a non-forwarded failure.
func TestConservativeRefund_DBAccounting_NotForwarded(t *testing.T) {
	const K, Q = int64(10_000_000), int64(1234)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	require.Equal(t, start-Q, reloadUserQuota(t), "pre-consume should deduct Q")

	c, _ := newRefundContext(t, false /*forwarded*/, false)
	markPreConsumed(c, Q)
	scheduleConservativeRefund(c, Q, fallbackTokenID, "upstream_http_error")
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"non-forwarded refund must return the pre-consumed quota exactly once")
}

// TestConservativeRefund_DBAccounting_Forwarded keeps the charge for a forwarded
// failure (conservative no-underbilling).
func TestConservativeRefund_DBAccounting_Forwarded(t *testing.T) {
	const K, Q = int64(10_000_000), int64(4321)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	c, _ := newRefundContext(t, true /*forwarded*/, false)
	markPreConsumed(c, Q)
	scheduleConservativeRefund(c, Q, fallbackTokenID, "upstream_http_error")
	drainBilling(t)

	require.Equal(t, start-Q, reloadUserQuota(t),
		"forwarded request must NOT be refunded (conservative no-underbilling policy)")
}

// TestConservativeRefund_DBAccounting_SurvivesClientDisconnect asserts the refund
// lands even when the request context is cancelled before the refund goroutine runs.
//
// NOTE: this is a behavioral guard, not a reproduce-first. In production (MySQL/
// Postgres) a DB write bound to a cancelled context aborts, so the old gmw.Ctx(c) lost
// the refund; but in-memory sqlite does NOT honour context cancellation for its fast
// local writes, so this test passes against both the buggy and fixed code. The
// DISCRIMINATING reproduce-first for the detached-context property lives in
// billing_safety_race_test.go::TestScheduleConservativeRefund_DetachedSnapshot, which
// asserts the goroutine observes ctx.Err()==nil (and fails with gmw.Ctx(c)).
func TestConservativeRefund_DBAccounting_SurvivesClientDisconnect(t *testing.T) {
	const K, Q = int64(10_000_000), int64(777)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	c, cancel := newRefundContext(t, false /*forwarded*/, true /*cancellable*/)
	markPreConsumed(c, Q)
	scheduleConservativeRefund(c, Q, fallbackTokenID, "do_request_failed")
	cancel() // client disconnects / handler returns: request context is cancelled
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"refund must land on a detached context even after the request context is cancelled")
}

// TestConservativeRefund_DBAccounting_ConcurrentMix fires many concurrent refunds and
// asserts the final balance is the exact algebraic sum: forwarded requests keep their
// charge, non-forwarded requests are fully refunded. No lost or double refunds.
func TestConservativeRefund_DBAccounting_ConcurrentMix(t *testing.T) {
	const (
		K       = int64(50_000_000)
		Q       = int64(50)
		total   = 240
		skip    = 120 // forwarded => not refunded
		refunds = total - skip
	)
	start := billingAccountingSetup(t, K)

	// In-memory sqlite cannot service many concurrent writers (it returns "database
	// table is locked"); that is a test-DB limitation, not a logic concern — production
	// runs MySQL/Postgres. Serialize the connection pool so the concurrent refund
	// goroutines' DB writes queue instead of failing. The concurrency under test (the
	// goroutine spawning + per-request snapshotting) is unaffected; the high-concurrency
	// pool-recycling race is covered separately by the *_ConcurrentPoolRecycling tests.
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(config.SQLMaxOpenConns) })

	// Deduct all pre-consumes serially (the deduction itself is not under test).
	for i := 0; i < total; i++ {
		preConsume(t, Q)
	}
	require.Equal(t, start-int64(total)*Q, reloadUserQuota(t))

	// Fire the refunds; each scheduleConservativeRefund spawns its own goroutine, so
	// the refunds run concurrently against the DB.
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			forwarded := i < skip
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set(ctxkey.Id, fallbackUserID)
			c.Set(ctxkey.TokenId, fallbackTokenID)
			c.Set(ctxkey.RequestId, fmt.Sprintf("acct-mix-%d", i))
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, forwarded)
			markPreConsumed(c, Q)
			scheduleConservativeRefund(c, Q, fallbackTokenID, "upstream_http_error")
		}(i)
	}
	wg.Wait()
	drainBilling(t)

	want := start - int64(skip)*Q // refunds net to zero; only the skipped ones stay charged
	require.Equal(t, want, reloadUserQuota(t),
		"concurrent refunds must net exactly: %d forwarded kept, %d refunded", skip, refunds)
}
