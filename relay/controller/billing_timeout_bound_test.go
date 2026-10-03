package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
)

// These tests pin the contract of goDetachedBillingWork — the shared spawn primitive that
// runs every post-response refund/rollback on a detached, c-free context BOUNDED by
// config.BillingTimeoutSec. They cover two properties the bound must guarantee:
//
//   - BOUNDED (reproduce-first for the timeout fix): the refund goroutine's ctx carries the
//     BillingTimeoutSec deadline, so a stuck billing DB cannot run unbounded. With the bound
//     removed the goroutine's ctx error stays nil forever; with it, a 0s timeout makes the
//     goroutine observe context.DeadlineExceeded. This is the discriminator: it FAILS against
//     the pre-fix `graceful.GoCritical(relayctx.Detach(c), ...)` (no timeout) and PASSES now.
//   - TRACKED: fn runs DIRECTLY inside the tracked graceful.GoCritical closure (not a detached
//     sub-goroutine), so graceful.Drain blocks until a parked refund unwinds — shutdown never
//     exits out from under an in-flight refund DB write.

// TestGoDetachedBillingWork_RefundBoundedByBillingTimeout is the reproduce-first guard for the
// refund timeout bound. It drives scheduleConservativeRefund (which spawns via
// goDetachedBillingWork) with config.BillingTimeoutSec == 0 and asserts the goroutine observes
// context.DeadlineExceeded — proving its context is bounded. Against the pre-fix unbounded
// spawn the observed error is nil.
//
// It asserts on the OBSERVED ctx error (the deterministic signal), never on a DB balance:
// in-memory sqlite ignores context cancellation, so the bound is only observable via the
// context, exactly as the production MySQL/Postgres drivers would abort the query.
func TestGoDetachedBillingWork_RefundBoundedByBillingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withBillingTimeout(t, 0) // WithTimeout(ctx, 0) is already past its deadline

	observed := make(chan error, 1)
	refundObservedForTest = func(ctxErr error, _ conservativeRefundSnapshot) {
		observed <- ctxErr
	}
	refundGoroutineReleaseForTest = nil
	t.Cleanup(func() { refundObservedForTest = nil })

	c, _ := newBillingTestContext(t)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 7)
	c.Set(ctxkey.RequestId, "req_bounded")
	// NOT forwarded => the snapshot would refund; the bound must still apply.
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
	markPreConsumed(c, 999)

	scheduleConservativeRefund(c, 999, 7, "upstream_http_error")

	drainBilling(t)

	select {
	case err := <-observed:
		require.Error(t, err, "refund ctx must be bounded by config.BillingTimeoutSec")
		require.True(t, errors.Is(err, context.DeadlineExceeded),
			"refund ctx must hit DeadlineExceeded under a 0s billing timeout, got %v; "+
				"a nil error means the detached refund goroutine is unbounded (a stuck DB "+
				"could leak the goroutine and extend graceful drain)", err)
	case <-time.After(2 * time.Second):
		t.Fatal("refund goroutine never reached the observation seam")
	}
}

// TestGoDetachedBillingWork_DrainWaitsForParkedRefund pins the lifecycle contract: a refund
// parked inside goDetachedBillingWork keeps graceful.Drain blocked, because fn runs directly
// in the tracked GoCritical closure. Drain must not return until the refund unwinds.
func TestGoDetachedBillingWork_DrainWaitsForParkedRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withBillingTimeout(t, 300) // generous: the park, not the timeout, governs this test

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(release) }) }

	ran := make(chan struct{})
	refundGoroutineReleaseForTest = release
	refundObservedForTest = func(_ error, _ conservativeRefundSnapshot) { close(ran) }
	t.Cleanup(func() {
		releaseGate()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		refundGoroutineReleaseForTest = nil
		refundObservedForTest = nil
	})

	c, _ := newBillingTestContext(t)
	c.Set(ctxkey.Id, 2)
	c.Set(ctxkey.TokenId, 8)
	c.Set(ctxkey.RequestId, "req_drain_refund")
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
	markPreConsumed(c, 500)

	scheduleConservativeRefund(c, 500, 8, "upstream_http_error")

	// Drain in the background; it must stay blocked while the refund is parked.
	drainReturned := make(chan error, 1)
	go func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		drainReturned <- graceful.Drain(drainCtx)
	}()

	select {
	case <-drainReturned:
		t.Fatal("graceful.Drain returned while the refund goroutine was still parked; " +
			"goDetachedBillingWork must run fn in the tracked closure so Drain waits for it")
	case <-time.After(300 * time.Millisecond):
		// Still blocked, as required.
	}

	releaseGate()
	select {
	case err := <-drainReturned:
		require.NoError(t, err, "Drain must return cleanly once the refund unwinds")
	case <-time.After(3 * time.Second):
		t.Fatal("graceful.Drain did not return after the refund was released")
	}
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("refund goroutine never ran")
	}
}
