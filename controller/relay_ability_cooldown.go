package controller

import (
	"context"
	"net/http"
	"os"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/relaymode"
)

// suspendRelayAbility records the adaptive event for a proven upstream response.
// Explicit legacy duration settings and failures lacking upstream provenance
// retain the existing fixed suspension behavior. It returns the selected delay,
// whether a transition was written, and any database error.
func suspendRelayAbility(ctx context.Context, p processChannelRelayErrorParams, override string, fixed time.Duration) (time.Duration, bool, error) {
	if _, configured := os.LookupEnv(override); configured || p.ReadOnly || p.StartedAt.IsZero() || p.CompletedAt.IsZero() || !p.Err.FromUpstream {
		err := dbmodel.SuspendAbility(ctx, p.Group, p.OriginalModel, p.ChannelId, fixed)
		return fixed, err == nil, err
	}
	duration, err := dbmodel.RecordAbilityCooldownFailureAtRevision(ctx, p.Group, p.OriginalModel, p.ChannelId, p.Revision, p.StartedAt, p.CompletedAt)
	return duration, duration > 0 && err == nil, err
}

// recordRelayCooldownSuccess clears the adaptive streak after successful
// delivery of an actual provider attempt; local replies and async task
// acknowledgments do not constitute recovery. group and name are the routing
// snapshot Relay took before the first attempt (an adaptor may rewrite
// ctxkey.RequestModel to a mapped upstream id); the channel is the one that
// served this attempt. revision is the routed channel snapshot's upstream
// revision, and started and completed bound the attempt in UTC.
func recordRelayCooldownSuccess(ctx context.Context, c *gin.Context, mode int, group, name string, revision int64, started, completed time.Time) {
	if mode == relaymode.Proxy || mode == relaymode.Realtime || isReadOnlyRelayRequest(c) ||
		c.GetString(asyncvideo.DurableTaskKey) != "" ||
		!c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) || !c.Writer.Written() ||
		c.Writer.Status() < http.StatusOK || c.Writer.Status() >= http.StatusMultipleChoices {
		return
	}
	channelID := c.GetInt(ctxkey.ChannelId)
	if group == "" || name == "" || channelID <= 0 {
		return
	}
	if err := dbmodel.QueueAbilityCooldownRecovery(ctx, group, name, channelID, revision, started, completed); err != nil {
		gmw.GetLogger(ctx).Warn("failed to record upstream cooldown recovery", zap.Error(err))
	}
}

// isReadOnlyRelayRequest reports whether the client request is a read-only
// provider call (GET or HEAD), such as a video status poll or content download.
// Such outcomes describe the provider's read path, not generation capacity.
//
// Parameters:
//   - c: the request context; nil or a missing request is not read-only.
//
// Returns:
//   - bool: true for GET and HEAD requests.
func isReadOnlyRelayRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead
}

// relayAttemptUpstreamRevision returns the UpstreamRevision of the channel
// snapshot the current attempt was routed with, as bound by
// middleware.SetupContextForSelectedChannel, or
// dbmodel.AbilityUpstreamRevisionUnknown when no snapshot was bound.
//
// Parameters:
//   - c: the request context.
//
// Returns:
//   - int64: the routed snapshot's revision or the unknown sentinel.
func relayAttemptUpstreamRevision(c *gin.Context) int64 {
	value, ok := c.Get(ctxkey.ChannelUpstreamRevision)
	if !ok {
		return dbmodel.AbilityUpstreamRevisionUnknown
	}
	revision, ok := value.(int64)
	if !ok {
		return dbmodel.AbilityUpstreamRevisionUnknown
	}
	return revision
}
