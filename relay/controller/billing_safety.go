package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// shouldSkipPreConsumedRefund reports whether a refund should be skipped because
// the request may already have been forwarded upstream.
//
// Parameters:
//   - c: request context containing forwarding marker.
//
// Returns:
//   - bool: true when conservative policy requires skipping refund.
func shouldSkipPreConsumedRefund(c *gin.Context) bool {
	if c == nil {
		return false
	}
	forwardedAny, exists := c.Get(ctxkey.UpstreamRequestPossiblyForwarded)
	if !exists {
		return false
	}
	forwarded, ok := forwardedAny.(bool)
	return ok && forwarded
}

// userVisibleModelName returns the origin/public model name for user-facing responses and logs.
func userVisibleModelName(meta *metalib.Meta, fallback string) string {
	if meta != nil {
		if origin := strings.TrimSpace(meta.OriginModelName); origin != "" {
			return origin
		}
	}
	return strings.TrimSpace(fallback)
}

// conservativeRefundSnapshot captures every request-scoped value the conservative
// refund needs, taken on the request goroutine. It lets the refund run later — even
// on a background goroutine after the handler returns — without ever touching the
// recycled *gin.Context (gin v1.12.0 pools and reset()s c the instant ServeHTTP
// returns, so reading c.Keys from a background goroutine is a use-after-return race).
type conservativeRefundSnapshot struct {
	skipRefund       bool // request may have been forwarded upstream => no-underbilling skip
	userID           int
	tokenID          int
	provisionalLogID int
	requestID        string
	reason           string
	quota            int64
}

// newConservativeRefundSnapshot captures the refund snapshot from c. It must be
// called on the request goroutine, before any spawn.
func newConservativeRefundSnapshot(c *gin.Context, quota int64, tokenID int, reason string) conservativeRefundSnapshot {
	return conservativeRefundSnapshot{
		skipRefund:       shouldSkipPreConsumedRefund(c),
		userID:           c.GetInt(ctxkey.Id),
		tokenID:          tokenID,
		provisionalLogID: c.GetInt(ctxkey.ProvisionalLogId),
		requestID:        c.GetString(ctxkey.RequestId),
		reason:           reason,
		quota:            quota,
	}
}

// refund executes the conservative pre-consumed quota refund using ONLY the
// snapshotted values — no *gin.Context, no gmw.*(c). The logger is resolved from
// ctx (relayctx.Detach carries the request logger by value), so this is safe to run
// on a background goroutine.
//
// ctx must be a non-cancelled context (relayctx.Detach) when this runs after the
// handler returns, so the refund DB write is not aborted by request-context
// cancellation.
//
// Returns true when the refund was executed; false when skipped for no-underbilling
// safety or when the refund DB write failed.
func (s conservativeRefundSnapshot) refund(ctx context.Context) bool {
	if s.quota <= 0 {
		return false
	}

	lg := gmw.GetLogger(ctx)
	if s.skipRefund {
		lg.Warn("skip pre-consumed refund to prevent underbilling",
			zap.Int64("pre_consumed_quota", s.quota),
			zap.Int("token_id", s.tokenID),
			zap.String("reason", s.reason),
		)
		return false
	}

	// Reconcile provisional log to 0 so it doesn't appear as a duplicate entry.
	if s.provisionalLogID > 0 {
		if err := model.ReconcileConsumeLog(ctx, s.provisionalLogID, 0,
			fmt.Sprintf("refunded: %s", s.reason), 0, 0, 0, nil); err != nil {
			lg.Warn("failed to reconcile provisional log on refund",
				zap.Error(err), zap.Int("provisional_log_id", s.provisionalLogID))
		}
	}

	// Refund the quota via model.PostConsumeTokenQuota directly rather than
	// billing.ReturnPreConsumedQuota (which logs and swallows the error). The caller
	// has already marked billing reconciled SYNCHRONOUSLY, so the deferred
	// billingAuditSafetyNet can no longer catch a failed refund; emit a CRITICAL audit
	// signal here instead so a lost refund stays visible for manual reconciliation.
	if err := model.PostConsumeTokenQuota(ctx, s.tokenID, -s.quota); err != nil {
		lg.Error("CRITICAL BILLING AUDIT: conservative refund failed to return pre-consumed quota; "+
			"billing was already marked reconciled, manual reconciliation may be required",
			zap.Error(err),
			zap.Int64("pre_consumed_quota", s.quota),
			zap.Int("user_id", s.userID),
			zap.Int("token_id", s.tokenID),
			zap.String("request_id", s.requestID),
			zap.String("reason", s.reason),
		)
		return false
	}

	if s.userID > 0 {
		syncUserQuotaCacheAfterRefund(ctx, s.userID, s.reason)
	}
	return true
}

// returnPreConsumedQuotaConservative refunds pre-consumed quota only when the request
// has not potentially been forwarded upstream.
//
// This is the SYNCHRONOUS chokepoint, called on the request goroutine from terminal
// error paths; reading c here is safe. For asynchronous refunds spawned into a
// background goroutine, use scheduleConservativeRefund, which snapshots c up front and
// runs the same refund body on a detached, c-free context.
//
// Parameters:
//   - ctx: execution context for quota refund operations.
//   - c: request context carrying forwarding marker and logger.
//   - preConsumedQuota: amount to refund.
//   - tokenID: token identifier used for quota accounting.
//   - reason: short reason label for logs.
//
// Returns:
//   - bool: true when refund was executed, false when skipped for no-underbilling safety.
func returnPreConsumedQuotaConservative(
	ctx context.Context,
	c *gin.Context,
	preConsumedQuota int64,
	tokenID int,
	reason string,
) bool {
	if preConsumedQuota <= 0 {
		return false
	}
	if c == nil {
		// No request context to snapshot; best-effort refund with the values we have.
		return conservativeRefundSnapshot{tokenID: tokenID, reason: reason, quota: preConsumedQuota}.refund(ctx)
	}

	snap := newConservativeRefundSnapshot(c, preConsumedQuota, tokenID, reason)
	// Mark reconciled on the request goroutine in both the skip and refund cases so the
	// deferred billingAuditSafetyNet observes it (preserves the previous behavior).
	markBillingReconciled(c)
	return snap.refund(ctx)
}

// ResetPerAttemptBillingForRetry refunds and clears the request-scoped billing
// state of a just-failed/abandoned relay attempt so the next cross-channel retry
// starts from a clean slate.
//
// It must be called by the retry loop only once a retry channel has been
// selected and the loop has definitively decided to retry (i.e. immediately
// before middleware.SetupContextForSelectedChannel). Terminal failures never
// reach this point, so the no-underbilling guarantee on terminal failures is
// preserved.
//
// The four per-attempt billing ctxkeys (UpstreamRequestPossiblyForwarded,
// PreConsumedQuotaAmount, ProvisionalLogId, BillingReconciled) are set per
// attempt but never reset between attempts; SetupContextForSelectedChannel only
// resets channel keys. Left untouched, the next attempt pre-consumes again and
// post-consumes in full while the abandoned attempt's pre-consume is never
// refunded (conservative skip on a forwarded-then-failed attempt), double
// charging the user.
//
// Exactly-once reasoning (verified against billing_safety.go +
// claude_messages.go refund call sites): the only refund chokepoint is
// returnPreConsumedQuotaConservative, which NEVER zeroes PreConsumedQuotaAmount.
// When the attempt was not forwarded it already refunded (or the billing audit
// safety net did), so re-refunding here would over-credit. When the attempt was
// forwarded, it skipped the refund and the pre-consumed quota is still
// outstanding. Gating the refund on shouldSkipPreConsumedRefund (i.e. the
// forwarded marker) therefore yields EXACTLY one outcome per attempt: one charge
// on success, one refund when abandoned — never both, never twice.
//
// This helper is generic: all relay modes share the same retry loop and the same
// per-attempt ctxkeys, so it covers text/response/claude/etc.
func ResetPerAttemptBillingForRetry(ctx context.Context, c *gin.Context) {
	if c == nil {
		return
	}

	lg := gmw.GetLogger(c)
	userID := c.GetInt(ctxkey.Id)
	tokenID := c.GetInt(ctxkey.TokenId)
	amount := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	provID := c.GetInt(ctxkey.ProvisionalLogId)

	// Only refund/void when the abandoned attempt left its pre-consumed quota
	// outstanding, which happens exactly when the conservative policy skipped the
	// refund because the request may have been forwarded upstream. In every other
	// case the normal refund path (or the billing audit safety net) has already
	// returned the quota, so doing it again would over-credit the user.
	if amount > 0 && shouldSkipPreConsumedRefund(c) {
		const reason = "refunded: superseded by cross-channel retry"
		lg.Info("refunding abandoned attempt pre-consumed quota before cross-channel retry",
			zap.Int64("pre_consumed_quota", amount),
			zap.Int("token_id", tokenID),
			zap.Int("provisional_log_id", provID),
		)
		// Mirror the existing best-effort refund pattern: run the side effects in a
		// lifecycle-managed, BillingTimeoutSec-bounded critical goroutine so a slow DB never
		// blocks the retry or extends graceful drain. relayctx.Detach gives the goroutine a
		// non-cancelled, c-free context (all the values it needs are already value-captured
		// above), so it neither aborts on request cancellation nor races gin's recycle of c.
		goDetachedBillingWork(relayctx.Detach(c), "resetPerAttemptBillingForRetry", func(bctx context.Context) {
			billing.ReturnPreConsumedQuota(bctx, amount, tokenID)
			syncUserQuotaCacheAfterRefund(bctx, userID, "cross_channel_retry")
			if provID > 0 {
				if err := model.ReconcileConsumeLog(bctx, provID, 0, reason, 0, 0, 0, nil); err != nil {
					lg.Warn("failed to void provisional log on cross-channel retry refund",
						zap.Error(errors.Wrapf(err, "reconcile provisional log %d to zero", provID)),
						zap.Int("provisional_log_id", provID),
					)
				}
			}
		})
	}

	// Clear the per-attempt billing markers so the next attempt starts clean and
	// its own pre-consume/refund accounting is independent of this attempt.
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
	c.Set(ctxkey.PreConsumedQuotaAmount, int64(0))
	c.Set(ctxkey.ProvisionalLogId, 0)
	c.Set(ctxkey.BillingReconciled, false)
}

// refundGoroutineReleaseForTest, when non-nil, blocks the asynchronous refund
// goroutine spawned by scheduleConservativeRefund until the channel is closed.
// It exists solely to make the synchronous-reconcile guarantee deterministically
// testable and is always nil in production builds.
var refundGoroutineReleaseForTest chan struct{}

// refundObservedForTest, when non-nil, receives the context error and the snapshot
// the scheduleConservativeRefund goroutine actually runs with, then short-circuits the
// real DB refund. Tests use it to assert the goroutine (a) runs on a non-cancelled,
// detached context (refund-loss fix) and (b) acts on values snapshotted at spawn time,
// not on a recycled/mutated *gin.Context. Always nil in production builds.
var refundObservedForTest func(ctxErr error, snap conservativeRefundSnapshot)

// scheduleConservativeRefund performs the conservative pre-consumed quota refund
// in a lifecycle-managed critical goroutine so a slow DB never blocks the handler.
//
// Billing is marked reconciled SYNCHRONOUSLY here, before the goroutine is
// spawned. This is essential: the deferred billingAuditSafetyNet runs on the
// request goroutine the instant the handler returns, which can happen before the
// refund goroutine is scheduled. If the reconciled flag were only set inside the
// goroutine, the safety net would observe an unreconciled pre-consume on a request
// already forwarded upstream — which it cannot auto-refund — and emit a
// false-positive "manual reconciliation required" CRITICAL alarm. Marking reconciled
// up front mirrors the success path, which also marks synchronously before spawning
// its postBilling goroutine.
//
// The refund runs via goDetachedBillingWork on relayctx.Detach(c): a non-cancelled, c-free
// background context bounded by config.BillingTimeoutSec. Non-cancelled means a client
// disconnect (request-context cancellation) cannot abort the refund DB write — the silent
// refund-loss this fixes. c-free means the goroutine holds no live *gin.Context, so it cannot
// race gin's sync.Pool recycle of c. The timeout bound means a stuck billing DB cannot leak
// the goroutine or extend graceful drain past the deadline. Every request-scoped value the
// refund needs is captured up front via the snapshot.
func scheduleConservativeRefund(c *gin.Context, preConsumedQuota int64, tokenID int, reason string) {
	if c == nil {
		return
	}
	// Mark reconciled SYNCHRONOUSLY, before spawning (see the doc note above).
	markBillingReconciled(c)
	if preConsumedQuota <= 0 {
		return
	}

	// Snapshot every request-scoped value on the REQUEST goroutine, including the
	// UpstreamRequestPossiblyForwarded marker (via skipRefund) — the most important
	// no-underbilling decision must not be read off a recycled c inside the goroutine.
	snap := newConservativeRefundSnapshot(c, preConsumedQuota, tokenID, reason)
	// Snapshot the test gate on the request goroutine so the spawned goroutine
	// never reads the package variable concurrently with a test mutating it.
	gate := refundGoroutineReleaseForTest
	observe := refundObservedForTest
	goDetachedBillingWork(relayctx.Detach(c), "returnPreConsumedQuota", func(cctx context.Context) {
		if gate != nil {
			<-gate
		}
		if observe != nil {
			observe(cctx.Err(), snap)
			return
		}
		_ = snap.refund(cctx)
	})
}

// goRollbackPreConsumed refunds the pre-consumed quota of a failed audio/video
// request in a lifecycle-managed critical goroutine. It is shared by the audio and
// video rollback paths, which are otherwise identical.
//
// It runs from a defer that executes after the handler returns, so it goes through
// goDetachedBillingWork on a detached, non-cancelled, c-free context (relayctx.Detach)
// bounded by config.BillingTimeoutSec: a refund DB write bound to the cancelled request
// context could be aborted, silently losing the refund; a goroutine holding c could race
// gin's sync.Pool recycle; and the timeout keeps a stuck DB from extending graceful drain.
//
// tokenID/quotaToReturn are value params captured on the request goroutine. gate and
// observeCtxErr are test seams (always nil in production) so audio and video can keep
// their own deterministic reproduce-first tests while sharing this body. They are
// snapshotted by the caller on the request goroutine and passed in by value.
func goRollbackPreConsumed(
	c *gin.Context,
	taskName string,
	tokenID int,
	quotaToReturn int64,
	gate chan struct{},
	observeCtxErr func(error),
) {
	goDetachedBillingWork(relayctx.Detach(c), taskName, func(ctx context.Context) {
		if gate != nil {
			<-gate
		}
		if observeCtxErr != nil {
			observeCtxErr(ctx.Err())
		}
		if err := model.PostConsumeTokenQuota(ctx, tokenID, -quotaToReturn); err != nil {
			gmw.GetLogger(ctx).Error("error rolling back pre-consumed quota", zap.Error(err))
		}
	})
}

// markPreConsumed records the pre-consumed quota amount in the gin context
// for the billing audit safety net.
func markPreConsumed(c *gin.Context, amount int64) {
	c.Set(ctxkey.PreConsumedQuotaAmount, amount)
}

// markBillingReconciled marks that post-billing or refund has been completed,
// clearing the audit safety net flag.
func markBillingReconciled(c *gin.Context) {
	c.Set(ctxkey.BillingReconciled, true)
}

// billingAuditSafetyNet should be deferred at the start of each relay handler
// (after pre-consume). It detects cases where pre-consumed quota was never
// reconciled (post-billed or refunded) and logs a CRITICAL warning for audit.
// If the request was NOT forwarded upstream, it also attempts to refund the quota.
func billingAuditSafetyNet(c *gin.Context) {
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	if reconciled != nil {
		if r, ok := reconciled.(bool); ok && r {
			return
		}
	}

	preConsumedAny, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	if !exists {
		return
	}
	preConsumed, ok := preConsumedAny.(int64)
	if !ok || preConsumed <= 0 {
		return
	}

	lg := gmw.GetLogger(c)
	userId := c.GetInt(ctxkey.Id)
	tokenId := c.GetInt(ctxkey.TokenId)
	requestId := c.GetString(ctxkey.RequestId)
	channelId := c.GetInt(ctxkey.ChannelId)

	lg.Error("CRITICAL BILLING AUDIT: pre-consumed quota was not reconciled (no post-billing or refund)",
		zap.Int64("pre_consumed_quota", preConsumed),
		zap.Int("user_id", userId),
		zap.Int("token_id", tokenId),
		zap.Int("channel_id", channelId),
		zap.String("request_id", requestId),
	)

	// Attempt emergency refund if the request was NOT forwarded upstream
	if !shouldSkipPreConsumedRefund(c) {
		lg.Warn("billing audit safety net: attempting emergency refund of unreconciled pre-consumed quota",
			zap.Int64("pre_consumed_quota", preConsumed),
			zap.Int("user_id", userId),
			zap.String("request_id", requestId),
		)
		goDetachedBillingWork(relayctx.Detach(c), "billingAuditRefund", func(ctx context.Context) {
			billing.ReturnPreConsumedQuota(ctx, preConsumed, tokenId)
		})
	} else {
		lg.Error("CRITICAL BILLING AUDIT: cannot refund - request was possibly forwarded upstream, manual reconciliation required",
			zap.Int64("pre_consumed_quota", preConsumed),
			zap.Int("user_id", userId),
			zap.String("request_id", requestId),
		)
	}
}

// recordProvisionalLog writes a provisional consume log entry at pre-consume time.
// This ensures every quota deduction has an audit trail in the logs table,
// even if post-billing never runs. Returns the log ID for later reconciliation.
func recordProvisionalLog(c *gin.Context, meta *metalib.Meta, modelName string, estimatedQuota int64) int {
	if estimatedQuota <= 0 || meta == nil {
		return 0
	}

	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceIDFromContext(gmw.Ctx(c))

	logEntry := &model.Log{
		UserId:    meta.UserId,
		ChannelId: meta.ChannelId,
		ModelName: modelName,
		TokenName: meta.TokenName,
		IsStream:  meta.IsStream,
		RequestId: requestId,
		TraceId:   traceId,
	}

	return model.RecordProvisionalConsumeLog(gmw.Ctx(c), logEntry, estimatedQuota)
}
