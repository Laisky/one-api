package model

import (
	"net/http"

	"github.com/Laisky/errors/v2"
)

// Delivery failure codes name a failed write to the downstream client after the
// provider response was accepted. Such a failure is local to the gateway and
// the client connection: it is not evidence about the provider or the channel.
const (
	// DeliveryCodeStream reports a failed write while relaying a stream.
	DeliveryCodeStream = "write_stream_failed"
	// DeliveryCodeResponse reports a failed write of a complete response.
	DeliveryCodeResponse = "write_response_failed"
	// DeliveryCodeResponseBody reports a failed write of a buffered response body.
	DeliveryCodeResponseBody = "write_response_body_failed"
)

// deliveryFailureCodes lists every one-api code that denotes a downstream
// delivery failure, including the adaptor-specific codes already in use.
var deliveryFailureCodes = map[string]struct{}{
	DeliveryCodeStream:                     {},
	DeliveryCodeResponse:                   {},
	DeliveryCodeResponseBody:               {},
	"write_embedding_response":             {},
	"write_embedding_response_body_failed": {},
	"write_mcp_response_failed":            {},
	"write_converted_response_failed":      {},
	"write_tool_search_response_failed":    {},
}

// IsDeliveryFailure reports whether e is a downstream delivery failure generated
// by one-api. Only one-api 5xx errors qualify, so a provider-supplied error body
// cannot spoof the classification by echoing a delivery code.
//
// Parameters:
//   - e: the normalized relay failure; nil is not a delivery failure.
//
// Returns:
//   - bool: true when the failure is local to the gateway/client connection.
func IsDeliveryFailure(e *ErrorWithStatusCode) bool {
	if e == nil || e.Type != ErrorTypeOneAPI || e.StatusCode < http.StatusInternalServerError {
		return false
	}
	code, ok := e.Code.(string)
	if !ok {
		return false
	}
	_, known := deliveryFailureCodes[code]
	return known
}

// NewDeliveryError reports a failed write to the downstream client. Adaptors use
// it instead of a provider-shaped error so that the relay controller attributes
// the failure to the gateway, never penalizes the channel's health, and keeps
// any measured usage for settlement. The cause chain, including a client
// context cancellation, remains available to errors.Is and errors.As.
//
// Parameters:
//   - err: the writer failure; it is wrapped with a stack.
//   - code: one of the delivery failure codes, such as DeliveryCodeStream.
//
// Returns:
//   - *ErrorWithStatusCode: a one-api 500 error that IsDeliveryFailure accepts.
func NewDeliveryError(err error, code string) *ErrorWithStatusCode {
	if err == nil {
		err = errors.New("downstream delivery failed")
	}
	wrapped := errors.Wrap(err, "deliver response to client")
	return &ErrorWithStatusCode{
		StatusCode: http.StatusInternalServerError,
		Error: Error{
			Message:  wrapped.Error(),
			Type:     ErrorTypeOneAPI,
			Code:     code,
			RawError: wrapped,
		},
	}
}
