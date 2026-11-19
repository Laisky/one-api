package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/asyncvideo"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// videoUpstreamErrorMessageLimit bounds, in bytes, the provider message copied
// into a client-visible error for a non-2xx video response.
const videoUpstreamErrorMessageLimit = 1024

// videoUpstreamErrorCode is the client-visible code used when a non-2xx video
// response carries no identifier-shaped provider code.
const videoUpstreamErrorCode = "upstream_video_error"

// videoUpstreamErrorCodePattern accepts provider error codes that are safe to
// expose verbatim as identifiers.
var videoUpstreamErrorCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,128}$`)

// VideoHandler forwards successful OpenAI video responses (JSON job metadata or
// binary content) unchanged to the caller. It logs only payload shape metadata.
// A typed provider error ({"error":{"type":...}}) is surfaced as decoded; any
// other non-2xx response is never forwarded and becomes a provider-attributed
// error carrying the provider status, so the relay refunds and fails the attempt.
//
// Parameters:
//   - c: the request context whose writer receives successful responses.
//   - resp: the provider response; its body is fully consumed and closed.
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: the failure, or nil when forwarded.
//   - *relaymodel.Usage: always nil; video responses carry no usage.
func VideoHandler(c *gin.Context, resp *http.Response) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	logger := gmw.GetLogger(c)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	logFields := []zap.Field{
		zap.Int("body_bytes", len(body)),
		zap.Bool("body_logging_suppressed", true),
	}
	if len(body) == 0 {
		logger.Debug("video handler upstream response empty", logFields...)
	} else {
		logger.Debug("video handler upstream response", logFields...)
	}

	var maybeError struct {
		Error *relaymodel.Error `json:"error,omitempty"`
	}
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &maybeError); unmarshalErr == nil {
			if maybeError.Error != nil && maybeError.Error.Type != "" {
				maybeError.Error.RawError = nil
				// Decoded from the provider body and returned with the provider's
				// status: attribute it to the channel, not to one-api.
				maybeError.Error.FromUpstream = true
				return &relaymodel.ErrorWithStatusCode{
					Error:      *maybeError.Error,
					StatusCode: resp.StatusCode,
				}, nil
			}
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		message, code := videoUpstreamErrorDetails(body, resp.StatusCode)
		logger.Debug("video handler upstream error not forwarded",
			zap.Int("status_code", resp.StatusCode),
			zap.String("error_code", code),
			zap.Int("body_bytes", len(body)))
		return UpstreamErrorWrapper(errors.New(message), code, resp.StatusCode), nil
	}

	if resp.StatusCode < http.StatusBadRequest && c.Request.Method == http.MethodPost {
		PersistAsyncVideoTask(c, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))

	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	return nil, nil
}

// PersistAsyncVideoTask preserves the shared task-binding entrypoint used by
// OpenAI and compatible providers without coupling provider implementations.
func PersistAsyncVideoTask(c *gin.Context, body []byte) {
	asyncvideo.PersistTask(c, body)
}

// videoUpstreamErrorDetails extracts a client-safe message and code from a
// non-2xx video response body without ever echoing raw body bytes.
//
// Parameters:
//   - body: the provider response body.
//   - statusCode: the provider HTTP status, used in the generic message.
//
// Returns:
//   - string: the trimmed, bounded provider message from a nested error.message
//     or flat top-level message string, else a generic status message.
//   - string: the provider's identifier-shaped code from a nested error.code or
//     flat top-level code string, else videoUpstreamErrorCode.
func videoUpstreamErrorDetails(body []byte, statusCode int) (string, string) {
	message := fmt.Sprintf("OpenAI video upstream returned HTTP %d", statusCode)
	code := videoUpstreamErrorCode

	var envelope struct {
		Error   json.RawMessage `json:"error"`
		Message json.RawMessage `json:"message"`
		Code    json.RawMessage `json:"code"`
	}
	if len(body) == 0 || json.Unmarshal(body, &envelope) != nil {
		return message, code
	}
	var nested struct {
		Message json.RawMessage `json:"message"`
		Code    json.RawMessage `json:"code"`
	}
	if len(envelope.Error) > 0 && json.Unmarshal(envelope.Error, &nested) != nil {
		nested.Message, nested.Code = nil, nil
	}

	if text := videoUpstreamErrorString(nested.Message); text != "" {
		message = boundVideoUpstreamErrorMessage(text)
	} else if text := videoUpstreamErrorString(envelope.Message); text != "" {
		message = boundVideoUpstreamErrorMessage(text)
	}
	if text := videoUpstreamErrorString(nested.Code); videoUpstreamErrorCodePattern.MatchString(text) {
		code = text
	} else if text := videoUpstreamErrorString(envelope.Code); videoUpstreamErrorCodePattern.MatchString(text) {
		code = text
	}
	return message, code
}

// videoUpstreamErrorString decodes raw as a JSON string and trims it.
//
// Parameters:
//   - raw: a JSON value from the provider body.
//
// Returns:
//   - string: the trimmed string, or "" when raw is absent or not a JSON string.
func videoUpstreamErrorString(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// boundVideoUpstreamErrorMessage truncates text to at most
// videoUpstreamErrorMessageLimit bytes without splitting a UTF-8 sequence.
//
// Parameters:
//   - text: the provider message.
//
// Returns:
//   - string: text itself when short enough, else its bounded prefix.
func boundVideoUpstreamErrorMessage(text string) string {
	if len(text) <= videoUpstreamErrorMessageLimit {
		return text
	}
	limit := videoUpstreamErrorMessageLimit
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}
