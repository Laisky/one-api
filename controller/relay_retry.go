package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// shouldRetry returns nil if should retry, otherwise returns error.
// Parameters: c carries the routing context (specific-channel pinning),
// bizErr is the normalized relay failure whose status, raw error, and
// user-originated classification drive the retry decision.
// Returns: nil when a retry may help, otherwise an error explaining the skip.
func shouldRetry(c *gin.Context, bizErr *model.ErrorWithStatusCode) error {
	if bizErr == nil {
		return nil
	}
	statusCode := bizErr.StatusCode
	rawErr := bizErr.RawError

	if specificChannelId := c.GetInt(ctxkey.SpecificChannelId); specificChannelId != 0 {
		return errors.Errorf(
			"specific channel ID (%d) was provided, retry is unvailable",
			specificChannelId)
	}

	// If we received a server error (5xx) but the underlying raw error is due to the caller's
	// context being cancelled or its deadline exceeded, we should NOT retry. Retrying would
	// waste quota and may incorrectly penalize the channel because the user aborted.
	if rawErr != nil {
		if errors.Is(rawErr, context.Canceled) || errors.Is(rawErr, context.DeadlineExceeded) {
			return errors.Wrap(rawErr, "do not retry: context cancelled or deadline exceeded")
		}
	}

	// Do not retry on client-request errors except for rate limit (429), capacity (413), and auth (401/403)
	// 404 should NOT retry, so it must not be excluded here.
	if statusCode >= 400 &&
		statusCode < 500 &&
		statusCode != http.StatusTooManyRequests &&
		statusCode != http.StatusRequestEntityTooLarge &&
		statusCode != http.StatusUnauthorized &&
		statusCode != http.StatusForbidden {
		return errors.Errorf("client error %d, not retrying", statusCode)
	}

	// A user-originated auth failure (insufficient quota, invalid/expired token,
	// model/tool not allowed, ...) cannot be fixed by switching channels, so it
	// must not enter the retry loop even though 401/403 are otherwise retryable:
	// upstream auth/permission failures may still be resolved by another channel.
	if (statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden) &&
		isUserOriginatedRelayError(bizErr) {
		return errors.Errorf("user-originated relay error (%s), not retrying", relayErrorCodeOrMessage(bizErr))
	}

	return nil
}

// relayErrorCodeOrMessage returns a stable diagnostic label for a relay error,
// preferring the error code and falling back to the message when the code is empty.
func relayErrorCodeOrMessage(e *model.ErrorWithStatusCode) string {
	if e == nil {
		return "unknown"
	}
	label := ""
	if e.Code != nil {
		label = strings.TrimSpace(fmt.Sprint(e.Code))
	}
	if label == "" {
		label = strings.TrimSpace(e.Message)
	}
	if label == "" {
		label = "unknown"
	}
	return label
}

// isRetryableUpstreamClientError reports whether a nominal 4xx upstream error should
// still be considered retryable by one-api.
//
// Parameters:
//   - relayErr: normalized relay error from upstream/adaptor.
//
// Returns:
//   - bool: true when this is a known transient upstream-client error shape.
func isRetryableUpstreamClientError(relayErr *model.ErrorWithStatusCode) bool {
	retryable, _ := classifyRetryableUpstreamClientError(relayErr)
	return retryable
}

// classifyRetryableUpstreamClientError evaluates whether a nominal 4xx error is
// actually retryable and returns a stable reason string for diagnostics.
//
// Parameters:
//   - relayErr: normalized relay error from upstream/adaptor.
//
// Returns:
//   - bool: true when this is a known transient upstream-client error shape.
//   - string: retry reason identifier for debug logging.
func classifyRetryableUpstreamClientError(relayErr *model.ErrorWithStatusCode) (bool, string) {
	if relayErr == nil {
		return false, ""
	}

	if relayErr.StatusCode < http.StatusBadRequest || relayErr.StatusCode >= http.StatusInternalServerError {
		return false, ""
	}

	code := strings.ToLower(strings.TrimSpace(fmt.Sprint(relayErr.Code)))
	message := strings.ToLower(strings.TrimSpace(relayErr.Message))

	if code == "websocket_connection_limit_reached" {
		return true, "websocket_connection_limit_reached"
	}

	if code == "output_parse_failed" {
		return true, "output_parse_failed"
	}

	if strings.Contains(message, "websocket connection limit reached") ||
		strings.Contains(message, "create a new websocket connection") {
		return true, "websocket_reconnect_hint"
	}

	if strings.Contains(message, "generated output that could not be parsed") {
		return true, "upstream_generated_unparseable_output"
	}

	return false, ""
}
