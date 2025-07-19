package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/middleware"
	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	"github.com/songquanpeng/one-api/relay/controller"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

func relayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	var err *model.ErrorWithStatusCode
	switch relayMode {
	case relaymode.ImagesGenerations,
		relaymode.ImagesEdits:
		err = controller.RelayImageHelper(c, relayMode)
	case relaymode.AudioSpeech:
		fallthrough
	case relaymode.AudioTranslation:
		fallthrough
	case relaymode.AudioTranscription:
		err = controller.RelayAudioHelper(c, relayMode)
	case relaymode.Proxy:
		err = controller.RelayProxyHelper(c, relayMode)
	case relaymode.ResponseAPI:
		err = controller.RelayResponseAPIHelper(c)
	case relaymode.ClaudeMessages:
		err = controller.RelayClaudeMessagesHelper(c)
	default:
		err = controller.RelayTextHelper(c)
	}
	return err
}

func Relay(c *gin.Context) {
	ctx := c.Request.Context()
	relayMode := relaymode.GetByPath(c.Request.URL.Path)
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)

	// Start timing for Prometheus metrics
	startTime := time.Now()

	// Get metadata for monitoring
	relayMeta := meta.GetByContext(c)

	// Track channel request in flight
	PrometheusMonitor.RecordChannelRequest(relayMeta, startTime)

	bizErr := relayHelper(c, relayMode)
	if bizErr == nil {
		monitor.Emit(channelId, true)

		// Record successful relay request metrics
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, true, 0, 0, 0)
		return
	}
	lastFailedChannelId := channelId
	channelName := c.GetString(ctxkey.ChannelName)
	group := c.GetString(ctxkey.Group)
	originalModel := c.GetString(ctxkey.OriginalModel)
	go processChannelRelayError(ctx, userId, channelId, channelName, group, originalModel, *bizErr)

	// Record failed relay request metrics
	PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, false, 0, 0, 0)

	requestId := c.GetString(helper.RequestIdKey)
	retryTimes := config.RetryTimes
	if err := shouldRetry(c, bizErr.StatusCode); err != nil {
		logger.Errorf(ctx, "relay error happen, won't retry since of %v", err.Error())
		retryTimes = 0
	}

	// For 429 errors, increase retry attempts to exhaust all available channels
	// to avoid returning 429 to users when other channels might be available
	if bizErr.StatusCode == http.StatusTooManyRequests && retryTimes > 0 {
		// Try to get an estimate of available channels for this model/group
		// to increase retry attempts accordingly
		retryTimes = retryTimes * 2 // Increase retry attempts for 429 errors
		logger.Infof(ctx, "429 error detected, increasing retry attempts to %d to exhaust alternative channels", retryTimes)
	}

	// For 413 errors, increase retry attempts to exhaust all available channels
	// to avoid returning 413 to users when other channels might be available
	if bizErr.StatusCode == http.StatusRequestEntityTooLarge {
		// Get the total number of channels for this model/group
		// and try to retry all channels
		channels, err := dbmodel.GetChannelsFromCache(group, originalModel)
		if err != nil {
			retryTimes = 1
			logger.Debugf(ctx, "413 error detected, Get channels from cache error: %v", err)
			logger.Warnf(ctx, "413 error detected, Failed to get total number of channels for a model/group from cache. increasing retry attempts to %d", retryTimes)
		} else {
			retryTimes = len(channels) - 1
			logger.Infof(ctx, "413 error detected, increasing retry attempts to %d to exhaust alternative channels", retryTimes)
		}
	}

	// Track failed channels to avoid retrying them, especially for 429 errors
	failedChannels := make(map[int]bool)
	failedChannels[lastFailedChannelId] = true

	// Debug logging to track channel exclusions (only when debug is enabled)
	if config.DebugEnabled {
		logger.Infof(ctx, "Debug: Starting retry logic - Initial failed channel: %d, Error: %d, Request ID: %s",
			lastFailedChannelId, bizErr.StatusCode, requestId)
	}

	// For 429 errors, we should try lower priority channels first
	// since the highest priority channel is rate limited
	shouldTryLowerPriorityFirst := bizErr.StatusCode == http.StatusTooManyRequests

	// For 413 errors, we should try Larger MaxTokens channels
	shouldTryLargerMaxTokensFirst := bizErr.StatusCode == http.StatusRequestEntityTooLarge

	for i := retryTimes; i > 0; i-- {
		var channel *dbmodel.Channel
		var err error

		// Try to find an available channel, preferring lower priority channels for 429 errors
		if config.DebugEnabled {
			logger.Infof(ctx, "Debug: Attempting retry %d, excluding channels: %v, shouldTryLowerPriorityFirst: %v, shouldTryLargerMaxTokensFirst: %v",
				retryTimes-i+1, getChannelIds(failedChannels), shouldTryLowerPriorityFirst, shouldTryLargerMaxTokensFirst)
		}

		if shouldTryLargerMaxTokensFirst {
			// For 413 errors, try larger max_tokens channels
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, true)
		} else if shouldTryLowerPriorityFirst {
			// For 429 errors, first try lower priority channels while excluding failed ones
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, true, failedChannels, false)
			if err != nil {
				// If no lower priority channels available, try highest priority channels (excluding failed ones)
				logger.Infof(ctx, "No lower priority channels available, trying highest priority channels, excluding: %v", getChannelIds(failedChannels))
				channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, false)
			}
		} else {
			// For non-429 errors, try highest priority first, then lower priority (excluding failed ones)
			channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, false, failedChannels, false)
			if err != nil {
				logger.Infof(ctx, "No highest priority channels available, trying lower priority channels, excluding: %v", getChannelIds(failedChannels))
				channel, err = dbmodel.CacheGetRandomSatisfiedChannelExcluding(group, originalModel, true, failedChannels, false)
			}
		}

		if err != nil {
			logger.Errorf(ctx, "CacheGetRandomSatisfiedChannelExcluding failed: %+v, excluding in-memory failed channels: %v, model: %s, group: %s",
				err, getChannelIds(failedChannels), originalModel, group)

			// Log database suspension status to help distinguish between in-memory and database exclusions
			// Only check the channels that were actually excluded in this request
			logChannelSuspensionStatus(ctx, group, originalModel, failedChannels)
			break
		}

		logger.Infof(ctx, "using channel #%d to retry (remain times %d)", channel.Id, i)
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
			logger.Infof(ctx, "Debug: Added channel %d to failed channels list, total failed channels: %v, request ID: %s",
				channelId, getChannelIds(failedChannels), requestId)
		}
		channelName := c.GetString(ctxkey.ChannelName)
		// Update group and originalModel potentially if changed by middleware, though unlikely for these.
		group = c.GetString(ctxkey.Group)
		originalModel = c.GetString(ctxkey.OriginalModel)
		go processChannelRelayError(ctx, userId, channelId, channelName, group, originalModel, *bizErr)
	}

	if bizErr != nil {
		if bizErr.StatusCode == http.StatusTooManyRequests {
			// Provide more specific messaging for 429 errors after exhausting retries
			if len(failedChannels) > 1 {
				bizErr.Error.Message = fmt.Sprintf("All available channels (%d) for this model are currently rate limited, please try again later", len(failedChannels))
			} else {
				bizErr.Error.Message = "The current group load is saturated, please try again later"
			}
		}

		// BUG: bizErr is in race condition
		bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
		c.JSON(bizErr.StatusCode, gin.H{
			"error": bizErr.Error,
		})
	}
}

// shouldRetry returns nil if should retry, otherwise returns error
func shouldRetry(c *gin.Context, statusCode int) error {
	if specificChannelId := c.GetInt(ctxkey.SpecificChannelId); specificChannelId != 0 {
		return errors.Errorf(
			"specific channel ID (%d) was provided, retry is unvailable",
			specificChannelId)
	}

	return nil
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

	var channelIds []int
	for id := range failedChannelIds {
		channelIds = append(channelIds, id)
	}

	var abilities []dbmodel.Ability
	now := time.Now()
	groupCol := "`group`"
	if common.UsingPostgreSQL {
		groupCol = `"group"`
	}

	err := dbmodel.DB.Where(groupCol+" = ? AND model = ? AND channel_id IN (?)", group, model, channelIds).Find(&abilities).Error
	if err != nil {
		logger.Errorf(ctx, "Failed to check suspension status: %v", err)
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
		logger.Infof(ctx, "Debug: Database suspension status - Suspended channels: %v, Available channels: %v, Model: %s, Group: %s",
			suspended, available, model, group)
	}
}

func processChannelRelayError(ctx context.Context, userId int, channelId int, channelName string, group string, originalModel string, err model.ErrorWithStatusCode) {
	logger.Errorf(ctx, "relay error (channel id %d, name %s, user_id %d, group: %s, model: %s): %s", channelId, channelName, userId, group, originalModel, err.Message)

	// Handle 400 errors differently - they are client request issues, not channel problems
	if err.StatusCode == http.StatusBadRequest {
		// For 400 errors, log but don't disable channel or suspend abilities
		// These are typically schema validation errors or malformed requests
		logger.Infof(ctx, "client request error (400) for channel %d (%s) - not disabling channel as this is not a channel issue", channelId, channelName)
		// Still emit failure for monitoring purposes, but don't disable the channel
		monitor.Emit(channelId, false)
		return
	}

	if err.StatusCode == http.StatusTooManyRequests {
		// For 429, we will suspend the specific model for a while
		logger.Infof(ctx, "suspending model %s in group %s on channel %d (%s) due to rate limit", originalModel, group, channelId, channelName)
		if suspendErr := dbmodel.SuspendAbility(ctx,
			group, originalModel, channelId,
			config.ChannelSuspendSecondsFor429); suspendErr != nil {
			logger.Errorf(ctx, "failed to suspend ability for channel %d, model %s, group %s: %v", channelId, originalModel, group, errors.Wrap(suspendErr, "suspend ability failed"))
		}
	}

	// Only disable channel for server errors (5xx) or specific client errors that indicate channel issues
	// 400 errors are client request problems and should not disable channels
	if monitor.ShouldDisableChannel(&err.Error, err.StatusCode) {
		monitor.DisableChannel(channelId, channelName, err.Message)
	} else {
		monitor.Emit(channelId, false)
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := model.Error{
		Message: "API not implemented",
		Type:    "one_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := model.Error{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}
