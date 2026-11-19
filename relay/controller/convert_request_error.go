package controller

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

var convertRequestBadRequestHints = []string{
	"validation failed",
	"does not support embedding",
	"does not support the v1/messages endpoint",
}

// shouldTreatConvertRequestErrorAsBadRequest determines whether a request-conversion
// failure should be returned as a 400 invalid_request_error instead of a 500.
func shouldTreatConvertRequestErrorAsBadRequest(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, common.ErrAmbiguousJSONKey) || adaptor.IsUnbillableRequestControl(err) {
		return true
	}

	msg := strings.ToLower(err.Error())
	for _, hint := range convertRequestBadRequestHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}

	return false
}

// wrapConvertRequestError wraps conversion failures into a consistent API error shape.
// It maps validation-like errors to 400 and preserves existing 500 behavior otherwise.
func wrapConvertRequestError(err error) *relaymodel.ErrorWithStatusCode {
	if apiErr, ok := unbillableRequestControlError(err); ok {
		return apiErr
	}
	if shouldTreatConvertRequestErrorAsBadRequest(err) {
		return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
	}

	return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
}

// unbillableRequestControlError answers a client control the gateway cannot
// bill as HTTP 400 invalid_request_error whose message is the refusal's own
// text, without internal wrap context. Parameters: err is a preparation or
// admission error. Returns: the client error and true for a marked refusal.
func unbillableRequestControlError(err error) (*relaymodel.ErrorWithStatusCode, bool) {
	message, ok := adaptor.UnbillableRequestControlMessage(err)
	if !ok {
		return nil, false
	}
	apiErr := openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
	apiErr.Error.Message = message
	return apiErr, true
}
