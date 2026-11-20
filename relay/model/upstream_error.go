package model

import "github.com/Laisky/errors/v2"

// UpstreamErrorOption adjusts an error built by NewUpstreamError before it is
// returned. The options in this package only set provider-supplied fields and
// never change the error's provenance.
type UpstreamErrorOption func(*ErrorWithStatusCode)

// WithUpstreamErrorParam sets the provider-supplied Param of an error built by
// NewUpstreamError. Without it Param is "".
//
// Parameters:
//   - param: the provider's param value, exposed to clients as-is.
//
// Returns:
//   - UpstreamErrorOption: the option that sets Param.
func WithUpstreamErrorParam(param string) UpstreamErrorOption {
	return func(e *ErrorWithStatusCode) {
		e.Param = param
	}
}

// NewUpstreamError is the canonical constructor for an error that an adaptor
// builds in Go from fields of a provider's response (its status, message, error
// type or code). Such an error describes the provider, never this gateway, so it
// is always marked FromUpstream: even a provider-supplied "one_api_error" type
// then reports IsOneAPIError()==false, and the relay applies the provider
// failure policy (ability cooldown, channel health) instead of treating it as a
// one-api internal error. Prefer it over an Error composite literal whose Type is
// derived from provider data; the ast-grep rule
// upstream-error-literal-needs-provenance enforces that in relay/**,
// controller/** and middleware/**.
//
// Parameters:
//   - statusCode: the HTTP status to report to the client, normally the provider's.
//   - message: the client-visible message; RawError is errors.New(message). It
//     must be a parsed provider field or a generic description, never a raw body.
//   - errType: the error type, typically model.ErrorType(<provider field>).
//   - code: the client-visible code (string, number or nil).
//   - opts: optional adjustments such as WithUpstreamErrorParam; nil is ignored.
//
// Returns:
//   - *ErrorWithStatusCode: the error, with FromUpstream set.
func NewUpstreamError(statusCode int, message string, errType ErrorType, code any, opts ...UpstreamErrorOption) *ErrorWithStatusCode {
	result := &ErrorWithStatusCode{
		StatusCode: statusCode,
		Error: Error{
			Message:      message,
			Type:         errType,
			Code:         code,
			FromUpstream: true,
			RawError:     errors.New(message),
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(result)
		}
	}
	return result
}
