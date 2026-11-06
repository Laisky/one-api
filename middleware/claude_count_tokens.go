package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
)

const (
	// ClaudeMessagesPath is the canonical, billed Claude Messages generation endpoint.
	ClaudeMessagesPath = "/v1/messages"
	// ClaudeCountTokensPath is the canonical, free Claude token-counting endpoint.
	ClaudeCountTokensPath = "/v1/messages/count_tokens"
	// ClaudeCountTokensMaxBodyBytes caps a count_tokens body at Anthropic's
	// documented 32 MiB Messages request limit. The endpoint is free, so it must
	// never accept more input than the provider would for a billed request.
	ClaudeCountTokensMaxBodyBytes int64 = 32 << 20
)

// IsClaudeCountTokensPath reports whether path is the canonical count_tokens
// endpoint, tolerating a single trailing slash.
//
// Parameters:
//   - path: the request URL path.
//
// Return values:
//   - bool: true for /v1/messages/count_tokens and /v1/messages/count_tokens/.
func IsClaudeCountTokensPath(path string) bool {
	return strings.TrimSuffix(path, "/") == ClaudeCountTokensPath
}

// ClaudeErrorTypeForStatus maps an HTTP status to the Anthropic error type that
// the Messages API documents for it.
//
// Parameters:
//   - status: the HTTP status code of the error response.
//
// Return values:
//   - string: the Anthropic error.type value.
func ClaudeErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	}
	if status >= http.StatusInternalServerError {
		return "api_error"
	}
	return "invalid_request_error"
}

// AbortWithClaudeError writes an Anthropic-format error envelope
// ({"type":"error","error":{"type":...,"message":...}}) and aborts the chain.
// It does not log; the caller owns the single log line for the failure.
//
// Parameters:
//   - c: the request context.
//   - status: the HTTP status to answer with.
//   - message: the client-facing message; it must not carry secrets.
func AbortWithClaudeError(c *gin.Context, status int, message string) {
	body := gin.H{
		"type": "error",
		"error": gin.H{
			"type":    ClaudeErrorTypeForStatus(status),
			"message": message,
		},
	}
	if requestID := c.GetString(helper.RequestIdKey); requestID != "" {
		body["request_id"] = requestID
	}
	c.JSON(status, body)
	c.Abort()
}

// claudeCountTokensBodyLimitBytes returns the effective count_tokens body cap:
// the smaller of ClaudeCountTokensMaxBodyBytes and the global relay cap.
//
// Return values:
//   - int64: the byte cap, always positive.
func claudeCountTokensBodyLimitBytes() int64 {
	if global := maxRequestBodyBytes(); global > 0 && global < ClaudeCountTokensMaxBodyBytes {
		return global
	}
	return ClaudeCountTokensMaxBodyBytes
}

// isRequestBodyTooLarge reports whether err comes from one of the request body caps.
//
// Parameters:
//   - err: the read error.
//
// Return values:
//   - bool: true when a byte cap rejected the body.
func isRequestBodyTooLarge(err error) bool {
	var limitError *http.MaxBytesError
	return errors.As(err, &limitError) || errors.Is(err, ErrRequestBodyTooLarge)
}

// ClaudeCountTokensBodyLimit buffers a count_tokens body, bounded by
// claudeCountTokensBodyLimitBytes, before authentication reads it, and answers
// an oversized or unreadable body with an Anthropic-format 413 or 400. The
// buffered bytes are cached for every later reader of the request body.
//
// Return values:
//   - gin.HandlerFunc: the middleware.
func ClaudeCountTokensBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil {
			c.Next()
			return
		}
		lg := gmw.GetLogger(c)
		limit := claudeCountTokensBodyLimitBytes()
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, limit+1))
		if err == nil && int64(len(body)) > limit {
			err = errors.WithStack(ErrRequestBodyTooLarge)
		}
		if err != nil {
			status, message := http.StatusBadRequest, "failed to read request body"
			if isRequestBodyTooLarge(err) {
				status, message = http.StatusRequestEntityTooLarge, "request body exceeds the count_tokens size limit"
			}
			lg.Warn("rejected claude count_tokens body",
				zap.Int("status_code", status),
				zap.Int64("limit_bytes", limit),
				zap.String("error", errMessage(err)),
			)
			AbortWithClaudeError(c, status, message)
			return
		}
		c.Set(ctxkey.KeyRequestBody, body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

// ClaudeCountTokensRequestGuard runs after TokenAuth and rejects, in Anthropic
// format, a count_tokens body that names no model. Without it the shared
// distributor would misreport an unparsable body as a missing channel (503).
// When a model is present TokenAuth has already checked it against the token.
//
// Return values:
//   - gin.HandlerFunc: the middleware.
func ClaudeCountTokensRequestGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetString(ctxkey.RequestModel)) != "" {
			c.Next()
			return
		}
		lg := gmw.GetLogger(c)
		message := "model: a non-empty string model is required"
		body, err := common.GetRequestBody(c)
		switch {
		case err != nil:
			message = "failed to read request body"
		case !json.Valid(body):
			message = "request body must be valid JSON"
		}
		lg.Warn("rejected claude count_tokens request without a usable model",
			zap.Int("status_code", http.StatusBadRequest),
			zap.Int("body_bytes", len(body)),
			zap.String("reason", message),
			zap.String("error", errMessage(err)),
		)
		AbortWithClaudeError(c, http.StatusBadRequest, message)
	}
}
