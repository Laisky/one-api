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
	applog "github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/monitor"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/channeltype"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// https://platform.openai.com/docs/api-reference/chat

// relayHelperForTest, when non-nil, replaces the post-routing adaptor dispatch so
// tests can script every compatible upstream attempt without a real adaptor and
// observe the exact channel order the retry loop walks.
var relayHelperForTest func(c *gin.Context, relayMode int) *model.ErrorWithStatusCode

// invokeRelayHelper dispatches one relay attempt through transport validation and
// provider handling. Parameters: c carries the request and relayMode selects the
// endpoint family. Returns: a normalized relay error or nil on success.
func invokeRelayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	// Actual attempts cannot inherit an earlier wire error. A skipped transport
	// candidate is not a provider attempt: Relay may select lastDispatchErr if
	// only such candidates remain, so keep its matching capture available.
	previous, _ := c.Get(ctxkey.ClaudeNativeErrorResponse)
	c.Set(ctxkey.ClaudeNativeErrorResponse, nil)
	// Observe the downstream writer for this attempt so a failed delivery is
	// reported and attributed to the gateway whatever the adaptor returned.
	tracker, releaseTracker := trackDelivery(c)
	failure := attributeDeliveryFailure(c, tracker, relayHelper(c, relayMode))
	releaseTracker()
	if failure != nil && adaptor.IsRESTTransportMismatch(failure.RawError) {
		c.Set(ctxkey.ClaudeNativeErrorResponse, previous)
	}
	return failure
}

// relayHelper dispatches the selected API format and returns its relay error.
func relayHelper(c *gin.Context, relayMode int) *model.ErrorWithStatusCode {
	// A Live-only model has no REST transport on a Google channel, and Google
	// itself answers such a request with HTTP 400. Deciding that here, per relay
	// attempt, keeps the selected channel in scope (a third-party REST bridge for
	// the same ID is still allowed). The typed routing mismatch below is retryable
	// so selection can exclude this incompatible channel without treating it as
	// evidence of channel health.
	if relayMode != relaymode.Realtime {
		if err := adaptor.ValidateRESTModelTransport(meta.GetByContext(c)); err != nil {
			return openai.ErrorWrapper(err, "unsupported_model_transport", http.StatusBadRequest)
		}
	}
	if relayHelperForTest != nil {
		return relayHelperForTest(c, relayMode)
	}
	var err *model.ErrorWithStatusCode
	switch relayMode {
	case relaymode.Realtime:
		// For Phase 1, route through text helper which will delegate to adaptor based on meta.Mode
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
	case relaymode.AsyncVideos:
		err = rcontroller.RelayAsyncVideoHelper(c, false)
	case relaymode.Videos:
		_, durable := channeltype.NativeAsyncVideoEndpoint(c.GetInt(ctxkey.Channel))
		if durable && c.Request.Method == http.MethodPost && c.Request.URL.Path == "/v1/videos/generations" {
			err = rcontroller.RelayAsyncVideoHelper(c, true)
		} else {
			err = rcontroller.RelayVideoHelper(c)
		}
	case relaymode.OCR:
		err = rcontroller.RelayOCRHelper(c)
	case relaymode.VoiceClone:
		err = rcontroller.RelayVoiceCloneHelper(c)
	default:
		err = rcontroller.RelayTextHelper(c)
	}
	return err
}

// Relay executes a relay request and any eligible cross-channel retries. It
// stops retrying when the current attempt's billing state indicates that replay
// could duplicate a paid upstream request, and writes the final API error.
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
	defer PrometheusMonitor.RecordChannelRequest(relayMeta)()

	// Snapshot the routing identity before any attempt runs. Some adaptors (AWS
	// Bedrock, Vertex) overwrite ctxkey.RequestModel with the channel-mapped
	// upstream id while converting the request; retry selection, ability
	// cooldowns and success resets must keep using the routed model, whose
	// abilities exist, or the retry walk finds no channel and the outcome
	// updates no ability.
	group := c.GetString(ctxkey.Group)
	originalModel := c.GetString(ctxkey.RequestModel)
	observation := &relayOutcomeObservation{retryOutcome: "not_needed"}
	// Derive completion from the monotonic clock: UTC() strips the monotonic
	// reading, and a wall-clock step must not make an outcome finish before it
	// started (the cooldown writer rejects such timestamps).
	attemptRevision := relayAttemptUpstreamRevision(c)
	attemptClock := time.Now()
	attemptStarted := attemptClock.UTC()
	bizErr := observation.invoke(c, relayMode)
	attemptCompleted := attemptStarted.Add(time.Since(attemptClock))
	defer func() { observation.failure = bizErr; observation.logFinal(c) }()
	if bizErr == nil {
		if c.GetString(asyncvideo.DurableTaskKey) != "" {
			PrometheusMonitor.RecordRelayRequest(c, relayMeta, startTime, c.Writer.Status() < 400, 0, 0, 0)
			return
		}
		recordRelayCooldownSuccess(ctx, c, relayMode, group, originalModel, attemptRevision, attemptStarted, attemptCompleted)
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
	tokenId := c.GetInt(ctxkey.TokenId)
	actualModel := relayMeta.ActualModelName
	requestURL := c.Request.URL.String()
	// Ensure channel error processing is completed during graceful drain
	goProcessChannelRelayError(ctx, bizErr, processChannelRelayErrorParams{
		StartedAt:     attemptStarted,
		CompletedAt:   attemptCompleted,
		ReadOnly:      isReadOnlyRelayRequest(c),
		Revision:      attemptRevision,
		Attempt:       observation.attempts,
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

	retryTimes := max(config.RetryTimes, 0)
	// Authorization and retry budget are different decisions. Capacity recovery
	// may increase an allowed budget, but must never undo a safety veto.
	retryAllowed := true
	lastDispatchErr := bizErr
	if isLocalDispatchRefusal(bizErr.RawError) {
		// No provider attempt was made. Reserve the initial dispatch, not an
		// expanded budget of paid retries proportional to the channel count.
		retryTimes++
		lastDispatchErr = nil
	}
	retryableClientError, retryableClientReason := classifyRetryableUpstreamClientError(bizErr)
	if err := shouldRetry(c, bizErr); err != nil {
		if retryableClientError && !videoReplayUnsafe(c) {
			lg.Debug("retryable upstream client error detected; keeping retry logic enabled",
				append(relayErrorMetadataLogFields(bizErr),
					zap.String("retry_reason", retryableClientReason),
				)...,
			)
		} else {
			errorMessagePreview := strings.TrimSpace(bizErr.Message)
			if bizErr.MessageContainsRawBody {
				errorMessagePreview = applog.Redact(errorMessagePreview)
			} else if len(errorMessagePreview) > 240 {
				errorMessagePreview = errorMessagePreview[:240] + "..."
			}
			relayLogParams := processChannelRelayErrorParams{
				Attempt:       observation.attempts,
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
				append(relayErrorMetadataLogFields(bizErr),
					zap.String("error_message_preview", errorMessagePreview),
				)...,
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
			retryAllowed = false
			observation.retryOutcome = "policy_veto"
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

	// Track failed channels to avoid retrying them, especially for 429 errors
	failedChannels := make(map[int]bool)
	failedChannels[lastFailedChannelId] = true

	// For 413 errors, increase retry attempts to exhaust all available channels
	// to avoid returning 413 to users when other channels might be available
	if bizErr.StatusCode == http.StatusRequestEntityTooLarge && retryAllowed {
		// Count every untried channel of the pool, cooling ones included, rather
		// than subtracting one from the routable pool: a concurrent cooldown may
		// already have hidden the failed channel, and a cooldown ending mid-walk
		// must not lose its slot (see capacityRetryBudget).
		retryTimes = capacityRetryBudget(gmw.Ctx(c), lg, group, originalModel, failedChannels)
	}

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

	// The selection policy starts from the initial failure: an upstream 413 seeks
	// channels with a larger max_tokens, everything else (including 429 and a
	// local one-api 413) walks the untried channels strictly by descending
	// priority. See selectRetryChannel for why the 429 case must NOT ask the
	// selector to skip a tier.
	selectionPolicy := retrySelectionPolicyFor(bizErr.StatusCode)
	// capacityFloor is the largest declared max_tokens of a channel whose upstream
	// answered 413 for this request. Only an upstream 413 is capacity evidence: a
	// channel that failed otherwise, was skipped without being tried, or was
	// refused by one-api itself says nothing about whether a channel of its size
	// can serve the request. Once positive, every later selection skips channels
	// declaring no more than it, for the remainder of the request.
	capacityFloor := dbmodel.NoCapacityFloor
	if isCapacityRejection(bizErr) {
		capacityFloor = dbmodel.RaiseCapacityFloor(capacityFloor, attemptChannel(c), originalModel)
	} else if selectionPolicy == retrySelectLargerMaxTokens {
		selectionPolicy = retrySelectStrictPriority
	}

	// For 5xx/server transient errors, avoid reusing the same ability first, probe within tier
	isServerTransient := bizErr.StatusCode >= 500 && bizErr.StatusCode <= 599

	for i := retryTimes; i > 0 && retryAllowed && !videoReplayUnsafe(c) && rcontroller.BillingAllowsRetry(c) &&
		c.GetInt(ctxkey.SpecificChannelId) == 0 && c.Request.Context().Err() == nil; i-- {
		var channel *dbmodel.Channel
		var err error

		// Try to find an available channel among the ones not yet tried.
		if config.DebugEnabled {
			lg.Info("Debug: Attempting retry",
				zap.Int("retry_attempt", retryTimes-i+1),
				dbmodel.ChannelRefsField("excluded_channels", getChannelIds(failedChannels)),
				zap.String("retry_selection_policy", string(selectionPolicy)),
				zap.Int32("retry_capacity_floor", capacityFloor),
				zap.Bool("server_transient", isServerTransient))
		}

		channel, err = selectCapacityRetryChannel(gmw.Ctx(c), group, originalModel, failedChannels, selectionPolicy, capacityFloor)
		if err != nil {
			relayLogParams := processChannelRelayErrorParams{
				Attempt:       observation.attempts,
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
				zap.Int32("retry_capacity_floor", capacityFloor),
				zap.Bool("server_transient", isServerTransient),
				zap.String("log_stage", "retry_decision"),
			)
			if isExpectedChannelSelectionExhaustedError(err) {
				observation.retryOutcome = "no_alternative"
				lg.Warn("relay retry exhausted: no alternative channel available",
					append(selectionFields, zap.String("selection_error", err.Error()), zap.String("retry_outcome", "no_alternative"))...,
				)
			} else {
				observation.retryOutcome = "selection_failed"
				lg.Error("relay retry channel selection failed",
					append(selectionFields, zap.Error(err), zap.String("retry_outcome", "selection_failed"))...,
				)
			}

			// Log database suspension status to help distinguish between in-memory and database exclusions
			// Only check the channels that were actually excluded in this request
			logChannelSuspensionStatus(ctx, group, originalModel, failedChannels)
			break
		}

		if !middleware.ChannelSupportsEndpointForRequest(c, channel, relayMode) {
			failedChannels[channel.Id] = true
			// Excluding an incompatible channel does not consume a paid retry attempt.
			i++
			continue
		}
		// channel is a DIFFERENT channel than the one bound onto lg, so name it
		// explicitly under its own key instead of shadowing the bound channel_id.
		lg.Info("using channel to retry",
			zap.String("log_stage", "retry_decision"), zap.String("retry_outcome", "selected"), zap.Int("relay_next_attempt", observation.attempts+1),
			zap.String("retry_channel", channel.Ref().String()),
			zap.Int64("retry_channel_priority", channel.GetPriority()),
			zap.String("retry_selection_policy", string(selectionPolicy)),
			zap.Int32("retry_capacity_floor", capacityFloor),
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
		attemptRevision = relayAttemptUpstreamRevision(c)
		retryStartTime := time.Now()
		retryMeta := meta.GetByContext(c)

		attemptStarted = retryStartTime.UTC()
		bizErr = observation.invoke(c, relayMode)
		attemptCompleted = attemptStarted.Add(time.Since(retryStartTime))
		if bizErr == nil {
			recordRelayCooldownSuccess(ctx, c, relayMode, group, originalModel, attemptRevision, attemptStarted, attemptCompleted)
			// Record successful retry
			PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, true, 0, 0, 0)
			return
		}

		// Record failed retry
		PrometheusMonitor.RecordRelayRequest(c, retryMeta, retryStartTime, false, 0, 0, 0)

		channelId = c.GetInt(ctxkey.ChannelId)
		failedChannels[channelId] = true // Track this failed channel
		lastFailedChannelId = channelId
		if isCapacityRejection(bizErr) {
			// Capacity evidence restricts the rest of the walk, whatever the
			// initial failure was.
			capacityFloor = dbmodel.RaiseCapacityFloor(capacityFloor, channel, originalModel)
			selectionPolicy = retrySelectLargerMaxTokens
		}

		// Debug logging to track which channels are being added to failed list (only when debug is enabled)
		if config.DebugEnabled {
			lg.Info("Debug: Added channel to failed channels list",
				dbmodel.ChannelRefsField("total_failed_channels", getChannelIds(failedChannels)))
		}
		channelName = c.GetString(ctxkey.ChannelName)
		// group and originalModel stay the routing snapshot taken before the first
		// attempt; an adaptor may have rewritten ctxkey.RequestModel meanwhile.
		// Get updated actual model from retry meta
		retryActualModel := retryMeta.ActualModelName
		actualModel = retryActualModel
		goProcessChannelRelayError(ctx, bizErr, processChannelRelayErrorParams{
			StartedAt:     attemptStarted,
			CompletedAt:   attemptCompleted,
			ReadOnly:      isReadOnlyRelayRequest(c),
			Revision:      attemptRevision,
			Attempt:       observation.attempts,
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
		if isLocalDispatchRefusal(bizErr.RawError) {
			// Exclusions make this finite. Local incompatibility or a missing
			// channel tariff consumes neither a provider call nor its retry slot,
			// wherever it occurs in the pool.
			i++
			continue
		}
		lastDispatchErr = bizErr
		if stopErr := shouldRetry(c, bizErr); stopErr != nil {
			// Apply the existing transient-4xx exception on every real attempt,
			// but never override cancellation or a terminal provider/client error.
			if isRetryableUpstreamClientError(bizErr) && !videoReplayUnsafe(c) &&
				!errors.Is(bizErr.RawError, context.Canceled) &&
				!errors.Is(bizErr.RawError, context.DeadlineExceeded) {
				continue
			}
			observation.retryOutcome = "policy_veto"
			lg.Debug("relay retry stopped after provider failure", zap.String("retry_skip_reason", stopErr.Error()))
			break
		}
	}

	if observation.retryOutcome == "budget_exhausted" {
		observation.retryOutcome = relayObservedRetryStop(c, retryAllowed)
	}

	// An exhausted local candidate (transport mismatch or missing tariff) is not
	// evidence about the request when a provider was actually tried: answer the
	// last real dispatch failure instead.
	if bizErr != nil && isLocalDispatchRefusal(bizErr.RawError) && lastDispatchErr != nil {
		bizErr = lastDispatchErr
	}
	if bizErr != nil {
		// No further attempt will run: publish a retained reservation of the
		// final failed attempt so the debit is visible in every ledger.
		rcontroller.SettleTerminalRetainedReservation(c)
		// Every attempted candidate was refused before any upstream contact
		// because its channel lacks a required tariff (a real dispatch failure
		// would have replaced bizErr above). Answer the billing-configuration
		// contract shared with image requests, not a generic dispatch failure.
		if adaptor.IsRequestPricingNotConfigured(bizErr.RawError) {
			bizErr.StatusCode = http.StatusServiceUnavailable
			bizErr.Error.Code = "model_pricing_not_configured"
		}
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
		observation.errorResponseWritten = writeRelayFinalError(c, bizErr)
		if observation.errorResponseWritten && shouldDebugLog {
			rcontroller.LogClientResponse(c, "client error response sent")
		}
	}
}

// isLocalDispatchRefusal reports whether err refused an attempt locally,
// before any provider contact: the selected channel cannot use this REST
// transport, or it lacks the tariff its provider requires. Such refusals spend
// no provider call, so they never consume a paid retry slot and never mask a
// real dispatch failure. Parameters: err is the attempt's raw error. Returns:
// true for a local refusal.
func isLocalDispatchRefusal(err error) bool {
	return adaptor.IsRESTTransportMismatch(err) || adaptor.IsRequestPricingNotConfigured(err)
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
	// retrySelectLargerMaxTokens is the 413 recovery policy: the strict priority
	// walk restricted to channels that may serve a request that channels declaring
	// up to the capacity floor already rejected with an upstream 413 -- channels
	// declaring a larger max_tokens, or none. It applies from the first upstream
	// 413 of the request, whatever the initial failure was.
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
// the tier boundary AFTER exclusions, so the "highest remaining tier" is precisely
// the next tier the operator wants tried. Asking to skip that tier (ignoreFirstPriority=true,
// as the 429 path used to) double-skips: with A(10) rate limited and excluded, the highest
// remaining tier is B(5), which was then skipped in favour of C(0), producing A → C → B.
//
// It applies no capacity requirement; the retry loop passes its upstream-413
// evidence through selectCapacityRetryChannel, which this delegates to.
//
// Returns the selected channel, or a wrapped error whose text still carries the
// selector's "no channels available" / "after exclusions" message so
// isExpectedChannelSelectionExhaustedError can classify exhaustion.
func selectRetryChannel(ctx context.Context, group string, originalModel string, failedChannels map[int]bool, policy retrySelectionPolicy) (*dbmodel.Channel, error) {
	return selectCapacityRetryChannel(ctx, group, originalModel, failedChannels, policy, dbmodel.NoCapacityFloor)
}

// RelayNotImplemented writes the unsupported-endpoint response to the request context.
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

// RelayNotFound writes the unknown-endpoint response to the request context.
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
