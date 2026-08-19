package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// billingIdentity carries the request-scoped identifiers a post-billing goroutine
// needs to write its consume log and reconcile its provisional entry. It is
// snapshotted on the request goroutine (see detachForBilling) so the goroutine never
// reads these off a *gin.Context that gin recycles via sync.Pool once the handler
// returns.
type billingIdentity struct {
	requestID        string
	provisionalLogID int
	traceID          string
	toolSummary      *model.ToolUsageSummary
}

type billingIdentityKey struct{}

// detachForBilling returns a detached, c-free context (relayctx.Detach) that also
// carries a snapshot of the request's billing identifiers. Post-billing goroutines
// must be spawned with this context instead of gmw.BackgroundCtx(c): the returned
// context is non-cancelled (so a client disconnect cannot abort the billing DB write)
// and holds no live *gin.Context (so postConsume* cannot race gin's recycle of c).
//
// It must be called on the request goroutine, before the spawn.
func detachForBilling(c *gin.Context) context.Context {
	ctx := relayctx.Detach(c)
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, billingIdentityKey{}, billingIdentityFromGinContext(c))
}

// billingIdentityFromGinContext resolves the billing identifiers directly off the request
// *gin.Context. It MUST run on the request goroutine — it reads c.Keys, and gin recycles c
// via sync.Pool the instant the handler returns. It is the explicit SYNCHRONOUS resolver:
// detachForBilling uses it to take the spawn-time snapshot, and the synchronous fallback in
// billingIdentityFromContext routes through it too.
func billingIdentityFromGinContext(c *gin.Context) billingIdentity {
	var id billingIdentity
	if c == nil {
		return id
	}
	id.requestID = c.GetString(ctxkey.RequestId)
	id.provisionalLogID = c.GetInt(ctxkey.ProvisionalLogId)
	id.traceID = tracing.GetTraceID(c)
	if raw, ok := c.Get(ctxkey.ToolInvocationSummary); ok {
		if summary, ok := raw.(*model.ToolUsageSummary); ok {
			id.toolSummary = summary
		}
	}
	return id
}

// billingIdentityFromContext resolves the billing identifiers for a post-billing path.
//
// INVARIANT: every ASYNCHRONOUS post-billing goroutine MUST be spawned with a
// detachForBilling context, so this resolves at the first branch — from the snapshot taken
// on the request goroutine, never off a *gin.Context that gin has recycled. The audit in
// docs/proposals/20260608_relay-billing-async-sync-race-fixes.md confirms no async caller
// reaches the fallback below.
//
// The embedded-gin fallback is a SYNCHRONOUS-ONLY escape hatch: a caller that still passes
// a context embedding the live *gin.Context (gmw.Ctx/BackgroundCtx) and carries no snapshot.
// Reading c there is safe only because c has not been recycled (the caller is on the request
// goroutine). A relayctx.Detach/detachForBilling context never reaches this branch:
// gmw.GetGinCtxFromStdCtx returns (nil, false) for it, so a detached context can never
// silently degrade into reading a recycled c — it lands on the last branch instead.
func billingIdentityFromContext(ctx context.Context) billingIdentity {
	if id, ok := ctx.Value(billingIdentityKey{}).(billingIdentity); ok {
		return id
	}

	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		return billingIdentityFromGinContext(ginCtx)
	}
	return billingIdentity{traceID: tracing.GetTraceIDFromContext(ctx)}
}
