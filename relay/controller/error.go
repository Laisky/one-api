package controller

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// RelayErrorHandler parses an upstream error response into the unified error
// model. Only parsed provider fields (message, type, code, param) reach the
// client. A body that cannot be read, is empty, exceeds
// model.MaxUpstreamErrorBodyBytes, is not a JSON object, or carries no
// recognizable message yields a generic message that never contains body bytes:
// provider bodies are untrusted and can carry internal hosts or account
// identifiers (OWASP API8:2023, API10:2023). The read is bounded (OWASP
// API4:2023) and the body is always closed. For request-scoped logging, prefer
// RelayErrorHandlerWithContext.
//
// Parameters:
//   - resp: the upstream response; nil yields a "resp is nil" error.
//
// Returns:
//   - *model.ErrorWithStatusCode: the normalized error, marked FromUpstream when
//     resp is non-nil.
func RelayErrorHandler(resp *http.Response) (ErrorWithStatusCode *model.ErrorWithStatusCode) {
	if resp == nil {
		return &model.ErrorWithStatusCode{
			StatusCode: 500,
			Error: model.Error{
				Message:  "resp is nil",
				Type:     model.ErrorTypeUpstream,
				Code:     "bad_response",
				RawError: errors.New("resp is nil"),
			},
		}
	}
	body, err := readUpstreamErrorBody(resp.Body)
	return relayErrorFromUpstreamBody(resp, body, err)
}

// RelayErrorHandlerWithContext is a context-aware variant of RelayErrorHandler
// that records response metadata (never the body) with the request-scoped logger.
//
// Parameters:
//   - c: the request context that carries the request-scoped logger.
//   - resp: the upstream response; nil yields a "resp is nil" error.
//
// Returns:
//   - *model.ErrorWithStatusCode: the same normalized error RelayErrorHandler returns.
func RelayErrorHandlerWithContext(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
	if resp == nil {
		return RelayErrorHandler(resp)
	}
	body, err := readUpstreamErrorBody(resp.Body)
	logUpstreamResponseFromBytes(gmw.GetLogger(c), resp, body, string(model.ErrorTypeUpstream))
	// The original body is closed; leave the bounded bytes for any later reader.
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return relayErrorFromUpstreamBody(resp, body, err)
}

// readUpstreamErrorBody reads at most model.MaxUpstreamErrorBodyBytes+1 bytes of
// an upstream error body and always closes it, so an over-limit body is
// detected without reading the rest of it.
//
// Parameters:
//   - body: the upstream response body; nil is treated as empty.
//
// Returns:
//   - []byte: the bytes read, at most model.MaxUpstreamErrorBodyBytes+1.
//   - error: a wrapped read and/or close failure, or nil.
func readUpstreamErrorBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, readErr := model.ReadBoundedUpstreamBody(body)
	closeErr := body.Close()
	switch {
	case readErr != nil && closeErr != nil:
		return data, errors.Join(errors.Wrap(readErr, "read upstream error body"), errors.Wrap(closeErr, "close upstream error body"))
	case readErr != nil:
		return data, errors.Wrap(readErr, "read upstream error body")
	case closeErr != nil:
		return data, errors.Wrap(closeErr, "close upstream error body")
	}
	return data, nil
}

// relayErrorFromUpstreamBody normalizes an upstream error response whose body
// was already read by readUpstreamErrorBody.
//
// Parameters:
//   - resp: the non-nil upstream response, used for its status and headers.
//   - body: the bounded body bytes.
//   - readErr: the read or close failure, if any; it takes precedence over body.
//
// Returns:
//   - *model.ErrorWithStatusCode: the normalized error, marked FromUpstream.
//     Message holds a parsed provider message or a generic description;
//     RawError holds diagnostics but no body bytes.
func relayErrorFromUpstreamBody(resp *http.Response, body []byte, readErr error) *model.ErrorWithStatusCode {
	status := resp.StatusCode
	contentType := resp.Header.Get("Content-Type")
	// Everything below describes the provider's response, so the result is marked
	// FromUpstream: a provider-supplied "one_api_error" type must never be trusted
	// as this gateway's own error.
	result := &model.ErrorWithStatusCode{
		StatusCode: status,
		Error: model.Error{
			Type:         model.ErrorTypeUpstream,
			Code:         "bad_response_status_code",
			Param:        strconv.Itoa(status),
			FromUpstream: true,
			RawError:     errors.Errorf("bad response %d", status),
		},
	}

	switch {
	case readErr != nil:
		// The transport error can carry upstream URLs; keep it out of the message.
		result.Message = model.UnreadableUpstreamErrorMessage(status)
		result.RawError = errors.Wrapf(readErr, "upstream HTTP %d", status)
		return result
	case len(body) > model.MaxUpstreamErrorBodyBytes:
		// Never parse a truncated prefix: it could yield a misleading message.
		result.Message = model.UnrecognizedUpstreamErrorMessage(status, contentType, len(body), true)
		result.RawError = errors.Errorf("upstream HTTP %d error body exceeds %d bytes", status, model.MaxUpstreamErrorBodyBytes)
		return result
	case len(bytes.TrimSpace(body)) == 0:
		result.Message = model.UnrecognizedUpstreamErrorMessage(status, contentType, 0, false)
		result.RawError = errors.Errorf("upstream HTTP %d error body is empty", status)
		return result
	}

	errResponse, err := decodeUpstreamErrorEnvelope(body)
	if err != nil {
		// Not a JSON object (HTML, text, array, malformed JSON): describe it.
		result.Message = model.UnrecognizedUpstreamErrorMessage(status, contentType, len(body), false)
		result.RawError = errors.Wrapf(err, "decode upstream HTTP %d error body", status)
		return result
	}

	if errResponse.Error.Message != "" {
		// A nested OpenAI error that carries a message is authoritative as a whole.
		result.Error = errResponse.Error
		result.FromUpstream = true
		result.RawError = errors.New(result.Message)
		return result
	}

	// Otherwise take attribution field by field: the nested error object first,
	// then flat OpenAI-compatible keys, then the defaults above.
	switch {
	case errResponse.Error.Type != "":
		result.Type = errResponse.Error.Type
	case errResponse.Type != "":
		result.Type = errResponse.Type
	}
	switch {
	case errResponse.Error.Code != nil:
		result.Code = errResponse.Error.Code
	case errResponse.Code != nil:
		result.Code = errResponse.Code
	}
	switch {
	case errResponse.Error.Param != "":
		result.Param = errResponse.Error.Param
	case errResponse.Param != nil:
		result.Param = *errResponse.Param
	}

	result.Message = errResponse.ToMessage()
	if result.Message == "" {
		result.Message = model.UnrecognizedUpstreamErrorMessage(status, contentType, len(body), false)
		result.RawError = errors.Errorf("upstream HTTP %d error body has no recognizable message", status)
	}
	return result
}
