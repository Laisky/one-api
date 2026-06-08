package controller

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/metrics"
)

// postBillingTimeoutInfo carries the per-request metadata the shared post-billing timeout
// monitor needs to emit the "CRITICAL BILLING TIMEOUT" audit log + metric when the
// post-consume work overruns config.BillingTimeoutSec. Every field is a value snapshotted
// on the request goroutine; it holds no *gin.Context.
//
// Together with detachForBilling's billingIdentity snapshot and conservativeRefundSnapshot,
// it formalizes the per-attempt "billing state": the closed set of request-scoped values a
// post-response billing goroutine may depend on, all captured before the spawn so the
// goroutine never touches a *gin.Context that gin recycles via sync.Pool once the handler
// returns.
type postBillingTimeoutInfo struct {
	userID    int
	channelID int
	model     string
	requestID string
	startTime time.Time

	// estimatedQuota computes the quota figure logged on timeout. It varies per relay
	// mode: float64(promptTokens+completionTokens)*ratio for token-billed modes, and
	// float64(totalQuota) for rerank/ocr. It is evaluated ONLY on the DeadlineExceeded
	// branch (and only when guardTimeoutLog passes), preserving each site's exact behavior.
	estimatedQuota func() float64

	// guardTimeoutLog gates the timeout log + metric. It reproduces the per-site
	// "&& usage != nil" guard verbatim: sites that have it pass func() bool { return usage != nil },
	// sites that dereference usage unconditionally pass func() bool { return true }.
	guardTimeoutLog func() bool

	// logMessage is "CRITICAL BILLING TIMEOUT" everywhere except the websocket path, which
	// uses "CRITICAL BILLING TIMEOUT (websocket)".
	logMessage string

	// includeElapsedField controls whether the elapsedTime zap field is logged. The
	// websocket path is the only site that omits it.
	includeElapsedField bool
}

// goDetachedBillingWork spawns fn in a lifecycle-managed (graceful.GoCritical) critical
// goroutine on spawnCtx — which MUST be a detached, c-free context (relayctx.Detach /
// detachForBilling) captured on the request goroutine — bounded by config.BillingTimeoutSec.
//
// It is the shared spawn primitive for post-response billing/refund side effects that have
// NO timeout-audit of their own (the refunds and rollbacks). Two invariants it enforces:
//
//   - DETACHED: spawnCtx is non-cancelled, so a client disconnect (request-context
//     cancellation) can never abort the refund/billing DB write — the silent refund-loss
//     this whole remediation fixes.
//   - BOUNDED: the BillingTimeoutSec deadline ensures a stuck billing DB can neither leak the
//     goroutine/connection nor extend graceful drain indefinitely. fn runs DIRECTLY in the
//     tracked closure (not a detached sub-goroutine), so graceful.Drain always waits for it.
//
// fn MUST use the passed ctx for its DB writes and logger (gmw.GetLogger(ctx)), never *c.
// goDetachedBillingWork is registered as a spawn helper in the no-gin-context-in-goroutine
// ast-grep rule, so fn literals passed to it are scanned for stray references to c.
func goDetachedBillingWork(spawnCtx context.Context, name string, fn func(ctx context.Context)) {
	graceful.GoCritical(spawnCtx, name, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(config.BillingTimeoutSec)*time.Second)
		defer cancel()
		fn(ctx)
	})
}

// runPostBillingWithTimeout runs post-response billing work in a lifecycle-managed,
// DETACHED critical goroutine bounded by config.BillingTimeoutSec, emitting the
// "CRITICAL BILLING TIMEOUT" audit log + RecordBillingTimeout metric if the work overruns.
// It de-duplicates the timeout-monitor machinery that was previously copy-pasted across
// every relay post-billing site (text x2, response, response websocket, response fallback
// x2, claude messages, rerank, ocr).
//
// spawnCtx MUST be detachForBilling(c) captured on the request goroutine BEFORE this call,
// so the spawned goroutine never holds a *gin.Context that gin recycles via sync.Pool once
// the handler returns. runPostBillingWithTimeout is registered as a spawn helper in the
// no-gin-context-in-goroutine ast-grep rule, so func literals passed as `work` (and the
// info closures) are scanned for stray references to the request c.
//
// work performs the per-site post-consume call AND its request-cost reconcile on the
// timeout-bounded ctx. ALL per-site behavioral differences (which postConsume* function,
// which reconcile guard, which extra captured values) live inside `work`, so this helper
// introduces ZERO behavioral drift versus the hand-written blocks it replaces. `work` and
// the info closures must close over values snapshotted on the request goroutine.
//
// TIMEOUT SEMANTICS (the timeout CANCELS the billing attempt — it does not let it run
// unbounded): `work` runs on `ctx`, which is bounded by config.BillingTimeoutSec. spawnCtx
// is a non-cancelled detached context (detachForBilling), so a client disconnect can never
// abort billing — only this deadline can. When the deadline fires, `ctx` is cancelled, so a
// `work` DB write that honors ctx is aborted, and the "CRITICAL BILLING TIMEOUT" audit +
// RecordBillingTimeout metric fire so the aborted attempt stays visible (recovery is the
// dead-letter-queue TODO below). This is deliberate: a stuck billing DB must never leak a
// goroutine/connection or extend graceful drain indefinitely. post-consume only RECONCILES
// the already pre-consumed estimate to the actual quota, so an aborted attempt leaves the
// user on the pre-consumed estimate (already charged at request start) — an un-reconciled
// estimate, never an un-charged request.
//
// LIFECYCLE: the tracked graceful.GoCritical closure joins the inner `work` goroutine
// (`<-done` on BOTH select branches) before it returns, so the billing goroutine stays
// tracked by graceful.Drain even after the timeout fires — graceful shutdown will not exit
// out from under an in-flight billing write. Because `work`'s ctx is already cancelled when
// the timeout branch runs, the join returns promptly and cannot extend drain past the
// deadline (a driver that honors context cancellation aborts the query at once).
func runPostBillingWithTimeout(
	spawnCtx context.Context,
	name string,
	lg glog.Logger,
	info postBillingTimeoutInfo,
	work func(ctx context.Context),
) {
	graceful.GoCritical(spawnCtx, name, func(ctx context.Context) {
		// Bound the billing attempt by the configurable billing timeout. spawnCtx is a
		// non-cancelled detached context, so this deadline (never a client disconnect) is
		// the only thing that can cancel work.
		ctx, cancel := context.WithTimeout(ctx, time.Duration(config.BillingTimeoutSec)*time.Second)
		defer cancel()

		// Run work in its own goroutine so the timeout branch can fire the audit while work
		// is still in flight, then join it (below) so it stays lifecycle-tracked.
		done := make(chan struct{})
		go func() {
			defer close(done)
			work(ctx)
		}()

		select {
		case <-done:
			// Billing completed (or was aborted by the deadline) before we observed timeout.
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && info.guardTimeoutLog() {
				estimatedQuota := info.estimatedQuota()
				elapsedTime := time.Since(info.startTime)

				fields := make([]zap.Field, 0, 5)
				fields = append(fields,
					zap.String("model", info.model),
					zap.String("requestId", info.requestID),
					zap.Int("userId", info.userID),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
				)
				if info.includeElapsedField {
					fields = append(fields, zap.Duration("elapsedTime", elapsedTime))
				}
				lg.Error(info.logMessage, fields...)

				metrics.GlobalRecorder.RecordBillingTimeout(info.userID, info.channelID, info.model, estimatedQuota, elapsedTime)
				// TODO: Implement dead letter queue or retry mechanism for failed billing
			}
			// Join the inner goroutine so it remains tracked by graceful.Drain. work's ctx is
			// already cancelled here, so this returns as soon as work unwinds.
			<-done
		}
	})
}
