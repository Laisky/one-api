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

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
)

// TestBilling_CrossPathConcurrency_ExactAccounting is the flagship cross-path concurrency +
// billing-correctness guard for the async/sync race remediation. It drives all THREE
// post-response background-billing primitives the remediation touches — concurrently, through
// a real gin.Engine (so the *gin.Context sync.Pool actually recycles contexts under live
// background goroutines), with each request on its OWN cancellable request context that is
// cancelled the instant the handler returns (simulating a client disconnect / handler return
// racing the background work):
//
//	bucket 0 SUCCESS  : pre-consume P, then post-billing reconcile of +(A-P) via
//	                    runPostBillingWithTimeout  => net charge A
//	bucket 1 REFUND   : pre-consume P, not-forwarded failure => scheduleConservativeRefund
//	                    returns P                  => net charge 0
//	bucket 2 FORWARDED: pre-consume P, forwarded failure => conservative refund SKIPPED
//	                    (no-underbilling)          => net charge P
//	bucket 3 ROLLBACK : pre-consume P, audio/video rollback returns P => net charge 0
//
// Across multiple rounds it asserts the user's quota balance equals the EXACT algebraic sum
// every round — proving, under heavy concurrency + pool recycling + request cancellation:
//   - DATA CONSISTENCY: every background goroutine acted on ITS OWN request's snapshot
//     (a cross-request bleed would corrupt the per-request amount and break the exact sum);
//   - REFUND DURABILITY: detached contexts mean cancellation never aborts a refund/charge
//     (a lost refund would over-charge; a lost charge would under-charge — either breaks it);
//   - NO DOUBLE/LOST billing: forwarded keeps, not-forwarded/rollback refund, success settles
//     exactly once;
//   - CLEAN LIFECYCLE: graceful.Drain returns nil each round (all tracked, bounded background
//     goroutines finished — none orphaned, none stuck past the billing timeout).
//
// Run under `-race`; with `-count=N` it doubles as the multi-round long-soak stress.
func TestBilling_CrossPathConcurrency_ExactAccounting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLog := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLog) })

	// Generous billing timeout so the background work always completes (it is the bound, not
	// the work, that we exercise elsewhere); here every charge/refund must actually settle.
	withBillingTimeout(t, 300)

	// In-memory sqlite serializes writers; funnel the pool to one connection with a long busy
	// timeout so concurrent quota UPDATEs queue instead of erroring. The goroutine spawning,
	// per-request snapshotting, context recycling and cancellation all still run concurrently
	// (production uses MySQL/Postgres which handle the write concurrency natively).
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(config.SQLMaxOpenConns) })
	require.NoError(t, model.DB.Exec("PRAGMA busy_timeout = 30000").Error)

	const (
		K         = int64(500_000_000) // start balance, large enough to never exhaust
		P         = int64(100)         // pre-consume per request
		A         = int64(150)         // settled charge for a SUCCESS request (delta = A-P)
		perBucket = 50                 // requests per bucket per round
		buckets   = 4
		rounds    = 5
		maxInF    = 32
	)
	// Net charge per round: SUCCESS contributes A each, FORWARDED contributes P each,
	// REFUND and ROLLBACK net to zero.
	perRoundDecrement := int64(perBucket) * (A + P)

	engine := gin.New()
	var dbErrs int64 // pre-consume / work DB errors, asserted == 0 after the wave
	engine.POST("/mix", func(c *gin.Context) {
		uid := atoiHeader(c, "x-uid")
		bucket := atoiHeader(c, "x-bucket")
		round := atoiHeader(c, "x-round")

		c.Set(ctxkey.Id, fallbackUserID)
		c.Set(ctxkey.TokenId, fallbackTokenID)
		c.Set(ctxkey.RequestId, fmt.Sprintf("mix-r%d-b%d-%d", round, bucket, uid))

		// Pre-consume P synchronously on the request goroutine (the deduction itself is not
		// the race under test; the background reconcile/refund is).
		if e := model.PostConsumeTokenQuota(c.Request.Context(), fallbackTokenID, P); e != nil {
			atomic.AddInt64(&dbErrs, 1)
		}
		markPreConsumed(c, P)

		switch bucket {
		case 0: // SUCCESS: reconcile the pre-consumed estimate up to the actual charge A.
			markBillingReconciled(c)
			lg := gmw.GetLogger(c)
			runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
				userID:          fallbackUserID,
				model:           "cross-path",
				requestID:       c.GetString(ctxkey.RequestId),
				startTime:       time.Now(),
				estimatedQuota:  func() float64 { return 0 },
				guardTimeoutLog: func() bool { return true },
				logMessage:      "CRITICAL BILLING TIMEOUT",
			}, func(ctx context.Context) {
				if e := model.PostConsumeTokenQuota(ctx, fallbackTokenID, A-P); e != nil {
					atomic.AddInt64(&dbErrs, 1)
				}
			})
		case 1: // not-forwarded failure: refund the whole pre-consume.
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
			scheduleConservativeRefund(c, P, fallbackTokenID, "do_request_failed")
		case 2: // forwarded failure: conservative skip keeps the pre-consume charged.
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
			scheduleConservativeRefund(c, P, fallbackTokenID, "upstream_http_error")
		case 3: // audio/video rollback: always refunds the pre-consume.
			goAudioRollbackPreConsumed(c, fallbackTokenID, P)
		}
	})

	n := perBucket * buckets
	for round := 0; round < rounds; round++ {
		// Reset to a known balance at the start of each round.
		require.NoError(t, model.DB.Model(&model.User{}).
			Where("id = ?", fallbackUserID).Update("quota", K).Error)
		atomic.StoreInt64(&dbErrs, 0)

		var wg sync.WaitGroup
		sem := make(chan struct{}, maxInF)
		for i := 0; i < n; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				// Each request gets its OWN cancellable context, cancelled the instant
				// ServeHTTP returns — racing gin's pool recycle against the background work.
				reqCtx, cancel := context.WithCancel(context.Background())
				req := httptest.NewRequest("POST", "/mix", nil).WithContext(reqCtx)
				req.Header.Set("x-uid", fmt.Sprintf("%d", i))
				req.Header.Set("x-bucket", fmt.Sprintf("%d", i%buckets))
				req.Header.Set("x-round", fmt.Sprintf("%d", round))
				engine.ServeHTTP(httptest.NewRecorder(), req)
				cancel()
			}(i)
		}
		wg.Wait()

		// Drain every spawned background goroutine (refunds, rollbacks, post-billing) before
		// reading the balance. A clean (nil) drain proves none were orphaned or stuck.
		drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		require.NoError(t, graceful.Drain(drainCtx),
			"round %d: graceful.Drain must return cleanly (no orphaned/stuck background billing)", round)
		cancel()

		require.EqualValues(t, 0, atomic.LoadInt64(&dbErrs),
			"round %d: unexpected DB errors during pre-consume/reconcile", round)

		got := reloadUserQuota(t)
		require.Equal(t, K-perRoundDecrement, got,
			"round %d: cross-path concurrent billing must net EXACTLY: want decrement %d "+
				"(SUCCESS=%d*%d + FORWARDED=%d*%d; REFUND/ROLLBACK net 0), got balance %d (start %d)",
			round, perRoundDecrement, perBucket, A, perBucket, P, got, K)
	}
}
