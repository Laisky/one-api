package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
)

// These tests validate runPostBillingWithTimeout — the shared post-billing timeout monitor
// that de-duplicates the spawn/timeout/done/select/CRITICAL-BILLING-TIMEOUT machinery across
// every relay post-billing site. They assert the behavioral invariants the de-duplication
// must preserve with ZERO drift:
//
//   - SUCCESS: work runs to completion and the timeout branch is never consulted.
//   - TIMEOUT: when work overruns config.BillingTimeoutSec, the timeout branch fires the
//     CRITICAL BILLING TIMEOUT audit. The timeout CANCELS the billing attempt (work runs on
//     the timeout-bounded ctx); the tracked closure then JOINS the worker so it stays
//     lifecycle-tracked by graceful.Drain.
//   - GUARD: guardTimeoutLog()==false (the per-site `&& usage != nil` guard) short-circuits
//     before estimatedQuota / log / metric.
//   - DETACHED: work runs on a c-free, non-cancelled context even after the request context
//     is cancelled (only the billing timeout, never a client disconnect, can cancel it).
//   - LIFECYCLE: after a timeout, graceful.Drain still waits for the inner billing goroutine
//     to unwind (it is joined, not orphaned).
//   - DATA CONSISTENCY: under heavy concurrency + gin pool recycling, each goroutine resolves
//     ITS OWN request's identifiers from the detached snapshot, never another request's.

// withBillingTimeout sets config.BillingTimeoutSec for the test and restores it afterward.
func withBillingTimeout(t *testing.T, sec int) {
	t.Helper()
	prev := config.BillingTimeoutSec
	config.BillingTimeoutSec = sec
	t.Cleanup(func() { config.BillingTimeoutSec = prev })
}

// newBillingTestContext builds a gin context with a cancellable request context.
func newBillingTestContext(t *testing.T) (*gin.Context, context.CancelFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	reqCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(reqCtx)
	return c, cancel
}

func TestRunPostBillingWithTimeout_SuccessRunsWorkNoTimeout(t *testing.T) {
	withBillingTimeout(t, 300) // generous; work completes well within it

	c, _ := newBillingTestContext(t)
	lg := gmw.GetLogger(c)

	var workCalled, estimatedCalled, guardCalled atomic.Bool
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		estimatedQuota:  func() float64 { estimatedCalled.Store(true); return 1 },
		guardTimeoutLog: func() bool { guardCalled.Store(true); return true },
	}, func(ctx context.Context) {
		workCalled.Store(true)
	})

	drainBilling(t)
	require.True(t, workCalled.Load(), "work must run")
	require.False(t, estimatedCalled.Load(), "no timeout => estimatedQuota must not be consulted")
	require.False(t, guardCalled.Load(), "no timeout => guard must not be consulted")
}

func TestRunPostBillingWithTimeout_TimeoutFiresButWorkStillCompletes(t *testing.T) {
	withBillingTimeout(t, 0) // immediate timeout (context.WithTimeout(ctx, 0) is already past deadline)

	c, _ := newBillingTestContext(t)
	lg := gmw.GetLogger(c)

	release := make(chan struct{})
	workDone := make(chan struct{})
	var workStarted, workCompleted, estimatedCalled atomic.Bool
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		userID:          1,
		channelID:       1,
		model:           "test-model",
		requestID:       "req-timeout",
		startTime:       time.Now(),
		estimatedQuota:  func() float64 { estimatedCalled.Store(true); return 42 },
		guardTimeoutLog: func() bool { return true },
		logMessage:      "CRITICAL BILLING TIMEOUT",
	}, func(ctx context.Context) {
		workStarted.Store(true)
		<-release // outlive the timeout
		workCompleted.Store(true)
		close(workDone)
	})

	// The helper must not block the caller; the timeout branch fires the audit while work is
	// still in flight (the alert does not wait for work to finish).
	require.Eventually(t, estimatedCalled.Load, 2*time.Second, 5*time.Millisecond,
		"timeout branch must fire (estimatedQuota consulted) when work overruns the billing timeout")
	require.False(t, workCompleted.Load(),
		"work should still be parked here (the timeout audit fires without waiting for work)")

	// After the audit, the tracked closure JOINS the inner billing goroutine, so it completes
	// in-process (it is not orphaned). This test work ignores ctx, mirroring an in-memory DB
	// that runs to completion despite the cancelled timeout ctx; release it and confirm.
	close(release)
	select {
	case <-workDone:
	case <-time.After(2 * time.Second):
		t.Fatal("billing work did not complete after the timeout fired")
	}
	require.True(t, workStarted.Load())
	require.True(t, workCompleted.Load(),
		"work must still complete after the timeout fired (the closure joins it, no leak/deadlock)")
}

func TestRunPostBillingWithTimeout_GuardSuppressesTimeoutBranch(t *testing.T) {
	withBillingTimeout(t, 0)

	c, _ := newBillingTestContext(t)
	lg := gmw.GetLogger(c)

	release := make(chan struct{})
	var estimatedCalled atomic.Bool
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		estimatedQuota:  func() float64 { estimatedCalled.Store(true); return 1 },
		guardTimeoutLog: func() bool { return false }, // mimics usage == nil
	}, func(ctx context.Context) {
		<-release
	})

	// Give the select time to observe ctx.Done() and run the (guarded-out) branch.
	time.Sleep(200 * time.Millisecond)
	require.False(t, estimatedCalled.Load(),
		"guardTimeoutLog()==false must short-circuit before estimatedQuota (no log/metric)")

	close(release)
	drainBilling(t)
}

func TestRunPostBillingWithTimeout_WorkRunsOnDetachedContext(t *testing.T) {
	withBillingTimeout(t, 300)

	c, cancelReq := newBillingTestContext(t)
	lg := gmw.GetLogger(c)

	type obs struct {
		carriesC bool
		ctxErr   error
	}
	got := make(chan obs, 1)
	gate := make(chan struct{})
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		estimatedQuota:  func() float64 { return 0 },
		guardTimeoutLog: func() bool { return true },
	}, func(ctx context.Context) {
		<-gate // wait until the request ctx is cancelled
		_, carriesC := gmw.GetGinCtxFromStdCtx(ctx)
		got <- obs{carriesC: carriesC, ctxErr: ctx.Err()}
	})

	cancelReq() // request ends / client disconnects
	close(gate)

	select {
	case o := <-got:
		require.False(t, o.carriesC, "work ctx must not carry *gin.Context (detachForBilling => c-free)")
		require.NoError(t, o.ctxErr, "work ctx must survive request-context cancellation (detached, non-cancelled)")
	case <-time.After(3 * time.Second):
		t.Fatal("work did not run")
	}
	drainBilling(t)
}

func TestRunPostBillingWithTimeout_ConcurrentPoolRecycling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withBillingTimeout(t, 300)

	const (
		n           = 3000
		concurrency = 64
	)
	observed := make(chan string, n)

	engine := gin.New()
	engine.POST("/stress", func(c *gin.Context) {
		uid := atoiHeader(c, "x-uid")
		c.Set(ctxkey.RequestId, fmt.Sprintf("rid-%d", uid))
		c.Set(ctxkey.ProvisionalLogId, uid+7_000_000)
		lg := gmw.GetLogger(c)
		runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
			estimatedQuota:  func() float64 { return 0 },
			guardTimeoutLog: func() bool { return true },
		}, func(ctx context.Context) {
			id := billingIdentityFromContext(ctx)
			// Encode both fields so a cross-request bleed is detectable.
			observed <- fmt.Sprintf("%s|%d", id.requestID, id.provisionalLogID)
		})
	})

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(id int) {
			defer wg.Done()
			defer func() { <-sem }()
			req := httptest.NewRequest("POST", "/stress", nil)
			req.Header.Set("x-uid", fmt.Sprintf("%d", id))
			engine.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()
	drainBilling(t)

	close(observed)
	seen := make(map[string]bool, n)
	count := 0
	for v := range observed {
		count++
		require.False(t, seen[v], "duplicate post-billing observation %q (cross-request contamination)", v)
		seen[v] = true
		var uid, prov int
		_, err := fmt.Sscanf(v, "rid-%d|%d", &uid, &prov)
		require.NoError(t, err, "unexpected observation format %q", v)
		require.Equal(t, uid+7_000_000, prov,
			"provisional log id from another request's recycled context for uid %d", uid)
	}
	require.Equal(t, n, count, "lost post-billing observations under pool recycling")
}

// TestRunPostBillingWithTimeout_DrainWaitsForInnerGoroutineAfterTimeout pins the corrected
// shutdown contract of the helper's timeout path (reproduce-first for the lifecycle-tracking
// fix). Before the fix the OUTER graceful.GoCritical closure returned the instant the timeout
// branch ran, so graceful.Drain's WaitGroup stopped tracking the INNER billing goroutine,
// which kept running UNTRACKED — graceful shutdown could exit out from under an in-flight
// billing write. The fix JOINS the worker (`<-done`) on the timeout branch, so the closure
// stays alive — and graceful.Drain stays blocked — until the inner billing goroutine unwinds.
//
// This test would FAIL against the old orphaning implementation (Drain returns while work is
// parked, so workCompleted is false after Drain) and PASSES with the join: Drain stays
// blocked while work is parked and only returns once work has finished.
func TestRunPostBillingWithTimeout_DrainWaitsForInnerGoroutineAfterTimeout(t *testing.T) {
	withBillingTimeout(t, 0) // immediate timeout

	c, _ := newBillingTestContext(t)
	lg := gmw.GetLogger(c)

	release := make(chan struct{})
	var workCompleted, estimatedCalled atomic.Bool
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		userID:          1,
		channelID:       1,
		model:           "drain-contract",
		requestID:       "req-drain",
		startTime:       time.Now(),
		estimatedQuota:  func() float64 { estimatedCalled.Store(true); return 1 },
		guardTimeoutLog: func() bool { return true },
		logMessage:      "CRITICAL BILLING TIMEOUT",
	}, func(ctx context.Context) {
		<-release // still in flight when Drain is called
		workCompleted.Store(true)
	})

	// Wait until the timeout branch has fired (audit emitted; closure is now joining work).
	require.Eventually(t, estimatedCalled.Load, 2*time.Second, 5*time.Millisecond)

	// Drain in the background; it must stay blocked while the inner billing goroutine is
	// parked, proving the worker is still lifecycle-tracked after the timeout fired.
	drainReturned := make(chan error, 1)
	go func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		drainReturned <- graceful.Drain(drainCtx)
	}()

	select {
	case <-drainReturned:
		t.Fatal("graceful.Drain returned while billing work was still in flight; the inner " +
			"billing goroutine is not tracked (orphaned on timeout)")
	case <-time.After(300 * time.Millisecond):
		// Still blocked, as required.
	}
	require.False(t, workCompleted.Load(), "work must still be parked while Drain is blocked")

	// Release the worker; Drain must now observe it finish and return cleanly.
	close(release)
	select {
	case err := <-drainReturned:
		require.NoError(t, err, "Drain must return once the joined billing goroutine completes")
	case <-time.After(3 * time.Second):
		t.Fatal("graceful.Drain did not return after the billing goroutine completed")
	}
	require.True(t, workCompleted.Load())
}
