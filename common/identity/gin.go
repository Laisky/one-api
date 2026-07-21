package identity

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
)

// FromGin reads the identity already published on the gin context by
// middleware/auth.go (user + token) and middleware/distributor.go (channel).
// It performs no database or Redis access: every value is a plain gin map read.
//
// Parameters:
//   - c: the gin context; nil yields the zero Set.
//
// Return values:
//   - Set: whatever identity is currently known for this request.
func FromGin(c *gin.Context) Set {
	if c == nil {
		return Set{}
	}
	return Set{
		User: NewUserRef(c.GetInt(ctxkey.Id),
			c.GetString(ctxkey.UserUUID), c.GetString(ctxkey.Username)),
		Token: NewTokenRef(c.GetInt(ctxkey.TokenId),
			c.GetString(ctxkey.TokenUUID), c.GetString(ctxkey.TokenName)),
		Channel: NewChannelRef(c.GetInt(ctxkey.ChannelId),
			c.GetString(ctxkey.ChannelUUID), c.GetString(ctxkey.ChannelName)),
	}
}

// Current returns the identity most recently bound onto c, refreshed from the raw
// gin keys so that it stays correct even when Bind has not run yet (an abort on an
// unauthenticated route) or when a later middleware overwrote a ctxkey without
// re-binding. Use it from an error funnel.
//
// Parameters:
//   - c: the gin context; nil yields the zero Set.
//
// Return values:
//   - Set: the request's current identity.
func Current(c *gin.Context) Set {
	if c == nil {
		return Set{}
	}
	if v, ok := c.Get(ctxkey.Identity); ok {
		if s, ok := v.(Set); ok {
			return s.Merge(FromGin(c))
		}
	}
	return FromGin(c)
}

// currentFromGinContext resolves the identity when ctx is a *gin.Context.
//
// It exists so that FromContext (in the gin-free identity.go) can special case
// gin without importing gin there.
//
// The second return value is true even for a TYPED-NIL *gin.Context, so that the
// caller never falls through to ctx.Value: that would dispatch to
// (*gin.Context).Value on a nil receiver and panic.
//
// Parameters:
//   - ctx: any context.
//
// Return values:
//   - Set: identity read off the gin context.
//   - bool: whether ctx was a *gin.Context (nil or not).
func currentFromGinContext(ctx context.Context) (Set, bool) {
	gctx, ok := ctx.(*gin.Context)
	if !ok {
		return Set{}, false
	}
	return Current(gctx), true
}

// BindBase captures the pristine request logger (the one installed by
// gmw.NewLoggerMiddleware, carrying url/remote/trace_id) and stores it under
// ctxkey.BaseLogger after applying extra. Every later Bind rebuilds from this
// base, so re-binding on a relay retry REPLACES the identity fields instead of
// appending a second channel_id.
//
// MUST be called on the request goroutine only: it mutates c.Keys and c.Request.
//
// Parameters:
//   - c: the gin context; nil is a no-op.
//   - extra: request-level fields to bake into the base logger (e.g. request_id).
func BindBase(c *gin.Context, extra ...zap.Field) {
	if c == nil {
		return
	}
	base := gmw.GetLogger(c)
	if len(extra) > 0 {
		base = base.With(extra...)
	}
	c.Set(ctxkey.BaseLogger, base)
	apply(c, base, Current(c))
}

// Bind merges set on top of the identity already known for this request, then
// attaches the result in two places at once:
//
//  1. the request-scoped logger (gmw.SetLogger on the gin key), so every later
//     gmw.GetLogger(c).X(...) call emits the identity with no edit at the call
//     site — including inside relayctx.Detach'd goroutines, which snapshot that
//     logger by value;
//  2. the request context value chain, so code that receives only a
//     context.Context (model/, relay/billing) can recover it via FromContext.
//
// Bind is idempotent and safe to call repeatedly (auth, then channel selection,
// then every retry): it always rebuilds from the pristine base logger.
//
// MUST be called on the request goroutine only: it mutates c.Keys and c.Request.
//
// Parameters:
//   - c: the gin context; nil is a no-op.
//   - set: newly learned identity; zero sub-refs are ignored.
func Bind(c *gin.Context, set Set) {
	if c == nil {
		return
	}
	base, _ := c.Value(ctxkey.BaseLogger).(glog.Logger)
	if base == nil {
		base = gmw.GetLogger(c)
		c.Set(ctxkey.BaseLogger, base)
	}
	apply(c, base, Current(c).Merge(set))
}

// BindFromGin re-binds using only what is already published on the gin keys. Call
// it right after a c.Set(ctxkey.Channel*/ctxkey.Token*/...) block.
//
// Parameters:
//   - c: the gin context; nil is a no-op.
func BindFromGin(c *gin.Context) { Bind(c, Set{}) }

// boundSet returns the identity that was last bound onto c's logger, without
// refreshing it from the gin keys.
func boundSet(c *gin.Context) Set {
	if v, ok := c.Get(ctxkey.Identity); ok {
		if s, ok := v.(Set); ok {
			return s
		}
	}
	return Set{}
}

// sameField reports whether two zap fields are identical for the field shapes
// this package emits (zap.Int and zap.String).
func sameField(a, b zap.Field) bool {
	return a.Key == b.Key && a.Type == b.Type &&
		a.Integer == b.Integer && a.String == b.String
}

// ExtraFields returns the identity fields that c's request-scoped logger does
// NOT already carry, namely:
//
//  1. identity published on the gin context after the last Bind (a handler that
//     resolved a user without re-binding), and
//  2. identity tagged onto err by identity.Tag, deep in the call stack where the
//     entity struct was in hand.
//
// Fields identical to already-bound ones are dropped, so an error funnel can call
// this unconditionally without printing user_id twice. A field that shares a key
// but carries a DIFFERENT value is kept: it means the log line is about another
// entity than the request's own, which is information, not noise.
//
// Parameters:
//   - c: the gin context; nil falls back to the error's own fields.
//   - err: the error being reported; nil contributes nothing.
//
// Return values:
//   - []zap.Field: fields to append to the log call.
func ExtraFields(c *gin.Context, err error) []zap.Field {
	if c == nil {
		return Fields(err)
	}

	seen := boundSet(c).Zap()
	var out []zap.Field
	add := func(candidates []zap.Field) {
		for _, f := range candidates {
			dup := false
			for _, known := range seen {
				if sameField(f, known) {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			seen = append(seen, f)
			out = append(out, f)
		}
	}

	add(FromGin(c).Zap())
	add(Fields(err))

	return out
}

// apply installs merged onto the gin logger key and the request context.
func apply(c *gin.Context, base glog.Logger, merged Set) {
	c.Set(ctxkey.Identity, merged)

	// gmw.SetLogger writes the gin key AND returns a std context derived from
	// c.Request.Context(); it does not assign c.Request itself, so we do.
	stdCtx := gmw.SetLogger(c, base.With(merged.Zap()...))
	stdCtx = NewContext(stdCtx, merged)
	if c.Request != nil {
		c.Request = c.Request.WithContext(stdCtx)
	}
}
