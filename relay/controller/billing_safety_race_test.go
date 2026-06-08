package controller

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
)

// TestScheduleConservativeRefund_MarksReconciledSynchronously reproduces the
// billing-audit false-alarm race logged at billing_safety.go:240
// ("CRITICAL BILLING AUDIT: cannot refund - request was possibly forwarded
// upstream, manual reconciliation required").
//
// On an error path of an already-forwarded request, the conservative refund —
// which is what flips BillingReconciled — used to be dispatched to an async
// graceful.GoCritical goroutine, while the deferred billingAuditSafetyNet runs
// synchronously the instant the handler returns. When the refund goroutine has
// not been scheduled yet, the safety net observes an unreconciled, forwarded
// pre-consume it cannot auto-refund and emits the false-positive CRITICAL alarm.
//
// The test freezes the refund goroutine on a release gate to make the race
// deterministic, then asserts that scheduleConservativeRefund has already marked
// billing reconciled by the time it returns (i.e. before the deferred safety net
// would run). Against the buggy implementation the flag is still false here and
// the assertion fails; with the synchronous-mark fix it passes.
func TestScheduleConservativeRefund_MarksReconciledSynchronously(t *testing.T) {
	gin.SetMode(gin.TestMode)

	release := make(chan struct{})
	refundGoroutineReleaseForTest = release
	// Always release and drain the parked refund goroutine before clearing the
	// gate, even when the assertion below fails via FailNow (which runs deferred
	// funcs). This keeps the goroutine from leaking and avoids racing the gate.
	defer func() {
		close(release)
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		refundGoroutineReleaseForTest = nil
	}()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 0)
	c.Set(ctxkey.RequestId, "req_race_repro")
	// Forwarded upstream: the safety net cannot auto-refund, so an unreconciled
	// pre-consume in this state is exactly the CRITICAL-alarm scenario.
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	markPreConsumed(c, 1207)

	// The error path schedules the refund. The release gate keeps the refund
	// goroutine parked, simulating "goroutine spawned but not yet scheduled".
	scheduleConservativeRefund(c, 1207, 0, "upstream_http_error")

	// The handler is about to return -> the deferred billingAuditSafetyNet runs
	// now, on the request goroutine, while the refund goroutine is still parked.
	// It must observe billing already reconciled.
	require.True(t, c.GetBool(ctxkey.BillingReconciled),
		"scheduleConservativeRefund must mark billing reconciled synchronously; "+
			"otherwise the deferred billingAuditSafetyNet races the async refund "+
			"goroutine and emits a false-positive CRITICAL alarm")

	// Sanity check: the deferred safety net is genuinely a no-op in this state
	// (it must not log the CRITICAL alarm). It reads only the reconciled flag.
	billingAuditSafetyNet(c)
}

// TestScheduleConservativeRefund_DetachedSnapshot reproduces two bugs in the async
// refund path at once:
//
//  1. Refund-loss: the refund goroutine used to run on gmw.Ctx(c), the request
//     context. It is spawned right before the handler returns, so the request
//     context is (being) cancelled by the time the refund DB write runs and the
//     write can be aborted, silently losing the refund. The fix runs the refund on
//     relayctx.Detach(c), a non-cancelled context, so the observed ctx error is nil
//     even after the request context is cancelled.
//
//  2. Stale/recycled read: the refund used to read shouldSkipPreConsumedRefund(c),
//     c.GetInt(ctxkey.Id) etc. INSIDE the goroutine, racing gin's sync.Pool recycle
//     of c. The fix snapshots every value on the request goroutine before spawning,
//     so mutating c after the spawn (simulating recycling/forwarded-flag flip) must
//     not change the values the goroutine acts on.
//
// The test parks the refund goroutine on the release gate, cancels the request
// context and mutates c while it is parked, then releases it and asserts on the
// observed (ctxErr, snapshot). Against the buggy implementation ctxErr is
// context.Canceled and skipRefund/userID reflect the mutated c; with the fix ctxErr
// is nil and the snapshot reflects spawn-time values.
func TestScheduleConservativeRefund_DetachedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(release) }) }
	observed := make(chan struct {
		ctxErr error
		snap   conservativeRefundSnapshot
	}, 1)
	refundGoroutineReleaseForTest = release
	refundObservedForTest = func(ctxErr error, snap conservativeRefundSnapshot) {
		observed <- struct {
			ctxErr error
			snap   conservativeRefundSnapshot
		}{ctxErr, snap}
	}
	t.Cleanup(func() {
		releaseGate()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		refundGoroutineReleaseForTest = nil
		refundObservedForTest = nil
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	c.Request = req.WithContext(reqCtx)
	c.Set(ctxkey.Id, 42)
	c.Set(ctxkey.TokenId, 7)
	c.Set(ctxkey.ProvisionalLogId, 99)
	c.Set(ctxkey.RequestId, "req_detached_snapshot")
	// NOT forwarded at spawn time: the snapshot must capture skipRefund=false.
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
	markPreConsumed(c, 1207)

	scheduleConservativeRefund(c, 1207, 7, "upstream_http_error")

	// Simulate the handler returning and gin recycling c for the next request: the
	// request context is cancelled and the forwarded flag / user id are mutated while
	// the refund goroutine is still parked on the gate.
	cancelReq()
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	c.Set(ctxkey.Id, 999)

	// Release the parked goroutine so it records what it actually runs with.
	releaseGate()

	select {
	case got := <-observed:
		require.NoError(t, got.ctxErr,
			"async conservative refund must run on a non-cancelled (detached) context; "+
				"running it on the request context lets the refund DB write abort after "+
				"the handler returns, silently losing the refund")
		require.False(t, got.snap.skipRefund,
			"the forwarded/skip decision must come from the spawn-time snapshot, not a "+
				"recycled *gin.Context mutated after the handler returned")
		require.Equal(t, 42, got.snap.userID,
			"user id must come from the spawn-time snapshot, not the mutated *gin.Context")
		require.Equal(t, 7, got.snap.tokenID)
		require.Equal(t, 99, got.snap.provisionalLogID)
	case <-time.After(2 * time.Second):
		t.Fatal("refund goroutine did not reach the observation seam")
	}
}
