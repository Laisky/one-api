package ali

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// aliImageErrorCode is the client-visible code used when a DashScope refusal
// carries no code of its own. It matches the code earlier releases used for
// every task-creation failure.
const aliImageErrorCode = "ali_async_task_failed"

// aliErrorEnvelope is the DashScope error body: {"code","message","request_id"}.
type aliErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// aliTaskPollStatusError carries the provider-marked relay error built from a
// non-2xx DashScope task poll answer through asyncTaskWait's error return, so
// ImageHandler can report the poll's status instead of a local wait failure.
type aliTaskPollStatusError struct {
	failure *model.ErrorWithStatusCode
}

// Error describes the poll refusal by its HTTP status only; the provider's
// message stays in the carried relay error and is never logged from here.
//
// Returns:
//   - string: a description such as "DashScope task poll returned HTTP 429".
func (e *aliTaskPollStatusError) Error() string {
	return fmt.Sprintf("DashScope task poll returned HTTP %d", e.failure.StatusCode)
}

// aliImageStatusError converts a non-2xx DashScope task-creation or task-poll
// answer into a provider-marked relay error that keeps the provider's HTTP
// status. Only the parsed DashScope message, code and request id reach the
// client; an unreadable, oversized, non-JSON or message-less body is described
// by the shared relay/model generic messages and never echoed, because it is
// untrusted provider data. It consumes and closes resp.Body.
//
// Parameters:
//   - resp: the non-2xx DashScope creation or poll response.
//
// Returns:
//   - *model.ErrorWithStatusCode: the error, with FromUpstream set and StatusCode
//     equal to resp.StatusCode.
func aliImageStatusError(resp *http.Response) *model.ErrorWithStatusCode {
	body, readErr := model.ReadBoundedUpstreamBody(resp.Body)
	var bodyErr error
	if readErr != nil {
		bodyErr = errors.Wrapf(readErr, "read DashScope HTTP %d error body", resp.StatusCode)
	}
	if resp.Body != nil {
		if closeErr := resp.Body.Close(); closeErr != nil && bodyErr == nil {
			bodyErr = errors.Wrapf(closeErr, "close DashScope HTTP %d error body", resp.StatusCode)
		}
	}

	code := aliImageErrorCode
	requestID := ""
	message := model.UnreadableUpstreamErrorMessage(resp.StatusCode)
	if readErr == nil {
		message = model.UpstreamErrorBodyMessage(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		var envelope aliErrorEnvelope
		if len(body) <= model.MaxUpstreamErrorBodyBytes && json.Unmarshal(body, &envelope) == nil {
			if text := strings.TrimSpace(envelope.Message); text != "" {
				message = text
			}
			if text := strings.TrimSpace(envelope.Code); text != "" {
				code = text
			}
			requestID = strings.TrimSpace(envelope.RequestID)
		}
	}

	result := model.NewUpstreamError(resp.StatusCode, message, model.ErrorTypeAli, code,
		model.WithUpstreamErrorParam(requestID))
	if bodyErr != nil {
		// Keep the transport failure for diagnostics; the provider status still
		// identifies the refusal, so the error stays provider-marked.
		result.RawError = bodyErr
	}
	return result
}
