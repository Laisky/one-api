// Package errkind records WHO IS AT FAULT for an error — the client or the
// server — at the point where the error is constructed, so that a log funnel
// several layers up can pick the right level without guessing.
//
// Why this exists: common/helper.RespondError answers every /api/* error with
// HTTP 200 and {"success":false,...}, which is this project's long-standing wire
// convention. The log funnel used to derive WARN-vs-ERROR from that status, so a
// user simply running out of quota was logged at ERROR with a full stack trace —
// the level reserved for genuine server faults that page an on-call engineer.
// The mirror defect is worse: middleware forces HTTP 401 for every token
// validation failure, including a real database outage, so an outage logged at
// WARN and paged nobody.
//
// A status code cannot express fault attribution in either direction, so the
// class is declared where it is known: at construction.
//
// This package deliberately mirrors common/identity's transparent-wrapper
// design. Marking an error NEVER changes err.Error(), because those texts are
// returned to API clients verbatim, are pattern-matched by the retry and
// channel-health classifiers, and are asserted literally by tests.
//
// IMPORT RULE: leaf package. Standard library only.
package errkind

import "net/http"

// Kind classifies the cause of an error.
type Kind uint8

const (
	// Unknown means the error was never classified. It preserves the previous
	// status-derived behaviour, so adopting this package is incremental.
	Unknown Kind = iota
	// InvalidRequest: malformed or semantically invalid client input.
	InvalidRequest
	// Unauthorized: missing or unrecognised credential.
	Unauthorized
	// Forbidden: valid credential, but not permitted to do this.
	Forbidden
	// NotFound: the referenced entity does not exist.
	NotFound
	// InsufficientQuota: the user or token has run out of funds. The condition
	// that motivated this package.
	InsufficientQuota
	// RateLimited: the client is sending too fast.
	RateLimited
	// Conflict: the request lost a race or violated a uniqueness constraint.
	Conflict
	// Config: an operator misconfiguration (e.g. no channel serves the model).
	// Client-attributed for logging: it is not a code fault, and it must not page
	// an on-call engineer, but it does need an operator's attention.
	Config
	// Upstream: a third-party provider failed. Not this server's fault and not
	// the caller's, but it is actionable, so it is treated as server-side.
	Upstream
	// Server: a genuine server-side fault — database failure, marshalling bug,
	// broken invariant. This is what ERROR level is reserved for.
	Server
)

// String returns the snake_case name of the kind, suitable as a log field value
// or a bounded metric label. It never contains user data.
//
// Return values:
//   - string: stable identifier for this kind.
func (k Kind) String() string {
	switch k {
	case InvalidRequest:
		return "invalid_request"
	case Unauthorized:
		return "unauthorized"
	case Forbidden:
		return "forbidden"
	case NotFound:
		return "not_found"
	case InsufficientQuota:
		return "insufficient_quota"
	case RateLimited:
		return "rate_limited"
	case Conflict:
		return "conflict"
	case Config:
		return "config"
	case Upstream:
		return "upstream"
	case Server:
		return "server"
	default:
		return "unknown"
	}
}

// IsClient reports whether the kind is attributable to the caller or to
// configuration rather than to a server fault. Client-attributed errors are
// logged at WARN without a stack trace.
//
// Unknown is NOT client-attributed: an unclassified error must keep the
// conservative behaviour of being treated as a potential server fault.
//
// Return values:
//   - bool: true when the error should be logged at WARN.
func (k Kind) IsClient() bool {
	switch k {
	case InvalidRequest, Unauthorized, Forbidden, NotFound,
		InsufficientQuota, RateLimited, Conflict, Config:
		return true
	default:
		return false
	}
}

// HTTPStatus returns the status code that would represent this kind on a
// status-honest API.
//
// NOTE: it is deliberately NOT used to change any existing response. This
// project answers /api/* with HTTP 200 plus {"success":false,...}; two of the
// three shipped web themes swallow non-2xx responses without re-throwing, and
// the external billing API documents the 200 convention. The accessor exists for
// future opt-in endpoints and for diagnostics.
//
// Return values:
//   - int: the representative HTTP status code.
func (k Kind) HTTPStatus() int {
	switch k {
	case InvalidRequest:
		return http.StatusBadRequest
	case Unauthorized:
		return http.StatusUnauthorized
	case Forbidden, InsufficientQuota:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict:
		return http.StatusConflict
	case RateLimited:
		return http.StatusTooManyRequests
	case Config:
		return http.StatusServiceUnavailable
	case Upstream:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
