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
- `METRICS_MAX_PATH_LABELS`: Maximum number of distinct normalized request paths recorded as `path` label values per process; further paths are counted under `/other`. Non-positive values fall back to the default. (default: `1000`)
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

The `path` label is normalized to bound cardinality: numeric ids, UUIDs and tokens under `/api/` become `:id`, `:uuid` and `:token`; relay routes collapse to their family (`/v1/chat/completions`, `/v1/images/:action`, `/v1/other`, ...); paths longer than 100 bytes are truncated on a UTF-8 boundary; any byte sequence that is not valid UTF-8 (for example a percent-encoded `%c0`) is replaced by U+FFFD; and once `METRICS_MAX_PATH_LABELS` distinct paths have been seen, every new path is recorded as `/other` (vulnerability scanners probe thousands of distinct paths).

### API Relay Metrics

- `one_api_relay_request_duration_seconds`: Histogram of API relay request durations
- `one_api_relay_requests_total`: Counter of total API relay requests
- `one_api_relay_tokens_total`: Counter of total tokens used
- `one_api_relay_quota_used_total`: Counter of total quota used

Labels: `channel_id`, `channel_type`, `model`, `group`, `api_format`, `api_type`, `success`, `token_type`

> Note: `user_id` and `token_id` are intentionally omitted from relay metrics to avoid unbounded label cardinality (one series per user/token combination grows memory without bound). The per-user `one_api_user_*` metrics below are broken down by `group` only (also no `user_id`/`username`); for per-user detail, query the request logs and billing tables in the database.

### Channel Metrics

- `one_api_channel_status`: Gauge of channel status (1=enabled, 0=disabled, -1=auto_disabled)
- `one_api_channel_balance_usd`: Gauge of channel balance in USD
- `one_api_channel_response_time_ms`: Gauge of channel response time in milliseconds
- `one_api_channel_success_rate`: Gauge of channel success rate (0-1)
- `one_api_channel_requests_in_flight`: Gauge of requests currently being processed

Labels: `channel_id`, `channel_name`, `channel_type`

### User Metrics

- `one_api_user_requests_total`: Counter of total requests by user group
- `one_api_user_quota_used_total`: Counter of total quota used by user group
- `one_api_user_tokens_total`: Counter of total tokens used by user group

Labels: `group` (and `token_type` on `one_api_user_tokens_total`)

> Note: `user_id` and `username` are intentionally omitted from these metrics to avoid unbounded label cardinality (one permanent time series per user grows memory without bound). Per-user breakdowns are available from the request logs and billing tables in the database.
>
> `one_api_user_balance` is no longer populated: once `user_id`/`username` are dropped, a per-group balance gauge would be last-write-wins across all users in the group and therefore misleading. Per-user balance lives in the database, and site-wide quota is covered by the `one_api_site_*` gauges.

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

- `one_api_rate_limit_hits_total`: Counter of responses rejected with HTTP 429
- `one_api_rate_limit_remaining`: Reserved; not populated. It was previously fed from the client-supplied `X-RateLimit-Remaining` request header, which is untrusted input, so the series is no longer written.

Labels: `limit_type` (which limiter rejected the request: `web`, `api`, `critical`, `download`, `upload`, `relay`, `conversations`, `channel`, `low_balance`; `other` for a 429 not produced by a limiter, e.g. an upstream provider's 429 relayed to the client), `identifier` (what the limiter keys on: `ip`, `token`, `user`; `none` for `other`). Neither label carries a client IP, token or user id: those values are unbounded and would create one time series per caller.

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

#### Top Groups by Quota Usage

```promql
topk(10, sum(rate(one_api_user_quota_used_total[1h])) by (group))
```

(Per-user quota breakdowns are no longer available as a metric; query the request logs / billing tables in the database instead.)

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
