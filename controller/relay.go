package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/middleware"
	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	rcontroller "github.com/songquanpeng/one-api/relay/controller"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

func relayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	var err *model.ErrorWithStatusCode
	switch relayMode {
	case relaymode.Realtime:
		// For Phase 1, route through text helper which will delegate to adaptor based on meta.Mode
		// Realtime adaptor code will handle websocket upgrade and upstream pass-through.
		err = rcontroller.RelayTextHelper(c)
	case relaymode.ImagesGenerations,
		relaymode.ImagesEdits:
		err = rcontroller.RelayImageHelper(c, relayMode)
	case relaymode.AudioSpeech:
		fallthrough
	case relaymode.AudioTranslation:
		fallthrough
	case relaymode.AudioTranscription:
		err = rcontroller.RelayAudioHelper(c, relayMode)
	case relaymode.Proxy:
		err = rcontroller.RelayProxyHelper(c, relayMode)
	case relaymode.ResponseAPI:
		err = rcontroller.RelayResponseAPIHelper(c)
	case relaymode.ClaudeMessages:
		err = rcontroller.RelayClaudeMessagesHelper(c)
	default:
		err = rcontroller.RelayTextHelper(c)
	}
	return err
}

func Relay(c *gin.Context) {
	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)
	relayMode := relaymode.GetByPath(c.Request.URL.Path)
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)
	shouldDebugLog := relayMode == relaymode.ChatCompletions || relayMode == relaymode.ResponseAPI || relayMode == relaymode.ClaudeMessages
	if shouldDebugLog {
		rcontroller.EnsureDebugResponseWriter(c)
	}

	// Start timing for Prometheus metrics
	startTime := time.Now()

	// Request start log for traceability
	lg.Debug("incoming relay request",
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.Int("relay_mode", relayMode),
		zap.Int("channel_id", channelId),
		zap.Int("user_id", userId),
		zap.String("content_type", c.GetHeader("Content-Type")),
		zap.Int64("content_length", c.Request.ContentLength),
		zap.String("request_id", c.GetString(helper.RequestIdKey)),
	)

	// Get metadata for monitoring
	relayMeta := meta.GetByContext(c)

	// Track channel request in flight
	PrometheusMonitor.RecordChannelRequest(relayMeta, startTime)

	bizErr := relayHelper(c, relayMode)
	if bizErr == nil {
		monitor.Emit(channelId, true)

		// Record successful relay request metrics
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, true, 0, 0, 0)
		if shouldDebugLog {
			rcontroller.LogClientResponse(c, "client response sent")
		}
		return
	}
	lastFailedChannelId := channelId
	channelName := c.GetString(ctxkey.ChannelName)
	group := c.GetString(ctxkey.Group)
	originalModel := c.GetString(ctxkey.RequestModel)
	// Ensure channel error processing is completed during graceful drain
	graceful.GoCritical(ctx, "processChannelRelayError", func(ctx context.Context) {
		processChannelRelayError(ctx, userId, channelId, channelName, group, originalModel, *bizErr)
	})

	// Record failed relay request metrics
	PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, false, 0, 0, 0)

	requestId := c.GetString(helper.RequestIdKey)
	retryTimes := config.RetryTimes
	if err := shouldRetry(c, bizErr.StatusCode, bizErr.RawError); err != nil {
		// Downgrade to WARN if the failure is caused by caller's context cancellation/deadline exceeded
		if isClientContextCancel(bizErr.StatusCode, bizErr.RawError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			lg.Warn("relay aborted by client (context canceled/deadline), won't retry", zap.Int("status_code", bizErr.StatusCode), zap.Error(err))
		} else {
			lg.Error("relay error happen, won't retry", zap.Int("status_code", bizErr.StatusCode), zap.Error(err))
		}
		retryTimes = 0
	}

	// For 429 errors, increase retry attempts to exhaust all available channels
	// to avoid returning 429 to users when other channels might be available
	if bizErr.StatusCode == http.StatusTooManyRequests && retryTimes > 0 {
		// Try to get an estimate of available channels for this model/group
		// to increase retry attempts accordingly
		retryTimes = retryTimes * 2 // Increase retry attempts for 429 errors
		lg.Info("429 error detected, increasing retry attempts to exhaust alternative channels",
			zap.Int("retry_attempts", retryTimes),
		)
	}

	// For 413 errors, increase retry attempts to exhaust all available channels
	// to avoid returning 413 to users when other channels might be available
	if bizErr.StatusCode == http.StatusRequestEntityTooLarge {
		// Get the total number of channels for this model/group
		// and try to retry all channels
		channels, err := dbmodel.GetChannelsFromCache(group, originalModel)
		if err != nil {
			retryTimes = 1
			lg.Debug("413 error detected, Get channels from cache error",
				zap.Error(err),
			)
			lg.Warn("413 error detected, Failed to get total number of channels for a model/group from cache. increasing retry attempts",
				zap.Int("retry_attempts", retryTimes),
				zap.Error(err),
			)
		} else {
			retryTimes = len(channels) - 1
			lg.Info("413 error detected, increasing retry attempts to exhaust alternative channels",
				zap.Int("retry_attempts", retryTimes),
			)
		}
	}

	// Track failed channels to avoid retrying them, especially for 429 errors
	failedChannels := make(map[int]bool)
	failedChannels[lastFailedChannelId] = true

	// Debug logging to track channel exclusions (only when debug is enabled)
	if config.DebugEnabled {
		if retryTimes > 0 {
			lg.Info("Debug: Starting retry logic - Initial failed channel",
				zap.Int("initial_failed_channel", lastFailedChannelId),
				zap.Int("error_code", bizErr.StatusCode),
				zap.String("request_id", requestId),
			)
		} else {
			lg.Info("Debug: No retry will be attempted (retryTimes=0)",
				zap.Int("channel_id", lastFailedChannelId),
				zap.Int("error_code", bizErr.StatusCode),
				zap.String("request_id", requestId),
			)
		}
	}

	// For 429 errors, we should try lower priority channels first
	// since the highest priority channel is rate limited
	shouldTryLowerPriorityFirst := bizErr.StatusCode == http.StatusTooManyRequests

	// For 413 errors, we should try Larger MaxTokens channels
	shouldTryLargerMaxTokensFirst := bizErr.StatusCode == http.StatusRequestEntityTooLarge

	// For 5xx/server transient errors, avoid reusing the same ability first, probe within tier
	isServerTransient := bizErr.StatusCode >= 500 && bizErr.StatusCode <= 599

	for i := retryTimes; i > 0; i-- {
		var channel *dbmodel.Channel
		var err error

		// Try to find an available channel, preferring lower priority channels for 429 errors
		if config.DebugEnabled {
			lg.Info("Debug: Attempting retry",
				zap.Int("retry_attempt", retryTimes-i+1),
				zap.Ints("excluded_channels", getChannelIds(failedChannels)),
				zap.Bool("try_lower_priority_first", shouldTryLowerPriorityFirst),
				zap.Bool("try_larger_max_tokens_first", shouldTryLargerMaxTokensFirst),
				zap.Bool("server_transient", isServerTransient))
		}

		if shouldTryLargerMaxTokensFirst {
			// For 413 errors, try larger max_tokens channels
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, true)
		} else if shouldTryLowerPriorityFirst {
			// For 429 errors, first try lower priority channels while excluding failed ones
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, true, failedChannels, false)
			if err != nil {
				// If no lower priority channels available, try highest priority channels (excluding failed ones)
				lg.Info("No lower priority channels available, trying highest priority channels",
					zap.Ints("excluded_channels", getChannelIds(failedChannels)),
				)
				channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, false)
			}
		} else {
			// For non-429 errors, try highest priority first, then lower priority (excluding failed ones)
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, false)
			if err != nil {
				lg.Info("No highest priority channels available, trying lower priority channels",
					zap.Ints("excluded_channels", getChannelIds(failedChannels)))
				channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, true, failedChannels, false)
			}
		}

		if err != nil {
			lg.Error("CacheGetRandomSatisfiedChannelExcluding failed",
				zap.Error(err),
				zap.Ints("excluded_channels", getChannelIds(failedChannels)),
				zap.String("model", originalModel),
				zap.String("group", group))

			// Log database suspension status to help distinguish between in-memory and database exclusions
			// Only check the channels that were actually excluded in this request
			logChannelSuspensionStatus(ctx, group, originalModel, failedChannels)
			break
		}

		lg.Info("using channel to retry",
			zap.Int("channel_id", channel.Id),
			zap.Int("remaining_attempts", i),
		)
		middleware.SetupContextForSelectedChannel(c, channel, originalModel)
		requestBody, err := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		// Record retry attempt
		retryStartTime := time.Now()
		retryMeta := meta.GetByContext(c)

		bizErr = relayHelper(c, relayMode)
		if bizErr == nil {
			// Record successful retry
			PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, true, 0, 0, 0)
			return
		}

		// Record failed retry
		PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, false, 0, 0, 0)

		channelId := c.GetInt(ctxkey.ChannelId)
		failedChannels[channelId] = true // Track this failed channel
		lastFailedChannelId = channelId

		// Debug logging to track which channels are being added to failed list (only when debug is enabled)
		if config.DebugEnabled {
			lg.Info("Debug: Added channel to failed channels list",
				zap.Int("channel_id", channelId),
				zap.Ints("total_failed_channels", getChannelIds(failedChannels)),
				zap.String("request_id", requestId))
		}
		channelName := c.GetString(ctxkey.ChannelName)
		// Update group and originalModel potentially if changed by middleware, though unlikely for these.
		group = c.GetString(ctxkey.Group)
		originalModel = c.GetString(ctxkey.RequestModel)
		graceful.GoCritical(ctx, "processChannelRelayError", func(ctx context.Context) {
			processChannelRelayError(ctx, userId, channelId, channelName, group, originalModel, *bizErr)
		})
	}

	if bizErr != nil {
		if bizErr.StatusCode == http.StatusTooManyRequests {
			// Provide more specific messaging for 429 errors after exhausting retries
			if len(failedChannels) > 1 {
				bizErr.Error.Message = fmt.Sprintf("All available channels (%d) for this model are currently rate limited, please try again later", len(failedChannels)) // Message for client, not logger
			} else {
				bizErr.Error.Message = "The current group load is saturated, please try again later"
			}
		}

		// BUG: bizErr is in race condition
		bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
		c.JSON(bizErr.StatusCode, gin.H{
			"error": bizErr.Error,
		})
		if shouldDebugLog {
			rcontroller.LogClientResponse(c, "client error response sent")
		}
	}
}

// shouldRetry returns nil if should retry, otherwise returns error
func shouldRetry(c *gin.Context, statusCode int, rawErr error) error {
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

	return nil
}

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
	if t == "authentication_error" || t == "permission_error" || t == "insufficient_quota" || t == "forbidden" {
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

// Helper function to get channel IDs from failed channels map for debugging
func getChannelIds(failedChannels map[int]bool) []int {
	var ids []int
	for id := range failedChannels {
		ids = append(ids, id)
	}
	return ids
}

// Helper function to check and log database suspension status for debugging
// Only performs expensive queries when debug logging is enabled
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
		lg.Error("Failed to check suspension status", zap.Error(err))
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

func processChannelRelayError(ctx context.Context, userId int, channelId int, channelName string, group string, originalModel string, err model.ErrorWithStatusCode) {
	// Always use a local logger variable
	lg := gmw.GetLogger(ctx)

	// Downgrade to WARN for client-side cancellations/timeouts to avoid noisy alerts
	if isClientContextCancel(err.StatusCode, err.RawError) {
		lg.Warn("relay aborted by client (context canceled/deadline)",
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
			zap.Int("user_id", userId),
			zap.String("group", group),
			zap.String("model", originalModel),
			zap.Error(err.RawError))
	} else {
		lg.Error("relay error",
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
			zap.Int("user_id", userId),
			zap.String("group", group),
			zap.String("model", originalModel),
			zap.Error(err.RawError))
	}

	// Handle 400 errors differently - they are client request issues, not channel problems
	if err.StatusCode == http.StatusBadRequest {
		// For 400 errors, log but don't disable channel or suspend abilities
		// These are typically schema validation errors or malformed requests
		lg.Info("client request error (400) for channel - not disabling channel as this is not a channel issue",
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
		)
		// Still emit failure for monitoring purposes, but don't disable the channel
		monitor.Emit(channelId, false)
		return
	}

	if err.StatusCode == http.StatusTooManyRequests {
		// For 429, we will suspend the specific model for a while
		lg.Info("suspending model due to rate limit",
			zap.String("model", originalModel),
			zap.String("group", group),
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
		)
		if suspendErr := dbmodel.SuspendAbility(ctx,
			group, originalModel, channelId,
			config.ChannelSuspendSecondsFor429); suspendErr != nil {
			lg.Error("failed to suspend ability for channel",
				zap.Int("channel_id", channelId),
				zap.String("model", originalModel),
				zap.String("group", group),
				zap.Error(errors.Wrap(suspendErr, "suspend ability failed")))
		}
		monitor.Emit(channelId, false)
		return
	}

	// context cancel or deadline exceeded - likely user aborted or timeout.
	// Detect via status or RawError classification; avoid suspending/disabling.
	if err.StatusCode == http.StatusRequestTimeout || (err.RawError != nil && (errors.Is(err.RawError, context.Canceled) || errors.Is(err.RawError, context.DeadlineExceeded))) {
		monitor.Emit(channelId, false)
		return
	}

	// 413 capacity issues: do not suspend; rely on retry selection to seek larger max_tokens
	if err.StatusCode == http.StatusRequestEntityTooLarge {
		monitor.Emit(channelId, false)
		return
	}

	// 5xx or network-type server errors -> suspend ability briefly
	if err.StatusCode >= 500 && err.StatusCode <= 599 {
		lg.Info("suspending model due to server error",
			zap.String("model", originalModel),
			zap.String("group", group),
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
			zap.Int("status_code", err.StatusCode),
		)
		if suspendErr := dbmodel.SuspendAbility(ctx, group, originalModel, channelId, config.ChannelSuspendSecondsFor5XX); suspendErr != nil {
			lg.Error("failed to suspend ability for 5xx", zap.Error(errors.Wrap(suspendErr, "suspend ability failed")))
		}
		// Do not immediately auto-disable; transient
		monitor.Emit(channelId, false)
		return
	}

	// Auth/permission/quota errors (401/403 or vendor-indicated) -> suspend ability; escalate to auto-disable only if fatal
	if err.StatusCode == http.StatusUnauthorized || err.StatusCode == http.StatusForbidden || classifyAuthLike(&err) {
		lg.Info("auth/permission issue, suspending ability",
			zap.String("model", originalModel),
			zap.String("group", group),
			zap.Int("channel_id", channelId),
			zap.String("channel_name", channelName),
		)
		if suspendErr := dbmodel.SuspendAbility(ctx, group, originalModel, channelId, config.ChannelSuspendSecondsForAuth); suspendErr != nil {
			lg.Error("failed to suspend ability for auth/permission", zap.Error(errors.Wrap(suspendErr, "suspend ability failed")))
		}

		if monitor.ShouldDisableChannel(&err.Error, err.StatusCode) {
			monitor.DisableChannel(channelId, channelName, err.Message)
		} else {
			monitor.Emit(channelId, false)
		}
		return
	}

	// Default: not fatal -> record failure only. If fatal per policy, auto-disable.
	if monitor.ShouldDisableChannel(&err.Error, err.StatusCode) {
		monitor.DisableChannel(channelId, channelName, err.Message)
	} else {
		monitor.Emit(channelId, false)
	}
}

func RelayNotImplemented(c *gin.Context) {
	msg := "API not implemented"
	errObj := model.Error{
		Message:  msg,
		Type:     "one_api_error",
		Param:    "",
		Code:     "api_not_implemented",
		RawError: errors.New(msg),
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": errObj,
	})
}

func RelayNotFound(c *gin.Context) {
	msg := fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path)
	errObj := model.Error{
		Message:  msg,
		Type:     "invalid_request_error",
		Param:    "",
		Code:     "",
		RawError: errors.New(msg),
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": errObj,
	})
}
