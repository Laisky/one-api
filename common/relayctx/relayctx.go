// Package relayctx provides safe primitives for running request-scoped background
// work without retaining the request's *gin.Context.
//
// Why this exists: gin v1.12.0 recycles *gin.Context via sync.Pool the instant
// ServeHTTP returns; the next request reuses the same object and calls c.reset()
// (clearing c.Keys) and c.Set(...). A background goroutine that still holds c and
// calls c.GetInt(...)/c.Set(...)/gmw.GetLogger(c) then races that map, which can
// bill the wrong user/token or crash the process with a concurrent map panic.
//
// gmw.BackgroundCtx(c) detaches request cancellation but still stores c itself via
// context.WithValue(ctx, CtxKeyGin, c), so any code that later resolves the gin
// context out of the std context (gmw.GetGinCtxFromStdCtx) and reads c.Keys is back
// in the same use-after-return hazard. Detach is the c-free alternative: it copies
// the request logger and trace id by value into a fresh context.Background() and
// never references the gin context.
package relayctx

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/graceful"
)

// Detach returns a non-cancellable background context that carries the request's
// logger and trace id BY VALUE but never retains the *gin.Context.
//
// The returned context is safe to hand to a goroutine that outlives the HTTP
// handler: gmw.GetLogger(ctx) resolves the snapshotted logger and reading the
// trace id off ctx works, but gmw.GetGinCtxFromStdCtx(ctx) returns (nil, false) —
// so no code path can dereference the recycled gin context through it.
//
// Unlike gmw.BackgroundCtx, the result is detached from request cancellation
// (rooted at context.Background()), so a DB write that runs after the handler
// returns is not aborted by the request context being cancelled.
func Detach(c *gin.Context) context.Context {
	ctx := context.Background()
	if c == nil {
		return ctx
	}

	// Snapshot the request logger by value. gmw.SetLogger stores it under its own
	// context key, and gmw.GetLogger reads it back from a non-gin context without
	// ever touching c.
	ctx = gmw.SetLogger(ctx, gmw.GetLogger(c))

	// Snapshot the trace id by value under the same key gin-middlewares uses, so
	// downstream logging/tracing still resolves it. gmw.TraceID must run on the gin
	// context (here, on the request goroutine) before we detach.
	if tid, err := gmw.TraceID(c); err == nil {
		ctx = context.WithValue(ctx, gutils.TracingKey, tid.String())
	}

	return ctx
}

// GoRequestScoped runs fn in a lifecycle-managed critical goroutine with a
// detached, c-free context (see Detach). fn receives ONLY the detached context;
// it must not close over *gin.Context — snapshot any request-scoped scalars on the
// request goroutine before calling this and capture them in fn's closure.
func GoRequestScoped(c *gin.Context, name string, fn func(ctx context.Context)) {
	graceful.GoCritical(Detach(c), name, fn)
}
