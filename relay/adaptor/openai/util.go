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

// NormalizeDataLine normalizes SSE data lines
// This function delegates to the shared implementation for consistency
func NormalizeDataLine(data string) string {
	return openai_compatible.NormalizeDataLine(data)
}
