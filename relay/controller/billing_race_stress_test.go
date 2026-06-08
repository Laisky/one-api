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

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
)

// These stress tests fire many concurrent requests through a real gin.Engine so
// gin's *gin.Context sync.Pool actually recycles contexts while billing goroutines
// are still running. They assert two correctness properties that the async/sync
// race fix must guarantee under load:
//
//   - DATA CONSISTENCY: every billing goroutine acts on the values snapshotted from
//     ITS OWN request — never another request's recycled *gin.Context. We encode a
//     unique id per request into the request-scoped keys and assert the multiset of
//     values the goroutines observed is exactly {0..N-1}, each once: no loss, no
//     duplication, no cross-request bleed.
//   - REFUND DURABILITY: each goroutine runs on a non-cancelled (detached) context,
//     so the observed ctx error is always nil even though the request goroutine has
//     long returned and gin has recycled the context.
//
// Run under `-race`; releasing nothing and asserting on observed values keeps the
// result deterministic rather than depending on the race detector firing.

// runRefundStressWave fires `n` concurrent requests through engine and returns the
// per-request snapshots the refund goroutines actually ran with, keyed by the unique
// id carried in ctxkey.Id.
func runRefundStressWave(t *testing.T, engine *gin.Engine, n, concurrency int) map[int]conservativeRefundSnapshot {
	t.Helper()

	observed := make(chan conservativeRefundSnapshot, n)
	ctxErrs := make(chan error, n)
	refundObservedForTest = func(ctxErr error, snap conservativeRefundSnapshot) {
		ctxErrs <- ctxErr
		observed <- snap
	}
	refundGoroutineReleaseForTest = nil // do not park; let goroutines race the pool freely
	t.Cleanup(func() { refundObservedForTest = nil })

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

	// Wait for every spawned refund goroutine to finish so all observations land.
	drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drainCtx), "critical tasks did not drain")

	// Every refund goroutine must have run on a non-cancelled detached context.
	close(ctxErrs)
	for err := range ctxErrs {
		require.NoError(t, err, "refund goroutine ran on a cancelled context (refund-loss risk)")
	}

	close(observed)
	byID := make(map[int]conservativeRefundSnapshot, n)
	for snap := range observed {
		if _, dup := byID[snap.userID]; dup {
			t.Fatalf("duplicate observation for user id %d: a goroutine read another request's recycled context", snap.userID)
		}
		byID[snap.userID] = snap
	}
	return byID
}

// TestScheduleConservativeRefund_ConcurrentPoolRecycling validates the refund path
// under heavy concurrency + gin pool recycling.
func TestScheduleConservativeRefund_ConcurrentPoolRecycling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.POST("/stress", func(c *gin.Context) {
		uid := atoiHeader(c, "x-uid")
		// Distinct, recomputable request-scoped state per request.
		c.Set(ctxkey.Id, uid)
		c.Set(ctxkey.TokenId, uid+1_000_000)
		c.Set(ctxkey.ProvisionalLogId, uid+2_000_000)
		c.Set(ctxkey.RequestId, fmt.Sprintf("req-%d", uid))
		// Half the requests are "forwarded upstream" (conservative no-refund), half not.
		c.Set(ctxkey.UpstreamRequestPossiblyForwarded, uid%2 == 0)
		markPreConsumed(c, 1000)
		scheduleConservativeRefund(c, 1000, uid+1_000_000, "stress")
	})

	const (
		waves       = 3
		perWave     = 1500
		concurrency = 64
	)
	for w := 0; w < waves; w++ {
		byID := runRefundStressWave(t, engine, perWave, concurrency)
		require.Len(t, byID, perWave, "wave %d: lost or merged observations (cross-request contamination)", w)
		for uid := 0; uid < perWave; uid++ {
			snap, ok := byID[uid]
			require.True(t, ok, "wave %d: no observation for uid %d", w, uid)
			require.Equal(t, uid+1_000_000, snap.tokenID, "uid %d: token id from another request", uid)
			require.Equal(t, uid+2_000_000, snap.provisionalLogID, "uid %d: provisional log id from another request", uid)
			require.Equal(t, fmt.Sprintf("req-%d", uid), snap.requestID, "uid %d: request id from another request", uid)
			require.Equal(t, uid%2 == 0, snap.skipRefund, "uid %d: forwarded/skip decision from another request", uid)
		}
	}
}

// TestDetachForBilling_ConcurrentPoolRecycling validates the post-billing identity
// snapshot under heavy concurrency + gin pool recycling: each post-billing goroutine
// must resolve its OWN request's identifiers from the detached snapshot.
func TestDetachForBilling_ConcurrentPoolRecycling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		n           = 3000
		concurrency = 64
	)
	observed := make(chan billingIdentity, n)

	engine := gin.New()
	engine.POST("/stress", func(c *gin.Context) {
		uid := atoiHeader(c, "x-uid")
		c.Set(ctxkey.RequestId, fmt.Sprintf("rid-%d", uid))
		c.Set(ctxkey.ProvisionalLogId, uid+5_000_000)
		ctx := detachForBilling(c)
		graceful.GoCritical(ctx, "stressPostBilling", func(ctx context.Context) {
			observed <- billingIdentityFromContext(ctx)
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

	drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drainCtx))

	close(observed)
	seen := make(map[string]bool, n)
	count := 0
	for id := range observed {
		count++
		require.False(t, seen[id.requestID], "duplicate request id %q observed (cross-request contamination)", id.requestID)
		seen[id.requestID] = true
		// The provisional log id encodes the same uid as the request id; they must agree.
		var uid int
		_, err := fmt.Sscanf(id.requestID, "rid-%d", &uid)
		require.NoError(t, err)
		require.Equal(t, uid+5_000_000, id.provisionalLogID,
			"request %q: provisional log id from another request's recycled context", id.requestID)
	}
	require.Equal(t, n, count, "lost post-billing observations under pool recycling")
}

// atoiHeader parses an integer request header; fatal-free helper for the handlers.
func atoiHeader(c *gin.Context, key string) int {
	var v int
	_, _ = fmt.Sscanf(c.GetHeader(key), "%d", &v)
	return v
}
