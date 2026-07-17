package controller

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// PrometheusRelayMonitor provides Prometheus monitoring for relay operations
type PrometheusRelayMonitor struct{}

// RecordRelayRequest records metrics for a relay request
func (p *PrometheusRelayMonitor) RecordRelayRequest(c *gin.Context, meta *meta.Meta, startTime time.Time, success bool, promptTokens, completionTokens int, quotaUsed float64) {
	// Get user information
	userId := strconv.Itoa(meta.UserId)
	username := c.GetString(ctxkey.Username)
	if username == "" {
		username = "unknown"
	}
	group := meta.Group
	if group == "" {
		group = "default"
	}

	// Get channel information
	channelType := channeltype.IdToName(meta.ChannelType)

	// Get API format and type
	apiFormat := c.GetString(ctxkey.APIFormat)
	if apiFormat == "" {
		apiFormat = "unknown"
	}
	apiType := relaymode.String(meta.Mode)
	tokenId := strconv.Itoa(meta.TokenId)

	// Record relay metrics
	metrics.GlobalRecorder.RecordRelayRequest(startTime, meta.ChannelId, channelType, meta.ActualModelName, userId, group, tokenId, apiFormat, apiType, success, promptTokens, completionTokens, quotaUsed)

	// Record user metrics
	var userBalance float64
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			userBalance = float64(u.Quota)
		}
	}
	metrics.GlobalRecorder.RecordUserMetrics(userId, username, group, quotaUsed, promptTokens, completionTokens, userBalance)

	// Record model usage
	if success {
		latency := time.Since(startTime)
		metrics.GlobalRecorder.RecordModelUsage(meta.ActualModelName, channelType, latency)
	}
}

// RecordChannelRequest tracks channel-specific request metrics
func (p *PrometheusRelayMonitor) RecordChannelRequest(meta *meta.Meta, startTime time.Time) {
	channelIdStr := strconv.Itoa(meta.ChannelId)
	channelType := channeltype.IdToName(meta.ChannelType)
	channelName := "channel_" + channelIdStr // We might want to get actual channel name from DB

	// Track requests in flight
	metrics.GlobalRecorder.UpdateChannelRequestsInFlight(meta.ChannelId, channelName, channelType, 1)

	// We'll update this when the request completes
	go func() {
		// Wait for request to complete (this is a simplified approach)
		// In practice, you'd want to track this more precisely
		time.Sleep(time.Until(startTime.Add(time.Minute))) // Max wait of 1 minute
		metrics.GlobalRecorder.UpdateChannelRequestsInFlight(meta.ChannelId, channelName, channelType, -1)
	}()
}

// RecordError records an error metric
func (p *PrometheusRelayMonitor) RecordError(errorType, component string) {
	metrics.GlobalRecorder.RecordError(errorType, component)
}

// RecordUUIDBackfillRows records rows processed by one external UUID backfill batch.
//
// role, phase, target, and result must be compile-time registry constants; they
// become metric labels and must never carry an ID, UUID, DSN, or error message.
func (p *PrometheusRelayMonitor) RecordUUIDBackfillRows(role, phase, target, result string, count int) {
	metrics.GlobalRecorder.RecordUUIDBackfillRows(role, phase, target, result, count)
}

// UpdateUUIDBackfillBacklog publishes the last observed backlog for one target.
//
// role and target must be compile-time registry constants.
func (p *PrometheusRelayMonitor) UpdateUUIDBackfillBacklog(role, target string, backlog float64) {
	metrics.GlobalRecorder.UpdateUUIDBackfillBacklog(role, target, backlog)
}

// RecordUUIDBackfillCycle records one catch-up or finalizer cycle outcome and duration.
//
// role, mode, and result must be compile-time registry constants.
func (p *PrometheusRelayMonitor) RecordUUIDBackfillCycle(role, mode, result string, duration time.Duration) {
	metrics.GlobalRecorder.RecordUUIDBackfillCycle(role, mode, result, duration)
}

// RecordUUIDBackfillFinalizer records one finalizer attempt result for a database role.
//
// role and result must be compile-time registry constants.
func (p *PrometheusRelayMonitor) RecordUUIDBackfillFinalizer(role, result string) {
	metrics.GlobalRecorder.RecordUUIDBackfillFinalizer(role, result)
}

// UpdateCompactUUIDState publishes whether one state is the current state of a role.
//
// Callers set active=true for the current state and active=false for every
// other known state, so exactly one state per role/process reports 1.
//
// role and state must be compile-time registry constants; they become metric
// labels and must never carry an ID, UUID, DSN, or error message.
func (p *PrometheusRelayMonitor) UpdateCompactUUIDState(role, state string, active bool) {
	metrics.GlobalRecorder.UpdateCompactUUIDState(role, state, active)
}

// UpdateCompactUUIDBacklog publishes the last bounded gap/mismatch/blocker observation.
//
// The value is one bounded observation, never a claimed global total. role,
// target, and kind must be compile-time registry constants.
func (p *PrometheusRelayMonitor) UpdateCompactUUIDBacklog(role, target, kind string, rows float64) {
	metrics.GlobalRecorder.UpdateCompactUUIDBacklog(role, target, kind, rows)
}

// RecordCompactUUIDAction records one DDL, fill, validation, marker, audit, or repair outcome.
//
// role, action, and result must be compile-time registry constants.
func (p *PrometheusRelayMonitor) RecordCompactUUIDAction(role, action, result string) {
	metrics.GlobalRecorder.RecordCompactUUIDAction(role, action, result)
}

// RecordCompactUUIDLookupFallback records one compact UUID lookup fallback.
//
// role and reason must be compile-time registry constants.
func (p *PrometheusRelayMonitor) RecordCompactUUIDLookupFallback(role, reason string) {
	metrics.GlobalRecorder.RecordCompactUUIDLookupFallback(role, reason)
}

// UpdateCompactUUIDLastProgress publishes the UTC timestamp of the last durable progress.
//
// role must be a compile-time registry constant.
func (p *PrometheusRelayMonitor) UpdateCompactUUIDLastProgress(role string, unixTime float64) {
	metrics.GlobalRecorder.UpdateCompactUUIDLastProgress(role, unixTime)
}

// RecordCompactUUIDDuration records the duration of one compact UUID operation.
//
// role and operation must be compile-time registry constants.
func (p *PrometheusRelayMonitor) RecordCompactUUIDDuration(role, operation string, duration time.Duration) {
	metrics.GlobalRecorder.RecordCompactUUIDDuration(role, operation, duration)
}

// Global instance
var PrometheusMonitor = &PrometheusRelayMonitor{}
