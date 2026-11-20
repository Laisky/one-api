package controller

import (
	"context"
	"net/http"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// isCapacityRejection reports whether a failed attempt proves the request is too
// large for the attempted channel's declared capacity: an HTTP 413 from the
// upstream. A 413 that one-api produced itself (IsOneAPIError), such as the
// Responses fallback's response_fallback_input_too_large, concerns a gateway
// limit, not the channel's max_tokens.
//
// Parameters:
//   - bizErr: the failed attempt; nil is not a rejection.
//
// Return values:
//   - bool: true for an upstream 413.
func isCapacityRejection(bizErr *model.ErrorWithStatusCode) bool {
	return bizErr != nil && bizErr.StatusCode == http.StatusRequestEntityTooLarge && !bizErr.IsOneAPIError()
}

// attemptChannel returns the channel the current attempt was routed to, as
// SetupContextForSelectedChannel recorded it, or nil when none is recorded.
//
// Parameters:
//   - c: the request context.
//
// Return values:
//   - *dbmodel.Channel: the routed channel snapshot, or nil.
func attemptChannel(c *gin.Context) *dbmodel.Channel {
	channel, _ := c.Get(ctxkey.ChannelModel)
	routed, _ := channel.(*dbmodel.Channel)
	return routed
}

// capacityRetryBudget returns the retry budget after an initial HTTP 413: one
// attempt for every untried channel of the routing pool. The count comes from
// dbmodel.CountRetryCandidates, which includes channels cooling down at this
// moment on both routing paths. It is only a ceiling: each retry selection
// enforces both cooldown deadlines and the capacity floor and ends the walk when
// nothing is eligible, so a cooling channel is never dispatched, but one whose
// cooldown ends while a later attempt is in flight keeps its slot. When the pool
// cannot be counted, one retry is allowed.
//
// Parameters:
//   - ctx: lifecycle scope for the database count on the database routing path.
//   - lg: the request-scoped logger.
//   - group, routedModel: the routing pool; routedModel is the caller-facing name.
//   - failed: channel IDs already tried, including the initial one.
//
// Return values:
//   - int: the retry budget to use.
func capacityRetryBudget(ctx context.Context, lg glog.Logger, group, routedModel string, failed map[int]bool) int {
	remaining, err := dbmodel.CountRetryCandidates(ctx, group, routedModel, failed)
	if err != nil {
		lg.Warn("413 error detected, failed to count the remaining channels; allowing one retry",
			zap.Int("retry_attempts", 1),
			zap.Error(err),
		)
		return 1
	}
	lg.Info("413 error detected, increasing retry attempts to exhaust alternative channels",
		zap.Int("retry_attempts", remaining),
	)
	return remaining
}

// selectCapacityRetryChannel picks the channel for the next retry attempt from
// the channels serving (group, originalModel) that are not in failedChannels,
// always from the highest remaining priority tier (see selectRetryChannel), and
// skips channels proven too small by capacityFloor.
//
// Parameters:
//   - ctx: request scope for logging and database reads.
//   - group, originalModel: the routing snapshot taken before the first attempt.
//   - failedChannels: channel IDs already tried or skipped.
//   - policy: the selection policy; it only labels the error.
//   - capacityFloor: the largest declared max_tokens that answered an upstream
//     413 for this request, or dbmodel.NoCapacityFloor before any.
//
// Return values:
//   - *dbmodel.Channel: the selected channel.
//   - error: a wrapped error whose text still carries the selector's exhaustion
//     message, so isExpectedChannelSelectionExhaustedError can classify it.
func selectCapacityRetryChannel(ctx context.Context, group string, originalModel string, failedChannels map[int]bool, policy retrySelectionPolicy, capacityFloor int32) (*dbmodel.Channel, error) {
	channel, err := dbmodel.CacheGetRandomSatisfiedChannelAboveCapacityWithContext(ctx, group, originalModel, false, failedChannels, capacityFloor)
	if err != nil {
		return nil, errors.Wrapf(err, "select retry channel (policy=%s, excluded=%d, capacity_floor=%d)", policy, len(failedChannels), capacityFloor)
	}
	return channel, nil
}
