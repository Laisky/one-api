package anthropic

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// ParseClaudeMessageAdmissionRejection reuses the native admission proof for an
// internal MCP round. Parameters: body is the complete bounded HTTP response,
// httpStatus is its actual status, and closeErr is the original body's Close
// result. The caller must first finish reading without error. Returns: a private
// proof only after clean close, JSON and receipt validation; otherwise nil.
func ParseClaudeMessageAdmissionRejection(body []byte, httpStatus int, closeErr error) *model.ErrorWithStatusCode {
	if closeErr != nil || len(body) > maxClaudeHTTPPayload {
		return nil
	}
	var envelope struct {
		Type  string          `json:"type"`
		Usage json.RawMessage `json:"usage"`
		Error Error           `json:"error"`
	}
	if decodeClaudeJSON(body, &envelope) != nil {
		return nil
	}
	var receipt httpUsageReceipt
	if receipt.apply(envelope.Usage) != nil {
		return nil
	}
	status := claudeAdmissionStatus(httpStatus, envelope.Type, envelope.Error.Type, envelope.Usage)
	if status == 0 {
		return nil
	}
	return &model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{
		Type: model.ErrorType(envelope.Error.Type), Code: envelope.Error.Type,
		Message: envelope.Error.Message, RawError: errors.WithStack(&admissionRejectionError{}),
	}}
}
