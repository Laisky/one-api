package otel

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OtelRecorder implements the MetricsRecorder interface using OpenTelemetry
type OtelRecorder struct {
	meter metric.Meter

	// Relay metrics
	relayRequestDuration metric.Float64Histogram
	relayRequestsTotal   metric.Int64Counter
	relayTokensUsed      metric.Int64Counter
	relayQuotaUsed       metric.Float64Counter

	// HTTP metrics
	httpRequestDuration metric.Float64Histogram
	httpRequestsTotal   metric.Int64Counter
	httpActiveRequests  metric.Float64UpDownCounter

	// Channel metrics
	channelStatus           metric.Int64Gauge
	channelBalance          metric.Float64Gauge
	channelResponseTime     metric.Int64Gauge
	channelSuccessRate      metric.Float64Gauge
	channelRequestsInFlight metric.Float64UpDownCounter

	// User metrics
	userRequestsTotal metric.Int64Counter
	userQuotaUsed     metric.Float64Counter
	userTokensUsed    metric.Int64Counter
	userBalance       metric.Float64Gauge

	// Database metrics
	dbQueriesTotal metric.Int64Counter

	// Redis metrics
	redisCommandDuration metric.Float64Histogram
	redisCommandsTotal   metric.Int64Counter

	// Rate limit metrics
	rateLimitHits metric.Int64Counter

	// Error metrics
	errorsTotal metric.Int64Counter

	// Model metrics
	modelUsageDuration metric.Float64Histogram

	// Site-wide statistics (Dashboard)
	siteTotalQuota  metric.Int64Gauge
	siteUsedQuota   metric.Int64Gauge
	siteTotalUsers  metric.Int64Gauge
	siteActiveUsers metric.Int64Gauge
}

// NewOtelRecorder creates a new OtelRecorder
func NewOtelRecorder() (*OtelRecorder, error) {
	meter := otel.Meter("one-api")
	r := &OtelRecorder{meter: meter}

	var err error
	// Relay metrics
	if r.relayRequestDuration, err = meter.Float64Histogram("one_api_relay_request_duration_seconds", metric.WithDescription("Duration of API relay requests in seconds")); err != nil {
		return nil, err
	}
	if r.relayRequestsTotal, err = meter.Int64Counter("one_api_relay_requests_total", metric.WithDescription("Total number of API relay requests")); err != nil {
		return nil, err
	}
	if r.relayTokensUsed, err = meter.Int64Counter("one_api_relay_tokens_total", metric.WithDescription("Total number of tokens used in relay requests")); err != nil {
		return nil, err
	}
	if r.relayQuotaUsed, err = meter.Float64Counter("one_api_relay_quota_used_total", metric.WithDescription("Total quota used in relay requests")); err != nil {
		return nil, err
	}

	// HTTP metrics
	if r.httpRequestDuration, err = meter.Float64Histogram("one_api_http_request_duration_seconds", metric.WithDescription("Duration of HTTP requests in seconds")); err != nil {
		return nil, err
	}
	if r.httpRequestsTotal, err = meter.Int64Counter("one_api_http_requests_total", metric.WithDescription("Total number of HTTP requests")); err != nil {
		return nil, err
	}
	if r.httpActiveRequests, err = meter.Float64UpDownCounter("one_api_http_active_requests", metric.WithDescription("Number of active HTTP requests")); err != nil {
		return nil, err
	}

	// Channel metrics
	if r.channelStatus, err = meter.Int64Gauge("one_api_channel_status", metric.WithDescription("Channel status (1=enabled, 0=disabled, -1=auto_disabled)")); err != nil {
		return nil, err
	}
	if r.channelBalance, err = meter.Float64Gauge("one_api_channel_balance_usd", metric.WithDescription("Channel balance in USD")); err != nil {
		return nil, err
	}
	if r.channelResponseTime, err = meter.Int64Gauge("one_api_channel_response_time_ms", metric.WithDescription("Channel response time in milliseconds")); err != nil {
		return nil, err
	}
	if r.channelSuccessRate, err = meter.Float64Gauge("one_api_channel_success_rate", metric.WithDescription("Channel success rate (0-1)")); err != nil {
		return nil, err
	}
	if r.channelRequestsInFlight, err = meter.Float64UpDownCounter("one_api_channel_requests_in_flight", metric.WithDescription("Number of requests currently being processed by channel")); err != nil {
		return nil, err
	}

	// User metrics
	if r.userRequestsTotal, err = meter.Int64Counter("one_api_user_requests_total", metric.WithDescription("Total number of requests by user")); err != nil {
		return nil, err
	}
	if r.userQuotaUsed, err = meter.Float64Counter("one_api_user_quota_used_total", metric.WithDescription("Total quota used by user")); err != nil {
		return nil, err
	}
	if r.userTokensUsed, err = meter.Int64Counter("one_api_user_tokens_total", metric.WithDescription("Total tokens used by user")); err != nil {
		return nil, err
	}
	if r.userBalance, err = meter.Float64Gauge("one_api_user_balance", metric.WithDescription("User balance")); err != nil {
		return nil, err
	}

	// Database metrics
	if r.dbQueriesTotal, err = meter.Int64Counter("one_api_db_queries_total", metric.WithDescription("Total number of database queries")); err != nil {
		return nil, err
	}

	// Redis metrics
	if r.redisCommandDuration, err = meter.Float64Histogram("one_api_redis_command_duration_seconds", metric.WithDescription("Duration of Redis commands in seconds")); err != nil {
		return nil, err
	}
	if r.redisCommandsTotal, err = meter.Int64Counter("one_api_redis_commands_total", metric.WithDescription("Total number of Redis commands")); err != nil {
		return nil, err
	}

	// Rate limit metrics
	if r.rateLimitHits, err = meter.Int64Counter("one_api_rate_limit_hits_total", metric.WithDescription("Total number of rate limit hits")); err != nil {
		return nil, err
	}

	// Error metrics
	if r.errorsTotal, err = meter.Int64Counter("one_api_errors_total", metric.WithDescription("Total number of errors")); err != nil {
		return nil, err
	}

	// Model metrics
	if r.modelUsageDuration, err = meter.Float64Histogram("one_api_model_usage_duration_seconds", metric.WithDescription("Duration of model usage")); err != nil {
		return nil, err
	}

	// Site-wide statistics (Dashboard)
	if r.siteTotalQuota, err = meter.Int64Gauge("one_api_site_total_quota", metric.WithDescription("Total quota across all users")); err != nil {
		return nil, err
	}
	if r.siteUsedQuota, err = meter.Int64Gauge("one_api_site_used_quota", metric.WithDescription("Total used quota across all users")); err != nil {
		return nil, err
	}
	if r.siteTotalUsers, err = meter.Int64Gauge("one_api_site_total_users", metric.WithDescription("Total number of users")); err != nil {
		return nil, err
	}
	if r.siteActiveUsers, err = meter.Int64Gauge("one_api_site_active_users", metric.WithDescription("Number of active users")); err != nil {
		return nil, err
	}

	return r, nil
}

// RecordHTTPRequest records HTTP request metrics
func (r *OtelRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {
	ctx := context.Background()
	duration := time.Since(startTime).Seconds()
	attrs := []attribute.KeyValue{
		attribute.String("path", path),
		attribute.String("method", method),
		attribute.String("status_code", statusCode),
	}
	r.httpRequestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	r.httpRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordHTTPActiveRequest records active HTTP request metrics
func (r *OtelRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("path", path),
		attribute.String("method", method),
	}
	r.httpActiveRequests.Add(ctx, delta, metric.WithAttributes(attrs...))
}

// RecordRelayRequest records API relay request metrics
func (r *OtelRecorder) RecordRelayRequest(startTime time.Time, channelId int, channelType, model, userId, group, tokenId, apiFormat, apiType string, success bool, promptTokens, completionTokens int, quotaUsed float64) {
	ctx := context.Background()
	duration := time.Since(startTime).Seconds()
	channelIdStr := strconv.Itoa(channelId)
	successStr := strconv.FormatBool(success)

	attrs := []attribute.KeyValue{
		attribute.String("channel_id", channelIdStr),
		attribute.String("channel_type", channelType),
		attribute.String("model", model),
		attribute.String("user_id", userId),
		attribute.String("group", group),
		attribute.String("token_id", tokenId),
		attribute.String("api_format", apiFormat),
		attribute.String("api_type", apiType),
		attribute.String("success", successStr),
	}

	r.relayRequestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	r.relayRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	if promptTokens > 0 {
		promptAttrs := append(attrs, attribute.String("token_type", "prompt"))
		r.relayTokensUsed.Add(ctx, int64(promptTokens), metric.WithAttributes(promptAttrs...))
	}
	if completionTokens > 0 {
		completionAttrs := append(attrs, attribute.String("token_type", "completion"))
		r.relayTokensUsed.Add(ctx, int64(completionTokens), metric.WithAttributes(completionAttrs...))
	}
	if quotaUsed > 0 {
		r.relayQuotaUsed.Add(ctx, quotaUsed, metric.WithAttributes(attrs...))
	}
}

// UpdateChannelMetrics updates channel-related metrics
func (r *OtelRecorder) UpdateChannelMetrics(channelId int, channelName, channelType string, status int, balance float64, responseTimeMs int, successRate float64) {
	ctx := context.Background()
	channelIdStr := strconv.Itoa(channelId)
	attrs := []attribute.KeyValue{
		attribute.String("channel_id", channelIdStr),
		attribute.String("channel_name", channelName),
		attribute.String("channel_type", channelType),
	}

	r.channelStatus.Record(ctx, int64(status), metric.WithAttributes(attrs...))
	r.channelBalance.Record(ctx, balance, metric.WithAttributes(attrs...))
	r.channelResponseTime.Record(ctx, int64(responseTimeMs), metric.WithAttributes(attrs...))
	r.channelSuccessRate.Record(ctx, successRate, metric.WithAttributes(attrs...))
}

// UpdateChannelRequestsInFlight updates the number of in-flight requests for a channel
func (r *OtelRecorder) UpdateChannelRequestsInFlight(channelId int, channelName, channelType string, delta float64) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("channel_id", strconv.Itoa(channelId)),
		attribute.String("channel_name", channelName),
		attribute.String("channel_type", channelType),
	}
	r.channelRequestsInFlight.Add(ctx, delta, metric.WithAttributes(attrs...))
}

// RecordUserMetrics records user-specific metrics
func (r *OtelRecorder) RecordUserMetrics(userId, username, group string, quotaUsed float64, promptTokens, completionTokens int, balance float64) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("user_id", userId),
		attribute.String("username", username),
		attribute.String("group", group),
	}

	r.userRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	if quotaUsed > 0 {
		r.userQuotaUsed.Add(ctx, quotaUsed, metric.WithAttributes(attrs...))
	}
	if promptTokens > 0 {
		promptAttrs := append(attrs, attribute.String("token_type", "prompt"))
		r.userTokensUsed.Add(ctx, int64(promptTokens), metric.WithAttributes(promptAttrs...))
	}
	if completionTokens > 0 {
		completionAttrs := append(attrs, attribute.String("token_type", "completion"))
		r.userTokensUsed.Add(ctx, int64(completionTokens), metric.WithAttributes(completionAttrs...))
	}
	r.userBalance.Record(ctx, balance, metric.WithAttributes(attrs...))
}

// RecordDBQuery records database query metrics
func (r *OtelRecorder) RecordDBQuery(startTime time.Time, operation, table string, success bool) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.String("table", table),
		attribute.String("success", strconv.FormatBool(success)),
	}
	r.dbQueriesTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// UpdateDBConnectionMetrics updates database connection pool metrics
func (r *OtelRecorder) UpdateDBConnectionMetrics(inUse, idle int) {
	// Not implemented for Otel yet as it requires async gauges or periodic recording
}

// RecordRedisCommand records Redis command metrics
func (r *OtelRecorder) RecordRedisCommand(startTime time.Time, command string, success bool) {
	ctx := context.Background()
	duration := time.Since(startTime).Seconds()
	attrs := []attribute.KeyValue{
		attribute.String("command", command),
		attribute.String("success", strconv.FormatBool(success)),
	}
	r.redisCommandDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	r.redisCommandsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// UpdateRedisConnectionMetrics updates Redis connection metrics
func (r *OtelRecorder) UpdateRedisConnectionMetrics(active int) {
	// Not implemented for Otel yet
}

// RecordRateLimitHit records rate limit hit metrics
func (r *OtelRecorder) RecordRateLimitHit(limitType, identifier string) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("limit_type", limitType),
		attribute.String("identifier", identifier),
	}
	r.rateLimitHits.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// UpdateRateLimitRemaining updates remaining rate limit metrics
func (r *OtelRecorder) UpdateRateLimitRemaining(limitType, identifier string, remaining int) {
	// Not implemented for Otel yet
}

// RecordTokenAuth records token authentication metrics
func (r *OtelRecorder) RecordTokenAuth(success bool) {
	// Not implemented for Otel yet
}

// UpdateActiveTokens updates active token metrics
func (r *OtelRecorder) UpdateActiveTokens(userId, tokenName string, count int) {
	// Not implemented for Otel yet
}

// RecordError records error metrics
func (r *OtelRecorder) RecordError(errorType, component string) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
		attribute.String("component", component),
	}
	r.errorsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordModelUsage records model usage duration
func (r *OtelRecorder) RecordModelUsage(modelName, channelType string, latency time.Duration) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("model", modelName),
		attribute.String("channel_type", channelType),
	}
	r.modelUsageDuration.Record(ctx, latency.Seconds(), metric.WithAttributes(attrs...))
}

// RecordBillingOperation records billing operation metrics
func (r *OtelRecorder) RecordBillingOperation(startTime time.Time, operation string, success bool, userId int, channelId int, modelName string, quotaAmount float64) {
}

// RecordBillingTimeout records billing timeout metrics
func (r *OtelRecorder) RecordBillingTimeout(userId int, channelId int, modelName string, estimatedQuota float64, elapsedTime time.Duration) {
}

// RecordBillingError records billing error metrics
func (r *OtelRecorder) RecordBillingError(errorType, operation string, userId int, channelId int, modelName string) {
}

// UpdateBillingStats updates billing statistics
func (r *OtelRecorder) UpdateBillingStats(totalBillingOperations, successfulBillingOperations, failedBillingOperations int64) {
}

// InitSystemMetrics initializes system metrics
func (r *OtelRecorder) InitSystemMetrics(version, buildTime, goVersion string, startTime time.Time) {
}

// UpdateSiteWideStats updates site-wide statistics
func (r *OtelRecorder) UpdateSiteWideStats(totalQuota, usedQuota int64, totalUsers, activeUsers int) {
	ctx := context.Background()
	r.siteTotalQuota.Record(ctx, totalQuota)
	r.siteUsedQuota.Record(ctx, usedQuota)
	r.siteTotalUsers.Record(ctx, int64(totalUsers))
	r.siteActiveUsers.Record(ctx, int64(activeUsers))
}
