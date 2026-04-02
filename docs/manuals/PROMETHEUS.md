# Prometheus Monitoring for One API

This document describes the comprehensive Prometheus monitoring system implemented for One API.

## Overview

The Prometheus monitoring system provides detailed metrics about:

- HTTP requests and responses
- API relay operations
- Channel performance and health
- User activity and quota usage
- Database operations
- Redis operations (if enabled)
- Rate limiting
- System performance

## Configuration

### Environment Variables

- `ENABLE_PROMETHEUS_METRICS`: Enable/disable Prometheus metrics collection (default: `true`)
- `METRICS_TOKEN`: Bearer token required to access the `/metrics` endpoint. When not set, the endpoint returns 403. (default: empty)
- `ENABLE_METRIC`: Enable/disable the existing channel monitoring system (default: `false`)

### Metrics Endpoint

When Prometheus monitoring is enabled, metrics are available at:

```
http://your-server:port/metrics
```

**Authentication:** The `/metrics` endpoint requires a Bearer token configured via the `METRICS_TOKEN` environment variable. Requests without a valid token will be rejected.

### Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'one-api'
    bearer_token: '<your-metrics-token>'
    metrics_path: /metrics
    static_configs:
      - targets: ['one-api:3000']
```

Or using a file-based token (recommended for production):

```yaml
scrape_configs:
  - job_name: 'one-api'
    bearer_token_file: /etc/prometheus/one-api-token
    metrics_path: /metrics
    static_configs:
      - targets: ['one-api:3000']
```

## Available Metrics

### HTTP Request Metrics

- `one_api_http_request_duration_seconds`: Histogram of HTTP request durations
- `one_api_http_requests_total`: Counter of total HTTP requests
- `one_api_http_active_requests`: Gauge of currently active HTTP requests

Labels: `path`, `method`, `status_code`

### API Relay Metrics

- `one_api_relay_request_duration_seconds`: Histogram of API relay request durations
- `one_api_relay_requests_total`: Counter of total API relay requests
- `one_api_relay_tokens_total`: Counter of total tokens used
- `one_api_relay_quota_used_total`: Counter of total quota used

Labels: `channel_id`, `channel_type`, `model`, `user_id`, `success`, `token_type`

### Channel Metrics

- `one_api_channel_status`: Gauge of channel status (1=enabled, 0=disabled, -1=auto_disabled)
- `one_api_channel_balance_usd`: Gauge of channel balance in USD
- `one_api_channel_response_time_ms`: Gauge of channel response time in milliseconds
- `one_api_channel_success_rate`: Gauge of channel success rate (0-1)
- `one_api_channel_requests_in_flight`: Gauge of requests currently being processed

Labels: `channel_id`, `channel_name`, `channel_type`

### User Metrics

- `one_api_user_requests_total`: Counter of total requests by user
- `one_api_user_quota_used_total`: Counter of total quota used by user
- `one_api_user_tokens_total`: Counter of total tokens used by user
- `one_api_user_balance`: Gauge of user balance/quota remaining

Labels: `user_id`, `username`, `group`, `token_type`

### Database Metrics

- `one_api_db_connections_in_use`: Gauge of database connections currently in use
- `one_api_db_connections_idle`: Gauge of idle database connections
- `one_api_db_query_duration_seconds`: Histogram of database query durations
- `one_api_db_queries_total`: Counter of total database queries

Labels: `operation`, `table`, `success`

### Redis Metrics (if enabled)

- `one_api_redis_connections_active`: Gauge of active Redis connections
- `one_api_redis_command_duration_seconds`: Histogram of Redis command durations
- `one_api_redis_commands_total`: Counter of total Redis commands

Labels: `command`, `success`

### Rate Limiting Metrics

- `one_api_rate_limit_hits_total`: Counter of rate limit hits
- `one_api_rate_limit_remaining`: Gauge of remaining rate limit tokens

Labels: `type`, `identifier`

### Model Usage Metrics

- `one_api_model_usage_total`: Counter of total usage per model
- `one_api_model_latency_seconds`: Histogram of model response latency

Labels: `model_name`, `channel_type`

### System Metrics

- `one_api_system_info`: Gauge with system information
- `one_api_system_start_time_seconds`: Gauge of system start time

Labels: `version`, `build_time`, `go_version`

### Error Metrics

- `one_api_errors_total`: Counter of errors by type and component

Labels: `error_type`, `component`

## Grafana Dashboard Configuration

### Sample Queries

#### Request Rate

```promql
rate(one_api_http_requests_total[5m])
```

#### Request Duration 95th Percentile

```promql
histogram_quantile(0.95, rate(one_api_http_request_duration_seconds_bucket[5m]))
```

#### Channel Success Rate

```promql
one_api_channel_success_rate
```

#### Top Users by Quota Usage

```promql
topk(10, rate(one_api_user_quota_used_total[1h]))
```

#### Database Query Performance

```promql
histogram_quantile(0.95, rate(one_api_db_query_duration_seconds_bucket[5m]))
```

#### Model Usage Distribution

```promql
topk(10, rate(one_api_model_usage_total[1h]))
```

### Alerting Rules

#### High Error Rate

```yaml
- alert: HighErrorRate
  expr: rate(one_api_http_requests_total{status_code=~"5.."}[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: High error rate detected
```

#### Channel Down

```yaml
- alert: ChannelDown
  expr: one_api_channel_status == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: Channel {{ $labels.channel_name }} is down
```

#### Database Slow Queries

```yaml
- alert: SlowDatabaseQueries
  expr: histogram_quantile(0.95, rate(one_api_db_query_duration_seconds_bucket[5m])) > 1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: Database queries are slow
```

## Integration with Existing Monitoring

The Prometheus monitoring system works alongside the existing channel monitoring system:

- The existing `monitor.Emit()` function continues to work for channel health tracking
- Prometheus metrics provide additional detailed insights
- Both systems can be enabled independently via environment variables

## Performance Considerations

- Metrics collection has minimal performance impact
- High cardinality labels (like user IDs) are limited to essential use cases
- Path normalization reduces metric cardinality for HTTP requests
- Background collection goroutines minimize blocking operations

## Development and Debugging

### Adding New Metrics

1. Define the metric in `monitor/prometheus.go`
2. Add recording functions as needed
3. Call the recording functions from appropriate locations
4. Update this documentation

### Testing Metrics

You can test metrics collection by:

1. Making requests to your API
2. Checking the `/metrics` endpoint
3. Using Prometheus query UI or Grafana

### Debugging

- Set `ENABLE_PROMETHEUS_METRICS=false` to disable if issues occur
- Check logs for Prometheus initialization messages
- Verify metric names and labels in the `/metrics` endpoint

## Sample Grafana Dashboard JSON

A complete Grafana dashboard configuration is available in the `docs/grafana-dashboard.json` file.
