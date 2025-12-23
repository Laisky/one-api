package controller

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
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
	userBalance := float64(c.GetInt64(ctxkey.UserQuota)) // Assuming we can get user balance from context
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

// Global instance
var PrometheusMonitor = &PrometheusRelayMonitor{}
