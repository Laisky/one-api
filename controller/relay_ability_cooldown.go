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
	if _, configured := os.LookupEnv(override); configured || p.StartedAt.IsZero() || p.CompletedAt.IsZero() || !p.Err.FromUpstream {
		err := dbmodel.SuspendAbility(ctx, p.Group, p.OriginalModel, p.ChannelId, fixed)
		return fixed, err == nil, err
	}
	duration, err := dbmodel.RecordAbilityCooldownFailure(ctx, p.Group, p.OriginalModel, p.ChannelId, p.StartedAt, p.CompletedAt)
	return duration, duration > 0 && err == nil, err
}

// recordRelayCooldownSuccess clears the adaptive streak after successful
// delivery of an actual provider attempt; local replies and async task
// acknowledgments do not constitute recovery.
func recordRelayCooldownSuccess(ctx context.Context, c *gin.Context, mode int, started, completed time.Time) {
	if mode == relaymode.Proxy || mode == relaymode.Realtime ||
		c.GetString(asyncvideo.DurableTaskKey) != "" ||
		!c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) || !c.Writer.Written() ||
		c.Writer.Status() < http.StatusOK || c.Writer.Status() >= http.StatusMultipleChoices {
		return
	}
	group, name, channelID := c.GetString(ctxkey.Group), c.GetString(ctxkey.RequestModel), c.GetInt(ctxkey.ChannelId)
	if group == "" || name == "" || channelID <= 0 {
		return
	}
	if err := dbmodel.ResetAbilityCooldown(ctx, group, name, channelID, started, completed); err != nil {
		gmw.GetLogger(ctx).Warn("failed to record upstream cooldown recovery", zap.Error(err))
	}
}
