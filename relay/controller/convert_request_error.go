package controller

import (
	"net/http"
	"strings"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
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
	if shouldTreatConvertRequestErrorAsBadRequest(err) {
		return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
	}

	return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
}
