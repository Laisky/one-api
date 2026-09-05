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
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/monitor"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

// relayHelperForTest, when non-nil, replaces relayHelper inside Relay so tests can
// script the upstream outcome of every attempt (initial and retries) without a real
// adaptor, and observe the exact channel order the retry loop walks.
var relayHelperForTest func(c *gin.Context, relayMode int) *model.ErrorWithStatusCode

// invokeRelayHelper dispatches one relay attempt to relayHelper, or to the test
// hook when one is installed.
func invokeRelayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	if relayHelperForTest != nil {
		return relayHelperForTest(c, relayMode)
	}
	return relayHelper(c, relayMode)
}

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
	case relaymode.Rerank:
		err = rcontroller.RelayRerankHelper(c)
	case relaymode.Videos:
		err = rcontroller.RelayVideoHelper(c)
	case relaymode.OCR:
		err = rcontroller.RelayOCRHelper(c)
	case relaymode.VoiceClone:
		err = rcontroller.RelayVoiceCloneHelper(c)
	default:
		err = rcontroller.RelayTextHelper(c)
	}
	return err
}

func Relay(c *gin.Context) {
	ctx := relayctx.Detach(c)
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
	// user/token/channel identity and request_id are already bound onto lg by
	// identity.Bind (middleware), so they must not be repeated here.
	lg.Debug("incoming relay request",
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.Int("relay_mode", relayMode),
		zap.String("content_type", c.GetHeader("Content-Type")),
		zap.Int64("content_length", c.Request.ContentLength),
	)

	// Get metadata for monitoring
	relayMeta := meta.GetByContext(c)
	requestId := c.GetString(helper.RequestIdKey)

	// Track channel request in flight
	PrometheusMonitor.RecordChannelRequest(relayMeta, startTime)

	bizErr := invokeRelayHelper(c, relayMode)
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
	tokenId := c.GetInt(ctxkey.TokenId)
	actualModel := relayMeta.ActualModelName
	requestURL := c.Request.URL.String()
	// Ensure channel error processing is completed during graceful drain
	goProcessChannelRelayError(ctx, bizErr, processChannelRelayErrorParams{
		RequestID:     requestId,
		UserId:        userId,
		TokenId:       tokenId,
		ChannelId:     channelId,
		ChannelName:   channelName,
		Group:         group,
		OriginalModel: originalModel,
		ActualModel:   actualModel,
		RequestURL:    requestURL,
	})

	// Record failed relay request metrics
	PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, false, 0, 0, 0)

	retryTimes := config.RetryTimes
	retryableClientError, retryableClientReason := classifyRetryableUpstreamClientError(bizErr)
	if err := shouldRetry(c, bizErr); err != nil {
		if retryableClientError {
			lg.Debug("retryable upstream client error detected; keeping retry logic enabled",
				zap.Int("status_code", bizErr.StatusCode),
				zap.String("error_type", string(bizErr.Type)),
				zap.String("error_code", strings.TrimSpace(fmt.Sprint(bizErr.Code))),
				zap.String("retry_reason", retryableClientReason),
			)
		} else {
			errorMessagePreview := strings.TrimSpace(bizErr.Message)
			if len(errorMessagePreview) > 240 {
				errorMessagePreview = errorMessagePreview[:240] + "..."
			}
			relayLogParams := processChannelRelayErrorParams{
				RequestID:     requestId,
				RequestURL:    requestURL,
				UserId:        userId,
				TokenId:       tokenId,
				ChannelId:     channelId,
				ChannelName:   channelName,
				Group:         group,
				OriginalModel: originalModel,
				ActualModel:   actualModel,
				Err:           *bizErr,
			}
			isUserSideRetrySkip := isClientContextCancel(bizErr.StatusCode, bizErr.RawError) ||
				errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
				isUserOriginatedRelayError(bizErr)
			lg.Debug("non-retry relay decision details",
				zap.Int("status_code", bizErr.StatusCode),
				zap.String("error_type", string(bizErr.Type)),
				zap.String("error_code", strings.TrimSpace(fmt.Sprint(bizErr.Code))),
				zap.String("error_message_preview", errorMessagePreview),
			)
			// retry_skip_reason already carries the decision text; zap.Error would add
			// one-api's own errors/v2 stack to a WARN line that reports a routing
			// decision, not a fault.
			lg.Warn("relay retry skipped after failure",
				appendRelayFailureFields(relayLogParams,
					stacklessErrorField(err),
					zap.Bool("user_originated", isUserSideRetrySkip),
					zap.String("retry_skip_reason", err.Error()),
				)...,
			)
			retryTimes = 0
		}
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
		// lastFailedChannelId is the request's own channel here, already bound onto lg.
		if retryTimes > 0 {
			lg.Info("Debug: Starting retry logic - Initial failed channel",
				zap.Int("error_code", bizErr.StatusCode),
			)
		} else {
			lg.Info("Debug: No retry will be attempted (retryTimes=0)",
				zap.Int("error_code", bizErr.StatusCode),
			)
		}
	}

	// The selection policy is fixed by the initial failure: 413 seeks channels with
	// a larger max_tokens, everything else (including 429) walks the untried
	// channels strictly by descending priority. See selectRetryChannel for why the
	// 429 case must NOT ask the selector to skip a tier.
	selectionPolicy := retrySelectionPolicyFor(bizErr.StatusCode)

	// For 5xx/server transient errors, avoid reusing the same ability first, probe within tier
	isServerTransient := bizErr.StatusCode >= 500 && bizErr.StatusCode <= 599

	for i := retryTimes; i > 0; i-- {
		var channel *dbmodel.Channel
		var err error

		// Try to find an available channel among the ones not yet tried.
		if config.DebugEnabled {
			lg.Info("Debug: Attempting retry",
				zap.Int("retry_attempt", retryTimes-i+1),
				dbmodel.ChannelRefsField("excluded_channels", getChannelIds(failedChannels)),
				zap.String("retry_selection_policy", string(selectionPolicy)),
				zap.Bool("server_transient", isServerTransient))
		}

		channel, err = selectRetryChannel(gmw.Ctx(c), group, originalModel, failedChannels, selectionPolicy)
		if err != nil {
			relayLogParams := processChannelRelayErrorParams{
				RequestID:     requestId,
				RequestURL:    requestURL,
				UserId:        userId,
				TokenId:       tokenId,
				ChannelId:     channelId,
				ChannelName:   channelName,
				Group:         group,
				OriginalModel: originalModel,
				ActualModel:   actualModel,
				Err:           *bizErr,
			}
			selectionFields := appendRelayFailureFields(relayLogParams,
				dbmodel.ChannelRefsField("excluded_channels", getChannelIds(failedChannels)),
				zap.Int("retry_attempt", retryTimes-i+1),
				zap.Int("remaining_attempts", i-1),
				zap.String("retry_selection_policy", string(selectionPolicy)),
				zap.Bool("server_transient", isServerTransient),
			)
			if isExpectedChannelSelectionExhaustedError(err) {
				lg.Warn("relay retry exhausted: no alternative channel available",
					append(selectionFields, zap.String("selection_error", err.Error()))...,
				)
			} else {
				lg.Error("relay retry channel selection failed",
					append(selectionFields, zap.Error(err))...,
				)
			}

			// Log database suspension status to help distinguish between in-memory and database exclusions
			// Only check the channels that were actually excluded in this request
			logChannelSuspensionStatus(ctx, group, originalModel, failedChannels)
			break
		}

		// channel is a DIFFERENT channel than the one bound onto lg, so name it
		// explicitly under its own key instead of shadowing the bound channel_id.
		lg.Info("using channel to retry",
			zap.String("retry_channel", channel.Ref().String()),
			zap.Int64("retry_channel_priority", channel.GetPriority()),
			zap.String("retry_selection_policy", string(selectionPolicy)),
			zap.Int("remaining_attempts", i),
		)
		// We have definitively decided to retry on a different channel. Refund and
		// clear the just-failed attempt's per-attempt billing state so the next
		// attempt's pre-consume/post-consume does not stack on top of the abandoned
		// attempt's (conservative-skip) outstanding pre-consume, which would double
		// charge the user. Terminal failures never reach this point.
		rcontroller.ResetPerAttemptBillingForRetry(ctx, c)
		middleware.SetupContextForSelectedChannel(c, channel, originalModel)
		// SetupContextForSelectedChannel re-binds the request identity onto a NEW
		// request-scoped logger, so refresh both the logger value and the detached
		// context; otherwise every later log line and the async error processor would
		// still be labelled with the first channel.
		lg = gmw.GetLogger(c)
		ctx = relayctx.Detach(c)
		requestBody, err := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		// Record retry attempt
		retryStartTime := time.Now()
		retryMeta := meta.GetByContext(c)

		bizErr = invokeRelayHelper(c, relayMode)
		if bizErr == nil {
			// Record successful retry
			PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, true, 0, 0, 0)
			return
		}

		// Record failed retry
		PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, false, 0, 0, 0)

		channelId = c.GetInt(ctxkey.ChannelId)
		failedChannels[channelId] = true // Track this failed channel
		lastFailedChannelId = channelId

		// Debug logging to track which channels are being added to failed list (only when debug is enabled)
		if config.DebugEnabled {
			// channelId is the currently bound channel; only the other failed ones need naming.
			lg.Info("Debug: Added channel to failed channels list",
				dbmodel.ChannelRefsField("total_failed_channels", getChannelIds(failedChannels)))
		}
		channelName = c.GetString(ctxkey.ChannelName)
		// Update group and originalModel potentially if changed by middleware, though unlikely for these.
		group = c.GetString(ctxkey.Group)
		originalModel = c.GetString(ctxkey.RequestModel)
		// Get updated actual model from retry meta
		retryActualModel := retryMeta.ActualModelName
		actualModel = retryActualModel
		goProcessChannelRelayError(ctx, bizErr, processChannelRelayErrorParams{
			RequestID:     requestId,
			UserId:        userId,
			TokenId:       tokenId,
			ChannelId:     channelId,
			ChannelName:   channelName,
			Group:         group,
			OriginalModel: originalModel,
			ActualModel:   retryActualModel,
			RequestURL:    requestURL,
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

		// Client-facing decoration. The async processChannelRelayError snapshots
		// *bizErr synchronously at spawn time (see goProcessChannelRelayError), so
		// rewriting Message here no longer races the error-processing goroutine.
		bizErr.Error.Message = helper.MessageWithRequestId(bizErr.Error.Message, requestId)
		c.JSON(bizErr.StatusCode, gin.H{
			"error": bizErr.Error,
		})
		if shouldDebugLog {
			rcontroller.LogClientResponse(c, "client error response sent")
		}
	}
}

// retrySelectionPolicy names how the relay retry loop picks the next channel
// after a failed attempt. It is fixed once, from the status of the initial
// failure, and logged as retry_selection_policy on every attempt.
type retrySelectionPolicy string

const (
	// retrySelectStrictPriority walks the channels not yet tried in descending
	// priority order: the highest tier that still holds an untried channel wins,
	// and a same-priority sibling is tried before any lower tier. Because every
	// failed channel is excluded, this already realises "move on to the next
	// tier" for a rate-limited top channel.
	retrySelectStrictPriority retrySelectionPolicy = "strict_priority"
	// retrySelectLargerMaxTokens is the 413 recovery policy: among the channels not
	// yet tried, only those whose configured max_tokens differs from every channel
	// that rejected the request as too large are eligible.
	retrySelectLargerMaxTokens retrySelectionPolicy = "larger_max_tokens"
)

// retrySelectionPolicyFor maps the initial failure status to the retry selection
// policy. 413 seeks larger max_tokens; every other retryable status, 429
// included, walks the remaining channels strictly by priority.
func retrySelectionPolicyFor(statusCode int) retrySelectionPolicy {
	if statusCode == http.StatusRequestEntityTooLarge {
		return retrySelectLargerMaxTokens
	}
	return retrySelectStrictPriority
}

// selectRetryChannel picks the channel for the next retry attempt from the
// channels serving (group, originalModel) that are not in failedChannels.
//
// The selector is always asked for the HIGHEST priority tier among the
// remaining candidates (ignoreFirstPriority=false). Both routing paths recompute
// the tier boundary AFTER exclusions, so once the failed channels are excluded
// the "highest remaining tier" is precisely the next tier the operator wants
// tried. Asking to skip that tier (ignoreFirstPriority=true, as the 429 path
// used to) double-skips: with A(10) rate limited and excluded, the highest
// remaining tier is B(5), which was then skipped in favour of C(0), producing
// A → C → B instead of A → B → C.
//
// Returns the selected channel, or a wrapped error whose text still carries the
// selector's "no channels available" / "after exclusions" message so
// isExpectedChannelSelectionExhaustedError can classify exhaustion.
func selectRetryChannel(ctx context.Context, group string, originalModel string, failedChannels map[int]bool, policy retrySelectionPolicy) (*dbmodel.Channel, error) {
	tryLargerMaxTokens := policy == retrySelectLargerMaxTokens
	channel, err := dbmodel.CacheGetRandomSatisfiedChannelExcludingWithContext(ctx, group, originalModel, false, failedChannels, tryLargerMaxTokens)
	if err != nil {
		return nil, errors.Wrapf(err, "select retry channel (policy=%s, excluded=%d)", policy, len(failedChannels))
	}
	return channel, nil
}

func RelayNotImplemented(c *gin.Context) {
	msg := "API not implemented"
	errObj := model.Error{
		Message:  msg,
		Type:     model.ErrorTypeOneAPI,
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
		Type:     model.ErrorTypeInvalidRequest,
		Param:    "",
		Code:     "",
		RawError: errors.New(msg),
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": errObj,
	})
}
