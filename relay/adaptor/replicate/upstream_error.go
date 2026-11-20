package replicate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// replicateStatusCode is the client-visible code of a Replicate response whose
// HTTP status is not the expected one; earlier releases used the same code.
const replicateStatusCode = "bad_status_code"

// replicateProblem is the RFC 9457 problem-details body Replicate uses for
// errors, for example {"title":"Too many requests","detail":"...","status":429}.
type replicateProblem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// replicateHTTPStatusError reports that a follow-up Replicate request (task
// poll, output stream or image download) answered with an unexpected HTTP
// status. Its text carries only the parsed provider message, never the raw
// body, and its presence marks the failure as provider-caused.
type replicateHTTPStatusError struct {
	action  string
	message string
	// cause is a failure to read the error body, kept for diagnostics only.
	cause error
}

// Error returns the client-safe description of the failed follow-up request.
//
// Returns:
//   - string: "<action>: <client-safe provider message>".
func (e *replicateHTTPStatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.action, e.message)
}

// Unwrap exposes the body read failure, if any, to errors.Is and errors.As.
//
// Returns:
//   - error: the read failure, or nil.
func (e *replicateHTTPStatusError) Unwrap() error {
	return e.cause
}

// replicateErrorMessage reads a bounded non-success Replicate response body and
// returns a client-safe description of it. Replicate reports errors as RFC 9457
// problem details, so a string "detail" (or, failing that, "title") member is
// passed through, as is any message model.UpstreamErrorBodyText recognizes; the
// result reads "upstream returned HTTP N: <message>". Any other body (empty,
// unreadable, oversized, non-JSON or message-less) is described generically by
// the shared relay/model helpers and never echoed, because it is untrusted
// provider data. The caller owns closing resp.Body.
//
// Parameters:
//   - resp: the non-success Replicate response.
//
// Returns:
//   - string: the client-safe message; never empty.
//   - error: a wrapped read failure, or nil.
func replicateErrorMessage(resp *http.Response) (string, error) {
	body, err := model.ReadBoundedUpstreamBody(resp.Body)
	if err != nil {
		return model.UnreadableUpstreamErrorMessage(resp.StatusCode), errors.Wrapf(err, "read replicate HTTP %d error body", resp.StatusCode)
	}
	text := replicateProblemText(body)
	if text == "" {
		text = model.UpstreamErrorBodyText(body)
	}
	if text != "" {
		return fmt.Sprintf("upstream returned HTTP %d: %s", resp.StatusCode, text), nil
	}
	return model.UpstreamErrorBodyMessage(resp.StatusCode, resp.Header.Get("Content-Type"), body), nil
}

// replicateProblemText returns the human-readable text of an RFC 9457
// problem-details body: its string "detail", else its string "title".
//
// Parameters:
//   - body: the bounded error body; an over-limit body is not parsed.
//
// Returns:
//   - string: the trimmed text, or "" when the body has neither member as a string.
func replicateProblemText(body []byte) string {
	if len(body) > model.MaxUpstreamErrorBodyBytes {
		return ""
	}
	var problem replicateProblem
	if json.Unmarshal(body, &problem) != nil {
		return ""
	}
	for _, candidate := range []string{problem.Detail, problem.Title} {
		if text := strings.TrimSpace(candidate); text != "" {
			return text
		}
	}
	return ""
}

// replicateStatusError builds the "bad_status_code" relay error for a
// prediction-creation response whose status is not 201 Created.
//
// A non-2xx status is the provider's refusal (for example 429 rate limiting):
// the error keeps that status and is marked FromUpstream, so the relay applies
// the provider failure policy (cooldown, failover, channel health) and the
// image helper refunds a 4xx. An unexpected 2xx keeps the earlier local 500
// classification, because the provider accepted the request. Either way the
// message is client-safe (see replicateErrorMessage).
//
// Parameters:
//   - resp: the unexpected Replicate response; the caller owns closing its body.
//
// Returns:
//   - *model.ErrorWithStatusCode: the relay error.
func replicateStatusError(resp *http.Response) *model.ErrorWithStatusCode {
	message, readErr := replicateErrorMessage(resp)
	cause := errors.New(message)
	if readErr != nil {
		cause = readErr
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		result := openai.ErrorWrapper(cause, replicateStatusCode, http.StatusInternalServerError)
		result.Message = message
		return result
	}
	result := openai.UpstreamErrorWrapper(cause, replicateStatusCode, resp.StatusCode)
	result.Message = message
	return result
}

// replicateStatusFailure returns the error for an unexpected status on a
// follow-up Replicate request. Its text reaches the client through the caller's
// error wrapper, so it never contains the raw response body.
//
// Parameters:
//   - resp: the unexpected Replicate response; the caller owns closing its body.
//   - action: a short description of the request, such as "get task".
//
// Returns:
//   - error: a *replicateHTTPStatusError, wrapped with a stack trace.
func replicateStatusFailure(resp *http.Response, action string) error {
	// A read failure only loses the provider's detail: the status alone still
	// identifies the provider failure, so the generic message is used and the
	// read failure is kept as the cause.
	message, readErr := replicateErrorMessage(resp)
	return errors.WithStack(&replicateHTTPStatusError{action: action, message: message, cause: readErr})
}

// replicateTaskError wraps a failure of the prediction-polling loop for the
// client. A follow-up request the provider answered with an unexpected HTTP
// status is provider-caused and marked FromUpstream; every other failure
// (local transport, decode, conversion or a failed prediction) keeps the local
// one-api classification it always had. The status reported to the client stays
// 500 in both cases, because the prediction was already accepted and settled.
//
// Parameters:
//   - err: the loop failure.
//   - code: the client-visible code, such as "image_task_failed".
//
// Returns:
//   - *model.ErrorWithStatusCode: the relay error.
func replicateTaskError(err error, code string) *model.ErrorWithStatusCode {
	var statusErr *replicateHTTPStatusError
	if errors.As(err, &statusErr) {
		return openai.UpstreamErrorWrapper(err, code, http.StatusInternalServerError)
	}
	return openai.ErrorWrapper(err, code, http.StatusInternalServerError)
}
