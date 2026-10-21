package middleware

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/errkind"
)

// requestLifecycleContextKey stores a private, typed snapshot outside the request
// context, so replacing that context cannot change its transport provenance.
const requestLifecycleContextKey = "one-api.internal.transport-request-context"

// transportRequestContext retains the original HTTP request context for logging.
type transportRequestContext struct {
	context.Context
}

// RequestLifecycle captures the originating HTTP request context once and then
// continues the Gin chain. Register it before middleware derives application
// contexts; no deadline or cancellation behavior is changed.
func RequestLifecycle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(requestLifecycleContextKey); !exists {
			var original context.Context
			if c.Request != nil {
				original = c.Request.Context()
			}
			c.Set(requestLifecycleContextKey, transportRequestContext{original})
		}
		c.Next()
	}
}

// abortLogPlan contains the error kind and severity used only for abort logging.
// It never changes the error or the HTTP response.
type abortLogPlan struct {
	kind    string
	warning bool
}

// classifyAbortLog chooses logging metadata for err at statusCode using c's
// captured transport lifecycle. Explicit fault attribution keeps its existing
// precedence; only pure cancellation with proven transport provenance is WARN.
func classifyAbortLog(c *gin.Context, statusCode int, err error) abortLogPlan {
	budget := 256
	complete, pureCancellation := inspectAbortError(err, 0, &budget)
	if !complete {
		// Cyclic or excessively large error trees cannot prove a harmless outcome.
		return abortLogPlan{kind: errkind.Unknown.String()}
	}

	kind, cancellation, _ := abortErrorKind(err)
	if kind != errkind.Unknown {
		return abortLogPlan{kind: kind.String(), warning: kind.IsClient()}
	}
	if pureCancellation && transportRequestCanceled(c) {
		return abortLogPlan{kind: "request_canceled", warning: true}
	}
	if cancellation {
		// A child cancellation, internal deadline, custom cause, or a joined real
		// fault must remain actionable even when an auth path responds with 401.
		return abortLogPlan{kind: kind.String()}
	}
	return abortLogPlan{kind: kind.String(), warning: shouldLogAsWarning(statusCode, err)}
}

// transportRequestCanceled reports whether c captured an originating request
// context canceled with the ordinary cancellation cause, excluding deadlines and
// application-specific cancellation causes. Missing provenance returns false.
func transportRequestCanceled(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, exists := c.Get(requestLifecycleContextKey)
	if !exists {
		return false
	}
	original, ok := value.(transportRequestContext)
	return ok && original.Context != nil &&
		original.Err() == context.Canceled && context.Cause(original.Context) == context.Canceled
}

// inspectAbortError checks err's standard unwrap tree with bounded depth and
// node count. It returns whether traversal completed and whether every non-nil
// leaf is the context.Canceled sentinel. Mixed joins never qualify as pure
// cancellation, and bounded traversal makes subsequent Is/As lookups safe for
// ordinary immutable error wrappers without following cycles indefinitely.
func inspectAbortError(err error, depth int, budget *int) (complete, pureCancellation bool) {
	if err == nil {
		return true, false
	}
	if depth >= 64 || *budget <= 0 {
		return false, false
	}
	*budget -= 1
	if err == context.Canceled {
		return true, true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		found := false
		pureCancellation = true
		for _, child := range wrapped.Unwrap() {
			if child == nil {
				continue
			}
			found = true
			complete, childPure := inspectAbortError(child, depth+1, budget)
			if !complete {
				return false, false
			}
			pureCancellation = pureCancellation && childPure
		}
		return true, found && pureCancellation
	case interface{ Unwrap() error }:
		child := wrapped.Unwrap()
		if child == nil {
			return true, false
		}
		return inspectAbortError(child, depth+1, budget)
	default:
		return true, false
	}
}

// abortErrorKind resolves explicit attribution after inspectAbortError has
// validated the unwrap tree. A direct mark deliberately overrides its complete
// wrapped error, preserving the existing outermost-mark contract. Independent
// joined branches retain server/upstream faults regardless of branch order.
func abortErrorKind(err error) (kind errkind.Kind, cancellation, unmarkedFault bool) {
	if err == nil {
		return errkind.Unknown, false, false
	}
	cancellation = errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if marked, ok := err.(interface{ Kind() errkind.Kind }); ok {
		if directKind := marked.Kind(); directKind != errkind.Unknown {
			return directKind, cancellation, false
		}
	}
	if err == context.Canceled {
		return errkind.Unknown, true, false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		return abortJoinedErrorKind(wrapped.Unwrap(), cancellation)
	case interface{ Unwrap() error }:
		if child := wrapped.Unwrap(); child != nil {
			kind, childCancellation, unmarkedFault := abortErrorKind(child)
			cancellation = cancellation || childCancellation
			if cancellation && unmarkedFault {
				return errkind.Unknown, true, true
			}
			return kind, cancellation, unmarkedFault
		}
	}
	// Unknown non-cancellation leaves, including deadlines and opaque Is-only
	// cancellation claims, cannot hide behind a canceled sibling in a join.
	return errkind.Unknown, cancellation, true
}

// abortJoinedErrorKind resolves validated independent branches, preferring
// actionable explicit faults. Cancellation mixed with an unmarked real fault
// stays unknown/ERROR; it must not inherit a sibling's client attribution.
func abortJoinedErrorKind(children []error, cancellation bool) (kind errkind.Kind, shaped, unmarkedFault bool) {
	kind = errkind.Unknown
	faultKind := errkind.Unknown
	found := false
	for _, child := range children {
		if child == nil {
			continue
		}
		found = true
		childKind, childCancellation, childFault := abortErrorKind(child)
		cancellation = cancellation || childCancellation
		unmarkedFault = unmarkedFault || childFault
		if kind == errkind.Unknown {
			kind = childKind
		}
		if childKind == errkind.Server || childKind == errkind.Upstream && faultKind != errkind.Server {
			faultKind = childKind
		}
	}
	if faultKind != errkind.Unknown {
		return faultKind, cancellation, false
	}
	if !found {
		unmarkedFault = true
	}
	if cancellation && unmarkedFault {
		return errkind.Unknown, true, true
	}
	return kind, cancellation, unmarkedFault
}
