package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/model"
)

// shouldRetry returns nil if should retry, otherwise returns error.
// Parameters: c carries the routing context (specific-channel pinning),
// bizErr is the normalized relay failure whose status, raw error, and
// user-originated classification drive the retry decision.
// Returns: nil when a retry may help, otherwise an error explaining the skip.
func shouldRetry(c *gin.Context, bizErr *model.ErrorWithStatusCode) error {
	// Judge the attempt that just failed before consulting the paid-video veto,
	// so this decision and every later videoReplayUnsafe call in the retry loop
	// (loop guard, transient-4xx exception, outcome logging) see its verdict.
	recordVideoCreationVerdict(c, bizErr)
	if videoReplayUnsafe(c) {
		return errors.New("video task is durable or may already be accepted; automatic replay is unsafe")
	}
	if bizErr == nil {
		return nil
	}
	if c.GetBool(adaptor.ImageReceiptAcceptedKey) || c.GetBool(adaptor.AudioReceiptAcceptedKey) {
		return errors.New("upstream work is already accepted; replay after downstream failure is unsafe")
	}
	statusCode := bizErr.StatusCode
	rawErr := bizErr.RawError

	if specificChannelId := c.GetInt(ctxkey.SpecificChannelId); specificChannelId != 0 {
		return errors.Errorf(
			"specific channel ID (%d) was provided, retry is unvailable",
			specificChannelId)
	}

	// A Live-only Google model cannot use this selected channel's REST adaptor,
	// but a later OpenAI-compatible bridge can legally implement the same model
	// ID. Keep the pinned-channel contract above, then let normal exclusions walk
	// the remaining candidates.
	if adaptor.IsRESTTransportMismatch(rawErr) {
		return nil
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

// videoReplayUnsafe is an authoritative veto, including when a provider labels
// an error as a normally retryable 4xx. Local reservations are irreversible too.
// A paid video creation that may have been forwarded stays unsafe to replay
// unless the provider definitively rejected that same channel's attempt before
// admitting any job (see recordVideoCreationVerdict); an accepted job or a bound
// durable task is never replayed.
// Parameter c is the request context. It returns true when another channel must
// not receive the creation.
func videoReplayUnsafe(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if c.GetString(asyncvideo.DurableTaskKey) != "" || c.GetBool(adaptor.AsyncVideoAcceptedKey) {
		return true
	}
	if !isPaidVideoCreation(c) || !c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) {
		return false
	}
	rejectedChannel := c.GetInt(videoCreationRejectedChannelKey)
	return rejectedChannel <= 0 || rejectedChannel != c.GetInt(ctxkey.ChannelId)
}

// isPaidVideoCreation reports whether c is a POST that creates a paid video job.
// Parameter c is the request context. It returns true for the video creation
// routes whose replay could start a second billable job.
func isPaidVideoCreation(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodPost {
		return false
	}
	switch c.Request.URL.Path {
	case "/v1/videos", "/v1/videos/generations", "/v1/async/videos":
		return true
	default:
		return false
	}
}

// videoCreationRejectedChannelKey stores the channel id whose latest paid video
// creation attempt the provider definitively rejected before admission; zero
// means the latest attempt proved nothing. Binding the verdict to a channel id
// keeps it from vouching for a later attempt on another channel.
const videoCreationRejectedChannelKey = "relay.video_creation_rejected_channel"

// recordVideoCreationVerdict stores whether the provider definitively rejected
// the paid video creation attempt that produced bizErr, replacing any verdict of
// an earlier attempt. Parameters: c is the request context bound to the
// attempt's channel and bizErr the attempt's normalized failure. It returns
// nothing and leaves non-video requests untouched.
func recordVideoCreationVerdict(c *gin.Context, bizErr *model.ErrorWithStatusCode) {
	if !isPaidVideoCreation(c) {
		return
	}
	rejectedChannel := 0
	if providerRejectedVideoCreation(bizErr) {
		rejectedChannel = c.GetInt(ctxkey.ChannelId)
		lg := gmw.GetLogger(c)
		lg.Debug("provider rejected paid video creation before admission; replay on another channel is safe",
			relayErrorMetadataLogFields(bizErr)...,
		)
	}
	c.Set(videoCreationRejectedChannelKey, rejectedChannel)
}

// providerRejectedVideoCreation reports whether bizErr proves the provider
// refused a video creation before admitting any job, so no billable job exists.
// Only provider-attributed (FromUpstream) answers count: a locally produced
// error proves nothing about the provider. The allowed statuses are explicit
// admission refusals (400, 401, 402, 403, 404, 413, 422 and 429): providers
// return them synchronously before creating a job (xAI documents that auth,
// missing-model and rate-limit errors precede job creation; Google AIP-151
// requires errors that prevent a long-running operation from starting to be
// returned instead of an operation; Stripe and AWS treat validation, auth and
// throttling as refusals before execution). 408 is excluded because proxies
// and gateways use it for processing timeouts, 409 because it can mean an
// equivalent job is already running, and 5xx, transport failures, timeouts and
// cancellations because the job may exist. Transient 4xx shapes that describe
// generated output are excluded too, since the provider may have done work.
// Parameter bizErr is the attempt's normalized failure. It returns true only for
// a definitive provider rejection.
func providerRejectedVideoCreation(bizErr *model.ErrorWithStatusCode) bool {
	if bizErr == nil || !bizErr.FromUpstream {
		return false
	}
	if errors.Is(bizErr.RawError, context.Canceled) || errors.Is(bizErr.RawError, context.DeadlineExceeded) {
		return false
	}
	if isRetryableUpstreamClientError(bizErr) {
		return false
	}
	switch bizErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden,
		http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}
