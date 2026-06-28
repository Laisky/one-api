package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/helper"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/monitor"
	"github.com/Laisky/one-api/relay/model"
)

// isClientContextCancel returns true if the error is caused by the caller's context
// cancellation or deadline exceeded conditions. These are typically user-originated
// and should be logged at WARN instead of ERROR to avoid false alerts.
func isClientContextCancel(statusCode int, rawErr error) bool {
	if rawErr != nil {
		if errors.Is(rawErr, context.Canceled) || errors.Is(rawErr, context.DeadlineExceeded) {
			return true
		}
	}
	// Also treat explicit 408 (Request Timeout) as client-side timeout in our pipeline
	if statusCode == http.StatusRequestTimeout {
		return true
	}
	return false
}

// isInternalInfraError reports whether rawErr is an internal infrastructure failure
// (e.g. ffprobe unavailable) rather than an upstream/channel fault, so the caller can
// skip channel suspension/disable and only emit a failure metric.
func isInternalInfraError(rawErr error) bool {
	if rawErr == nil {
		return false
	}
	if helper.IsFFProbeUnavailable(rawErr) {
		return true
	}
	return false
}

// isAdaptorInternalError reports whether err is an internal one-api adaptor error (a 5xx
// of type ErrorTypeOneAPI), which signals a gateway-side bug rather than an upstream
// channel fault, so the channel must not be suspended/disabled for it.
func isAdaptorInternalError(err *model.ErrorWithStatusCode) bool {
	if err == nil {
		return false
	}
	if err.StatusCode >= http.StatusInternalServerError && err.Type == model.ErrorTypeOneAPI {
		return true
	}
	return false
}

// upstreamSuggestsRetry detects whether the upstream error response indicates that the
// request should be retried. When this returns true, one-api should NOT suspend the
// ability or channel, as the upstream is signaling a transient issue rather than a
// persistent failure.
//
// Common patterns from various providers:
//   - OpenAI: "You can retry your request"
//   - Generic: "please try again", "try again later", "retry later"
//   - Server overload: "overloaded", "temporarily unavailable", "service unavailable"
func upstreamSuggestsRetry(err *model.ErrorWithStatusCode) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Message)
	if msg == "" {
		return false
	}

	// Common retry suggestion patterns from various AI providers
	retryPatterns := []string{
		"retry your request",
		"please retry",
		"try again",
		"retry later",
		"temporarily unavailable",
		"overloaded",
		"server is busy",
		"service is busy",
		"high load",
		"high traffic",
		"capacity limit",
		"temporary failure",
		"temporary error",
	}

	for _, pattern := range retryPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// classifyAuthLike returns true if error appears to be auth/permission/quota related
func classifyAuthLike(e *model.ErrorWithStatusCode) bool {
	if e == nil {
		return false
	}
	// Direct status codes
	if e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden {
		return true
	}
	// Check error type/code/message heuristics
	t := e.Type
	if t == model.ErrorTypeAuthentication || t == model.ErrorTypePermission ||
		t == model.ErrorTypeInsufficientQuota || t == model.ErrorTypeForbidden {
		return true
	}
	switch v := e.Code.(type) {
	case string:
		if v == "invalid_api_key" || v == "account_deactivated" || v == "insufficient_quota" {
			return true
		}
	}
	msg := e.Message
	if msg != "" {
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "invalid api key") || strings.Contains(lower, "api key not valid") || strings.Contains(lower, "api key expired") || strings.Contains(lower, "insufficient quota") || strings.Contains(lower, "insufficient credit") || strings.Contains(lower, "已欠费") || strings.Contains(lower, "余额不足") || strings.Contains(lower, "organization restricted") {
			return true
		}
	}
	return false
}

// getChannelIds returns the channel IDs from a failed-channels map, for debug logging.
func getChannelIds(failedChannels map[int]bool) []int {
	var ids []int
	for id := range failedChannels {
		ids = append(ids, id)
	}
	return ids
}

// logChannelSuspensionStatus checks and logs the database suspension status of the failed
// channels for debugging. It only performs the expensive queries when debug logging is enabled.
func logChannelSuspensionStatus(ctx context.Context, group, model string, failedChannelIds map[int]bool) {
	// Only perform expensive diagnostics if debug logging is enabled
	if !config.DebugEnabled {
		return
	}

	if len(failedChannelIds) == 0 {
		return
	}

	lg := gmw.GetLogger(ctx)

	var channelIds []int
	for id := range failedChannelIds {
		channelIds = append(channelIds, id)
	}

	var abilities []dbmodel.Ability
	now := time.Now()
	groupCol := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupCol = "\"group\""
	}

	err := dbmodel.DB.Where(groupCol+" = ? AND model = ? AND channel_id IN (?)", group, model, channelIds).Find(&abilities).Error
	if err != nil {
		lg.Warn("failed to inspect suspension status during relay diagnostics",
			zap.Error(err),
			zap.String("group", group),
			zap.String("model", model),
			zap.Ints("failed_channel_ids", channelIds),
		)
		return
	}

	var suspended []int
	var available []int

	for _, ability := range abilities {
		if ability.SuspendUntil != nil && ability.SuspendUntil.After(now) {
			suspended = append(suspended, ability.ChannelId)
		} else if ability.Enabled {
			available = append(available, ability.ChannelId)
		}
	}

	if len(suspended) > 0 {
		lg.Info("Debug: Database suspension status",
			zap.Ints("suspended_channels", suspended),
			zap.Ints("available_channels", available),
			zap.String("model", model),
			zap.String("group", group),
		)
	}
}

// processChannelRelayErrorParams contains all parameters needed for error processing.
// This struct helps maintain readability when passing multiple context values.
type processChannelRelayErrorParams struct {
	RequestID     string
	UserId        int
	TokenId       int
	ChannelId     int
	ChannelName   string
	Group         string
	OriginalModel string
	ActualModel   string
	RequestURL    string
	Err           model.ErrorWithStatusCode
}

// appendRelayFailureFields builds consistent relay failure context fields from params and appends extra fields.
// Parameters: params carries request, user, token, channel, model, and upstream error context; extra adds log-specific details.
// Returns: a zap field slice suitable for structured WARN/ERROR relay logs.
func appendRelayFailureFields(params processChannelRelayErrorParams, extra ...zap.Field) []zap.Field {
	fields := make([]zap.Field, 0, 12+len(extra))
	if params.RequestID != "" {
		fields = append(fields, zap.String("request_id", params.RequestID))
	}
	if params.RequestURL != "" {
		fields = append(fields, zap.String("request_url", params.RequestURL))
	}
	fields = append(fields,
		zap.Int("user_id", params.UserId),
		zap.Int("token_id", params.TokenId),
		zap.Int("channel_id", params.ChannelId),
	)
	if params.ChannelName != "" {
		fields = append(fields, zap.String("channel_name", params.ChannelName))
	}
	if params.Group != "" {
		fields = append(fields, zap.String("group", params.Group))
	}
	if params.OriginalModel != "" {
		fields = append(fields, zap.String("origin_model", params.OriginalModel))
	}
	if params.ActualModel != "" {
		fields = append(fields, zap.String("actual_model", params.ActualModel))
	}
	if params.Err.StatusCode > 0 {
		fields = append(fields, zap.Int("status_code", params.Err.StatusCode))
	}
	if errorCode := strings.TrimSpace(fmt.Sprint(params.Err.Code)); errorCode != "" && errorCode != "<nil>" {
		fields = append(fields, zap.String("error_code", errorCode))
	}
	if errorType := strings.TrimSpace(string(params.Err.Type)); errorType != "" {
		fields = append(fields, zap.String("error_type", errorType))
	}
	if upstreamError := strings.TrimSpace(params.Err.Message); upstreamError != "" {
		fields = append(fields, zap.String("upstream_error", upstreamError))
	}

	return append(fields, extra...)
}

// isExpectedChannelSelectionExhaustedError reports whether err means retry candidates were exhausted rather than an infrastructure failure.
// Parameters: err is the channel-selection error returned by the retry path.
// Returns: true when no alternative channel is available and false when the failure is unexpected and should stay at ERROR.
func isExpectedChannelSelectionExhaustedError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}

	errMsg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(errMsg, "no channels available for model") {
		return true
	}

	return strings.Contains(errMsg, "channel not found in memory cache")
}

// processChannelRelayErrorForTest, when non-nil, replaces the async error-processing
// body so tests can observe exactly what params (snapshotted Err) and context the
// goroutine sees. The ctx lets tests assert the error path runs on a detached, c-free,
// non-cancelled context (relayctx.Detach(c) from Relay), not the request context.
var processChannelRelayErrorForTest func(ctx context.Context, params processChannelRelayErrorParams)

// processChannelRelayErrorGateForTest, when non-nil, parks the spawned goroutine until
// the channel is closed, making the snapshot timing deterministically testable.
var processChannelRelayErrorGateForTest chan struct{}

// goProcessChannelRelayError snapshots the upstream error into params SYNCHRONOUSLY on
// the request goroutine before spawning the async processor, so it can never read
// bizErr.Error.Message while the request goroutine rewrites it for the client response.
func goProcessChannelRelayError(ctx context.Context, bizErr *model.ErrorWithStatusCode, params processChannelRelayErrorParams) {
	params.Err = *bizErr // synchronous snapshot on the request goroutine
	gate := processChannelRelayErrorGateForTest
	graceful.GoCritical(ctx, "processChannelRelayError", func(ctx context.Context) {
		if gate != nil {
			<-gate
		}
		if processChannelRelayErrorForTest != nil {
			processChannelRelayErrorForTest(ctx, params)
			return
		}
		processChannelRelayError(ctx, params)
	})
}

// processChannelRelayError applies the channel-health policy for a failed relay attempt:
// it logs the failure at the appropriate level, then decides whether to suspend the
// ability, auto-disable the channel, or merely emit a failure metric, based on the error's
// status code, type, and origin (user-originated vs upstream/infra). It runs on the
// detached, c-free context snapshotted by goProcessChannelRelayError.
func processChannelRelayError(ctx context.Context, params processChannelRelayErrorParams) {
	// Always use a local logger variable
	lg := gmw.GetLogger(ctx)
	isUserError := isUserOriginatedRelayError(&params.Err)

	// Downgrade to WARN for client-side cancellations/timeouts, user-originated
	// errors, and upstream rate limits.
	if isClientContextCancel(params.Err.StatusCode, params.Err.RawError) {
		lg.Warn("relay aborted by client (context canceled/deadline)",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
	} else if isUserError {
		lg.Warn("user-originated request error",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
	} else if params.Err.StatusCode == http.StatusTooManyRequests {
		lg.Warn("relay error",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
	} else {
		lg.Error("relay error",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
	}

	if isInternalInfraError(params.Err.RawError) {
		lg.Debug("internal infrastructure failure detected, skipping channel suspension",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
		monitor.Emit(params.ChannelId, false)
		return
	}

	if isAdaptorInternalError(&params.Err) {
		lg.Info("internal adaptor error, skipping channel suspension",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
		monitor.Emit(params.ChannelId, false)
		return
	}

	if isUserError {
		lg.Warn("user-originated request error, skipping channel suspension",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
		monitor.Emit(params.ChannelId, false)
		return
	}

	// Handle 400 errors differently - they are client request issues, not channel problems
	if params.Err.StatusCode == http.StatusBadRequest {
		// For 400 errors, log but don't disable channel or suspend abilities
		// These are typically schema validation errors or malformed requests
		lg.Info("client request error (400) for channel - not disabling channel as this is not a channel issue",
			appendRelayFailureFields(params, zap.Error(params.Err.RawError))...,
		)
		// Still emit failure for monitoring purposes, but don't disable the channel
		monitor.Emit(params.ChannelId, false)
		return
	}

	if params.Err.StatusCode == http.StatusTooManyRequests {
		// For 429, we will suspend the specific model for a while
		lg.Warn("ability suspended due to rate limit (429)",
			appendRelayFailureFields(params,
				zap.Error(params.Err.RawError),
				zap.String("suspension_rationale", "upstream rate limit exceeded; suspending ability to allow cooldown"),
				zap.Duration("suspension_duration", config.ChannelSuspendSecondsFor429),
			)...,
		)
		if suspendErr := dbmodel.SuspendAbility(ctx,
			params.Group, params.OriginalModel, params.ChannelId,
			config.ChannelSuspendSecondsFor429); suspendErr != nil {
			lg.Error("failed to suspend ability for channel",
				appendRelayFailureFields(params,
					zap.Error(errors.Wrap(suspendErr, "suspend ability failed")),
				)...,
			)
		}
		monitor.Emit(params.ChannelId, false)
		return
	}

	// context cancel or deadline exceeded - likely user aborted or timeout.
	// Detect via status or RawError classification; avoid suspending/disabling.
	if params.Err.StatusCode == http.StatusRequestTimeout || (params.Err.RawError != nil && (errors.Is(params.Err.RawError, context.Canceled) || errors.Is(params.Err.RawError, context.DeadlineExceeded))) {
		monitor.Emit(params.ChannelId, false)
		return
	}

	// 413 capacity issues: do not suspend; rely on retry selection to seek larger max_tokens
	if params.Err.StatusCode == http.StatusRequestEntityTooLarge {
		monitor.Emit(params.ChannelId, false)
		return
	}

	// 5xx or network-type server errors -> conditionally suspend ability
	// If upstream explicitly suggests retry, do NOT suspend - the service is healthy but had a one-off issue
	if params.Err.StatusCode >= 500 && params.Err.StatusCode <= 599 {
		if upstreamSuggestsRetry(&params.Err) {
			lg.Debug("upstream suggests retry for 5xx error, skipping ability suspension",
				appendRelayFailureFields(params,
					zap.Error(params.Err.RawError),
					zap.String("skip_rationale", "upstream error message suggests retry; treating as transient one-off issue"),
				)...,
			)
			monitor.Emit(params.ChannelId, false)
			return
		}

		lg.Error("ability suspended due to server error (5xx)",
			appendRelayFailureFields(params,
				zap.Error(params.Err.RawError),
				zap.String("suspension_rationale", "upstream server error; suspending ability to allow recovery"),
				zap.Duration("suspension_duration", config.ChannelSuspendSecondsFor5XX),
			)...,
		)
		if suspendErr := dbmodel.SuspendAbility(ctx, params.Group, params.OriginalModel, params.ChannelId, config.ChannelSuspendSecondsFor5XX); suspendErr != nil {
			lg.Error("failed to suspend ability for 5xx",
				appendRelayFailureFields(params,
					zap.Error(errors.Wrap(suspendErr, "suspend ability failed")),
				)...,
			)
		}
		// Do not immediately auto-disable; transient
		monitor.Emit(params.ChannelId, false)
		return
	}

	// Auth/permission/quota errors (401/403 or vendor-indicated) -> suspend ability; escalate to auto-disable only if fatal
	if params.Err.StatusCode == http.StatusUnauthorized || params.Err.StatusCode == http.StatusForbidden || classifyAuthLike(&params.Err) {
		lg.Error("ability suspended due to auth/permission error",
			appendRelayFailureFields(params,
				zap.Error(params.Err.RawError),
				zap.String("suspension_rationale", "authentication or permission failure; suspending ability pending credential verification"),
				zap.Duration("suspension_duration", config.ChannelSuspendSecondsForAuth),
			)...,
		)
		if suspendErr := dbmodel.SuspendAbility(ctx, params.Group, params.OriginalModel, params.ChannelId, config.ChannelSuspendSecondsForAuth); suspendErr != nil {
			lg.Error("failed to suspend ability for auth/permission",
				appendRelayFailureFields(params,
					zap.Error(errors.Wrap(suspendErr, "suspend ability failed")),
				)...,
			)
		}

		if monitor.ShouldDisableChannel(&params.Err.Error, params.Err.StatusCode) {
			lg.Error("channel disabled due to fatal auth/permission error",
				appendRelayFailureFields(params,
					zap.Error(params.Err.RawError),
					zap.String("disable_rationale", "fatal auth error detected; channel automatically disabled"),
				)...,
			)
			monitor.DisableChannel(params.ChannelId, params.ChannelName, params.Err.Message)
		} else {
			monitor.Emit(params.ChannelId, false)
		}
		return
	}

	// Default: not fatal -> record failure only. If fatal per policy, auto-disable.
	if monitor.ShouldDisableChannel(&params.Err.Error, params.Err.StatusCode) {
		lg.Error("channel disabled due to fatal error",
			appendRelayFailureFields(params,
				zap.Error(params.Err.RawError),
				zap.String("disable_rationale", "fatal error per auto-disable policy; channel automatically disabled"),
			)...,
		)
		monitor.DisableChannel(params.ChannelId, params.ChannelName, params.Err.Message)
	} else {
		monitor.Emit(params.ChannelId, false)
	}
}

// isUserOriginatedRelayError reports whether a relay failure was caused by caller-side
// request or quota conditions rather than upstream/channel health.
//
// Return values:
//   - true: user-originated and should not trigger channel suspension/disable.
//   - false: may be upstream/channel/system failure and can follow normal error policy.
func isUserOriginatedRelayError(e *model.ErrorWithStatusCode) bool {
	if e == nil {
		return false
	}

	if isClientContextCancel(e.StatusCode, e.RawError) {
		return true
	}

	if e.StatusCode == http.StatusBadRequest && e.Type == model.ErrorTypeOneAPI {
		return true
	}

	if isUpstreamMalformedToolCallError(e) {
		return true
	}

	if e.StatusCode != http.StatusForbidden && e.StatusCode != http.StatusUnauthorized {
		return false
	}

	if e.Type != model.ErrorTypeOneAPI {
		return false
	}

	code := ""
	switch v := e.Code.(type) {
	case string:
		code = strings.ToLower(v)
	}

	if code == "insufficient_user_quota" || code == "insufficient_token_quota" ||
		code == "invalid_api_key" || code == "token_expired" || code == "token_disabled" || code == "token_not_found" ||
		code == "model_not_allowed" || code == "model_not_available" || code == "tool_not_allowed" {
		return true
	}

	msg := strings.ToLower(e.Message)
	if code == "pre_consume_token_quota_failed" &&
		(strings.Contains(msg, "insufficient user quota") || strings.Contains(msg, "insufficient token quota") || strings.Contains(msg, "user quota is not enough") || strings.Contains(msg, "token quota is not enough")) {
		return true
	}

	if strings.Contains(msg, "token has expired") || strings.Contains(msg, "token is not enabled") ||
		strings.Contains(msg, "api key is invalid") || strings.Contains(msg, "api key has been disabled") ||
		strings.Contains(msg, "model not allowed") || strings.Contains(msg, "model is not available") || strings.Contains(msg, "not allowed for this token") ||
		strings.Contains(msg, "token model") || strings.Contains(msg, "quota has been exhausted") || strings.Contains(msg, "token quota exhausted") ||
		strings.Contains(msg, "whitelist") || strings.Contains(msg, "blacklist") {
		return true
	}

	return false
}

// isUpstreamMalformedToolCallError reports whether a 400 upstream error indicates
// the model produced malformed tool-call arguments JSON.
//
// These errors are user/request-side outcomes (prompt/model generation mismatch),
// not channel health failures, so they should use user-originated handling.
func isUpstreamMalformedToolCallError(e *model.ErrorWithStatusCode) bool {
	if e == nil || e.StatusCode != http.StatusBadRequest {
		return false
	}

	code := strings.ToLower(strings.TrimSpace(fmt.Sprint(e.Code)))
	message := strings.ToLower(strings.TrimSpace(e.Message))

	if code == "tool_use_failed" {
		return true
	}

	if code == "invalid_request_error" || code == "" {
		if strings.Contains(message, "failed to parse tool call arguments as json") {
			return true
		}
		if strings.Contains(message, "tool call arguments") && strings.Contains(message, "json") {
			return true
		}
	}

	return false
}
