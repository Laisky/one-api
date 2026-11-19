package openai

import (
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

func ErrorWrapper(err error, code string, statusCode int) *model.ErrorWithStatusCode {
	// Avoid using global logger here; callers should log with request-scoped logger.
	Error := model.Error{
		Message:  err.Error(),
		Type:     model.ErrorTypeOneAPI,
		Code:     code,
		RawError: err,
	}
	return &model.ErrorWithStatusCode{
		Error:      Error,
		StatusCode: statusCode,
	}
}

// UpstreamErrorWrapper wraps a failure the provider reported with its own HTTP
// status. It delegates to openai_compatible.UpstreamErrorWrapper, which documents
// the provenance contract; use ErrorWrapper only for failures one-api produced.
//
// Parameters:
//   - err: the failure description; it becomes Message and RawError.
//   - code: the stable error code exposed to clients.
//   - statusCode: the HTTP status to report, normally the provider's status.
//
// Returns:
//   - *model.ErrorWithStatusCode: the wrapped error, marked as provider-caused.
func UpstreamErrorWrapper(err error, code string, statusCode int) *model.ErrorWithStatusCode {
	return openai_compatible.UpstreamErrorWrapper(err, code, statusCode)
}

// NormalizeDataLine normalizes SSE data lines
// This function delegates to the shared implementation for consistency
func NormalizeDataLine(data string) string {
	return openai_compatible.NormalizeDataLine(data)
}
