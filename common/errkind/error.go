package errkind

import (
	"errors"
	"fmt"
	"net/http"
)

// kinded attaches a Kind to an error WITHOUT altering its message.
//
// Error() delegates verbatim to the wrapped error. This is a hard requirement,
// not a style choice: error texts in this repo are returned to API clients by
// openai.ErrorWrapper and by the /api/* JSON body, are pattern-matched by the
// retry and channel-health classifiers, and are asserted literally by tests.
//
// kinded is NOT a substitute for github.com/Laisky/errors/v2 wrapping: it adds
// no stack. Always mark an error that already carries one.
type kinded struct {
	err  error
	kind Kind
}

// Error returns the wrapped error's message unchanged.
func (k *kinded) Error() string { return k.err.Error() }

// Unwrap supports errors.Is and errors.As.
func (k *kinded) Unwrap() error { return k.err }

// Cause supports the github.com/Laisky/errors/v2 (pkg/errors-style) chain.
func (k *kinded) Cause() error { return k.err }

// Format delegates to the wrapped error so that %+v keeps printing the original
// stack trace captured by errors.Wrap / errors.WithStack.
func (k *kinded) Format(s fmt.State, verb rune) {
	if f, ok := k.err.(fmt.Formatter); ok {
		f.Format(s, verb)
		return
	}
	switch verb {
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", k.err.Error())
	default:
		_, _ = fmt.Fprint(s, k.err.Error())
	}
}

// Kind returns the kind recorded at this level.
func (k *kinded) Kind() Kind { return k.kind }

// Mark records who is at fault for err. It is a no-op for a nil error or for
// Unknown, and it never changes err.Error().
//
// Mark where the fault is KNOWN — typically where the error is constructed, with
// the entity in hand — not at the log site, which by definition has to guess.
//
// Parameters:
//   - err: the error to classify; nil returns nil.
//   - kind: the fault attribution; Unknown returns err unchanged.
//
// Return values:
//   - error: err itself when nothing was recorded, otherwise a transparent
//     wrapper whose message, stack formatting and Is/As behaviour are unchanged.
func Mark(err error, kind Kind) error {
	if err == nil || kind == Unknown {
		return err
	}
	return &kinded{err: err, kind: kind}
}

// Of returns the outermost kind recorded on the error chain, or Unknown.
//
// Outermost wins: a caller that has more context about the failure than the
// function it called is entitled to reclassify it.
//
// Parameters:
//   - err: the error to inspect; nil returns Unknown.
//
// Return values:
//   - Kind: the recorded fault attribution.
func Of(err error) Kind {
	var k *kinded
	if errors.As(err, &k) {
		return k.kind
	}
	return Unknown
}

// IsClient reports whether err was marked as client-attributable.
//
// Parameters:
//   - err: the error to inspect.
//
// Return values:
//   - bool: true only when a client-attributed kind was recorded.
func IsClient(err error) bool { return Of(err).IsClient() }

// The helpers below are the ergonomic form used at construction sites:
//
//	return errkind.Quota(errors.Errorf("insufficient user quota: ..."))

// InvalidRequestErr marks err as malformed or invalid client input.
func InvalidRequestErr(err error) error { return Mark(err, InvalidRequest) }

// UnauthorizedErr marks err as a missing or unrecognised credential.
func UnauthorizedErr(err error) error { return Mark(err, Unauthorized) }

// ForbiddenErr marks err as a permission denial for a valid credential.
func ForbiddenErr(err error) error { return Mark(err, Forbidden) }

// NotFoundErr marks err as a reference to a non-existent entity.
func NotFoundErr(err error) error { return Mark(err, NotFound) }

// Quota marks err as the user or token having run out of funds.
func Quota(err error) error { return Mark(err, InsufficientQuota) }

// RateLimitedErr marks err as the client exceeding a rate limit.
func RateLimitedErr(err error) error { return Mark(err, RateLimited) }

// ConflictErr marks err as a lost race or a uniqueness violation.
func ConflictErr(err error) error { return Mark(err, Conflict) }

// ConfigErr marks err as an operator misconfiguration.
func ConfigErr(err error) error { return Mark(err, Config) }

// UpstreamErr marks err as a third-party provider failure.
func UpstreamErr(err error) error { return Mark(err, Upstream) }

// ServerErr marks err as a genuine server-side fault. Use it to force ERROR
// level where the transport would otherwise imply a client fault — the database
// outage that middleware reports as HTTP 401 is the motivating case.
func ServerErr(err error) error { return Mark(err, Server) }

// LogAsWarn decides the log level for an error being reported to a client.
//
// The recorded kind wins in BOTH directions: a client-attributed error is WARN
// even when the transport says 200 or 500, and a Server-marked error is ERROR
// even when the transport says 401. Only unclassified errors fall back to the
// historical status-derived rule, so adoption is incremental and no unmarked
// call site changes behaviour.
//
// Parameters:
//   - status: the HTTP status being returned to the client.
//   - err: the error being reported.
//
// Return values:
//   - bool: true to log at WARN (without a stack), false to log at ERROR.
func LogAsWarn(status int, err error) bool {
	if kind := Of(err); kind != Unknown {
		return kind.IsClient()
	}
	return status >= http.StatusBadRequest && status < http.StatusInternalServerError
}
