package anthropic

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// admissionRejectionError is proof created only after decoding a complete, receipt-free
// Claude JSON admission error. Neither a client field nor an error message can create it.
type admissionRejectionError struct{}

// Error identifies the verified admission rejection without exposing provider payloads.
func (*admissionRejectionError) Error() string { return "Claude rejected the request before inference" }

// IsAdmissionRejection reports whether failure carries the private admission proof.
// It never classifies arbitrary HTTP statuses, text, stream errors, or missing receipts as free.
func IsAdmissionRejection(failure *model.ErrorWithStatusCode) bool {
	if failure == nil {
		return false
	}
	var rejection *admissionRejectionError
	return errors.As(failure.RawError, &rejection)
}

// claudeAdmissionStatus classifies a complete JSON admission error without usage.
// A status-rewriting intermediary may return HTTP 200. Conflicting statuses, any
// usage object (including future fields), and post-admission failures remain uncertain.
// Source: https://platform.claude.com/docs/en/api/errors (reviewed 2026-09-29).
func claudeAdmissionStatus(httpStatus int, envelopeType, errorType string, usage json.RawMessage) int {
	if envelopeType != "error" || (len(usage) > 0 && !bytes.Equal(bytes.TrimSpace(usage), []byte("null"))) {
		return 0
	}
	var status int
	switch errorType {
	case "invalid_request_error":
		status = http.StatusBadRequest
	case "authentication_error":
		status = http.StatusUnauthorized
	case "billing_error":
		status = http.StatusPaymentRequired
	case "permission_error":
		status = http.StatusForbidden
	case "not_found_error":
		status = http.StatusNotFound
	case "request_too_large":
		status = http.StatusRequestEntityTooLarge
	case "rate_limit_error":
		status = http.StatusTooManyRequests
	default:
		return 0
	}
	if httpStatus != http.StatusOK && httpStatus != status {
		return 0
	}
	return status
}
