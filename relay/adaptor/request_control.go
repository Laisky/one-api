package adaptor

import "github.com/Laisky/errors/v2"

// ErrUnbillableRequestControl marks a request refused before dispatch because
// a client-supplied control (for example a provider service tier or prompt
// cache lifetime) would make the provider bill a dimension the gateway does
// not meter. It is a client request error (HTTP 400): no upstream was
// contacted, nothing was charged, and the client can resend without the
// control. It is distinct from ErrRequestPricingNotConfigured, which is a
// channel configuration fault another channel may not share.
var ErrUnbillableRequestControl = errors.New("request control has no metered tariff")

// requestControlError carries a refusal's own message while matching
// ErrUnbillableRequestControl through errors.Is.
type requestControlError struct {
	err error
}

// Error returns the refusal's message. It takes no parameters.
func (e *requestControlError) Error() string { return e.err.Error() }

// Unwrap exposes the refusal's error. It takes no parameters.
func (e *requestControlError) Unwrap() error { return e.err }

// Is reports whether target is ErrUnbillableRequestControl. Parameters: target
// is the sentinel being matched. Returns: true for that sentinel.
func (e *requestControlError) Is(target error) bool { return target == ErrUnbillableRequestControl }

// UnbillableRequestControl marks err as a client control the gateway cannot
// bill, so every conversion boundary answers it as HTTP 400 with err's own
// message. Parameters: err is the refusal and may be nil. Returns: the marked
// error, or nil for a nil err.
func UnbillableRequestControl(err error) error {
	if err == nil {
		return nil
	}
	return &requestControlError{err: err}
}

// IsUnbillableRequestControl reports whether err is a client control refusal
// marked by UnbillableRequestControl. Parameters: err may be nil or wrapped.
// Returns: true for a marked refusal.
func IsUnbillableRequestControl(err error) bool {
	return err != nil && errors.Is(err, ErrUnbillableRequestControl)
}

// UnbillableRequestControlMessage returns the refusal's own message, without
// the internal wrap chain callers added on the way up, for client display.
// Parameters: err may be nil or wrapped. Returns: the message and true when err
// carries an UnbillableRequestControl mark.
func UnbillableRequestControlMessage(err error) (string, bool) {
	var marked *requestControlError
	if err == nil || !errors.As(err, &marked) {
		return "", false
	}
	return marked.err.Error(), true
}
