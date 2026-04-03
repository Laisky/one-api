# One API Grafana Dashboard Design Document

## Overview

A single Grafana Dashboard containing 7 collapsible Row sections, covering both Ops/SRE and platform management perspectives. All `one_api_*` Prometheus metrics are mapped to panels. Panels marked as "reserved" currently have no production data, but are pre-configured with PromQL and will automatically display data when it becomes available.

**Data source**: Prometheus, scraping `GET /metrics` (requires Bearer token authentication via `METRICS_TOKEN`).

---

## Top-level Variables

| Variable | Type | Query / Values | Default | Multi/All |
|------|------|-------------|--------|-----------|
| `$channel_id` | Query | `label_values(one_api_relay_requests_total, channel_id)` | All | Yes |
| `$channel_type` | Query | `label_values(one_api_relay_requests_total, channel_type)` | All | Yes |
| `$model` | Query | `label_values(one_api_relay_requests_total, model)` | All | Yes |
| `$username` | Query | `label_values(one_api_user_requests_total, username)` | All | Yes |
| `$group` | Query | `label_values(one_api_user_requests_total, group)` | All | Yes |
| `$currency` | Custom | `USD,CNY` | USD | No |
| `$exchange_rate` | Textbox | - | 8 | No |

### Currency Conversion

All monetary panels use the following formula:

```text
quota_value / 500000 * (1 + ($currency == "CNY") * ($exchange_rate - 1))
```

- `500000` = `QuotaPerUnit` (hardcoded in `common/config/config.go:1132`)
- When `$currency = USD`: display `$` symbol
- When `$currency = CNY`: multiply by `$exchange_rate` (default 8), display `¥` symbol

---

## Row 1: Overview (expanded by default)

### Stat Cards (top row, 6 cards)

| Panel | PromQL | Unit / Thresholds |
|------|--------|-------------|
| System Uptime | `time() - one_api_system_start_time_seconds` | Duration (s) |
| Total Request Rate | `sum(rate(one_api_http_requests_total[$__rate_interval]))` | req/s |
| Relay Success Rate | `sum(rate(one_api_relay_requests_total{success="true"}[$__rate_interval])) / sum(rate(one_api_relay_requests_total[$__rate_interval]))` | Percentage; green >99%, yellow >95%, red <=95% |
| Active Users | `one_api_site_active_users` | Count |
| Site-wide Quota Usage | `one_api_site_used_quota / one_api_site_total_quota` | Percentage + progress bar; green <60%, yellow <80%, red >=80% |
| Auth Failure Rate | `sum(rate(one_api_token_auth_attempts_total{success="false"}[$__rate_interval])) / sum(rate(one_api_token_auth_attempts_total[$__rate_interval]))` | Percentage; green <1%, yellow <5%, red >=5% (reserved) |

### Time Series (bottom row, 2 charts)

| Panel | PromQL | Legend |
|------|--------|------|
| HTTP Request Rate (by status code) | `sum(rate(one_api_http_requests_total[$__rate_interval])) by (status_code)` | `{{status_code}}` |
| Error Overview | `sum(rate(one_api_errors_total[$__rate_interval])) by (component)` | `{{component}}` (reserved) |

---

## Row 2: Relay Requests (collapsed)

| Panel | Type | PromQL | Legend / Description |
|------|------|--------|-------------|
| Relay QPS | Time series | `sum(rate(one_api_relay_requests_total{channel_id=~"$channel_id",model=~"$model"}[$__rate_interval])) by (model)` | `{{model}}` |
| Relay Success Rate | Time series | `sum(rate(one_api_relay_requests_total{...,success="true"}[$__rate_interval])) by (model) / sum(rate(one_api_relay_requests_total{...}[$__rate_interval])) by (model)` | `{{model}}` |
| Relay Latency P50/P95/P99 | Time series | `histogram_quantile(0.5/0.95/0.99, sum(rate(one_api_relay_request_duration_seconds_bucket{channel_id=~"$channel_id",model=~"$model"}[$__rate_interval])) by (le, model))` | `{{model}} p50/p95/p99` |
| Token Consumption Rate | Time series | `sum(rate(one_api_relay_tokens_total{channel_id=~"$channel_id",model=~"$model"}[$__rate_interval])) by (model, token_type)` | `{{model}}-{{token_type}}` |
| Quota Consumption Rate | Time series | `sum(rate(one_api_relay_quota_used_total{channel_id=~"$channel_id",model=~"$model"}[$__rate_interval])) by (model) / 500000 * currency_multiplier` | `{{model}}`, unit=$currency |
| Request Distribution (by channel type) | Pie chart | `sum(one_api_relay_requests_total{channel_id=~"$channel_id"}) by (channel_type)` | `{{channel_type}}` |
| Request Distribution (by API format) | Pie chart | `sum(one_api_relay_requests_total{channel_id=~"$channel_id"}) by (api_format)` | `{{api_format}}` |

---

## Row 3: Channel Health (collapsed)

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| Channel Status Table | Table | `one_api_channel_status` | Columns: channel_id, channel_name, channel_type, status. Colors: green=1 (enabled), red=0 (disabled), yellow=-1 (auto-disabled). (reserved) |
| Channel In-flight Requests | Time series | `one_api_channel_requests_in_flight{channel_id=~"$channel_id"}` | `{{channel_id}}-{{channel_type}}` |
| Channel Response Time | Time series | `one_api_channel_response_time_ms{channel_id=~"$channel_id"}` | Milliseconds (reserved) |
| Channel Success Rate | Time series | `one_api_channel_success_rate{channel_id=~"$channel_id"}` | 0-1 (reserved) |
| Channel Balance | Stat (repeated by channel_id) | `one_api_channel_balance_usd{channel_id=~"$channel_id"} * currency_multiplier` | Unit=$currency (reserved) |
| Channel Request Volume TopK | Bar chart | `topk(10, sum(rate(one_api_relay_requests_total{channel_id=~"$channel_id"}[$__rate_interval])) by (channel_id, channel_type))` | `{{channel_id}}-{{channel_type}}` |

---

## Row 4: Users & Quota (collapsed)

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| Site-wide Quota Usage | Gauge | `one_api_site_used_quota / one_api_site_total_quota` | Green <60%, yellow <80%, red >=80% |
| Site-wide Used Quota | Stat | `one_api_site_used_quota / 500000 * currency_multiplier` | Unit=$currency |
| Site-wide Total Quota | Stat | `one_api_site_total_quota / 500000 * currency_multiplier` | Unit=$currency |
| User Request Volume Top10 | Bar chart | `topk(10, sum(one_api_user_requests_total{username=~"$username",group=~"$group"}) by (username, group))` | `{{username}}({{group}})` |
| User Quota Consumption Top10 | Bar chart | `topk(10, sum(rate(one_api_billing_quota_processed_total{username=~"$username"}[$__rate_interval])) by (username)) / 500000 * currency_multiplier` | Unit=$currency |
| User Request Rate Trend | Time series | `sum(rate(one_api_user_requests_total{username=~"$username",group=~"$group"}[$__rate_interval])) by (username, group)` | `{{username}}({{group}})` |
| User Token Consumption Trend | Time series | `sum(rate(one_api_user_tokens_total{username=~"$username"}[$__rate_interval])) by (username, token_type)` | `{{username}}-{{token_type}}` |
| User Group Distribution | Pie chart | `sum(one_api_user_requests_total) by (group)` | default vs vip |

---

## Row 5: Billing (collapsed)

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| Billing Operation Rate | Time series | `sum(rate(one_api_billing_operations_total[$__rate_interval])) by (operation, success)` | `{{operation}}-{{success}}` |
| Billing P95 Latency | Time series | `histogram_quantile(0.95, sum(rate(one_api_billing_operation_duration_seconds_bucket[$__rate_interval])) by (le, operation))` | `{{operation}}` |
| Billing Success Rate | Stat | `sum(rate(one_api_billing_operations_total{success="true"}[$__rate_interval])) / sum(rate(one_api_billing_operations_total[$__rate_interval]))` | Green >99%, yellow >95%, red <=95% |
| Billing Timeout Count | Time series | `sum(rate(one_api_billing_timeouts_total[$__rate_interval])) by (model_name)` | `{{model_name}}` (reserved) |
| Billing Error Distribution | Time series | `sum(rate(one_api_billing_errors_total[$__rate_interval])) by (error_type, operation)` | `{{error_type}}-{{operation}}` (reserved) |
| Billing Stats Overview | Stat (3 cards) | `one_api_billing_stats{stat_type="total_operations"}` / `successful_operations` / `failed_operations` | (reserved) |
| Quota Processed Amount Trend | Time series | `sum(rate(one_api_billing_quota_processed_total[$__rate_interval])) by (model_name) / 500000 * currency_multiplier` | `{{model_name}}`, unit=$currency |

---

## Row 6: Infrastructure - DB & Redis (collapsed)

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| DB Connection Pool | Time series | `one_api_db_connections_in_use` and `one_api_db_connections_idle` | Two lines: in-use, idle |
| DB Query Rate | Time series | `sum(rate(one_api_db_queries_total[$__rate_interval])) by (operation, table)` | `{{operation}}-{{table}}` |
| DB Query P95 Latency | Time series | `histogram_quantile(0.95, sum(rate(one_api_db_query_duration_seconds_bucket[$__rate_interval])) by (le, table))` | `{{table}}` |
| DB Query Failure Rate | Time series | `sum(rate(one_api_db_queries_total{success="false"}[$__rate_interval])) by (table)` | `{{table}}` |
| DB Hot Tables Top5 | Bar chart | `topk(5, sum(rate(one_api_db_queries_total[$__rate_interval])) by (table))` | `{{table}}` |
| Redis Active Connections | Time series | `one_api_redis_connections_active` | Single line |
| Redis Command Rate | Time series | `sum(rate(one_api_redis_commands_total[$__rate_interval])) by (command)` | `{{command}}` (reserved) |
| Redis Command P95 Latency | Time series | `histogram_quantile(0.95, sum(rate(one_api_redis_command_duration_seconds_bucket[$__rate_interval])) by (le, command))` | `{{command}}` (reserved) |

---

## Row 7: Security & Rate Limiting + Go Runtime (collapsed)

### Security & Rate Limiting

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| Auth Success/Failure Trend | Time series | `sum(rate(one_api_token_auth_attempts_total[$__rate_interval])) by (success)` | (reserved) |
| Auth Failure Rate | Stat | `sum(rate(one_api_token_auth_attempts_total{success="false"}[$__rate_interval])) / sum(rate(one_api_token_auth_attempts_total[$__rate_interval]))` | Green <1%, yellow <5%, red >=5% (reserved) |
| Rate Limit Trigger Rate | Time series | `sum(rate(one_api_rate_limit_hits_total[$__rate_interval])) by (type)` | `{{type}}` (reserved) |
| Error Distribution | Time series | `sum(rate(one_api_errors_total[$__rate_interval])) by (error_type, component)` | `{{component}}-{{error_type}}` (reserved) |
| Error Top5 Components | Bar chart | `topk(5, sum(rate(one_api_errors_total[$__rate_interval])) by (component))` | `{{component}}` (reserved) |

### Go Runtime

| Panel | Type | PromQL | Description |
|------|------|--------|------|
| Goroutines | Time series | `go_goroutines` | Count |
| Heap Memory | Time series | `go_memstats_heap_inuse_bytes` and `go_memstats_heap_idle_bytes` | Bytes (auto) |
| GC Pause Time | Time series | `rate(go_gc_duration_seconds_sum[$__rate_interval])` | Seconds |
| Process CPU | Time series | `rate(process_cpu_seconds_total[$__rate_interval])` | Ratio (0-1) |
| Process RSS Memory | Stat | `process_resident_memory_bytes` | Bytes (auto) |

---

## Panel Count Summary

| Row | Panel Count |
|-----|--------|
| 1. Overview | 8 |
| 2. Relay Requests | 7 |
| 3. Channel Health | 6 |
| 4. Users & Quota | 8 |
| 5. Billing | 7 |
| 6. Infrastructure DB+Redis | 8 |
| 7. Security & Rate Limiting + Go Runtime | 10 |
| **Total** | **54** |

---

## Production Data Status

Metrics with actual production data (confirmed from `https://one-api.xxx.com/metrics`):

- `one_api_http_requests_total`, `one_api_http_active_requests`, `one_api_http_request_duration_seconds`
- `one_api_relay_requests_total`, `one_api_relay_request_duration_seconds`
- `one_api_billing_operations_total`, `one_api_billing_operation_duration_seconds`, `one_api_billing_quota_processed_total`
- `one_api_channel_requests_in_flight` (3 channels: 1=openaicompatible, 2=openaicompatible, 3=alibailian)
- `one_api_model_usage_total`, `one_api_model_latency_seconds` (qwen3.5-plus, MiniMax-M2, Qwen3-30B-A3B)
- `one_api_user_requests_total`, `one_api_user_tokens_total`, `one_api_user_balance`
- `one_api_db_queries_total`, `one_api_db_query_duration_seconds`, `one_api_db_connections_*`
- `one_api_site_*` (active_users=87, total_users=87)
- `one_api_system_info`, `one_api_system_start_time_seconds`
- `one_api_redis_connections_active`
- `go_*`, `process_*`

Reserved metrics (currently no data):

- `one_api_channel_status`, `one_api_channel_balance_usd`, `one_api_channel_response_time_ms`, `one_api_channel_success_rate`
- `one_api_relay_tokens_total`, `one_api_relay_quota_used_total`
- `one_api_token_auth_attempts_total`, `one_api_active_tokens`
- `one_api_rate_limit_hits_total`, `one_api_rate_limit_remaining`
- `one_api_errors_total`
- `one_api_billing_timeouts_total`, `one_api_billing_errors_total`, `one_api_billing_stats`
- `one_api_redis_commands_total`, `one_api_redis_command_duration_seconds`
- `one_api_user_quota_used_total`

---

## Implementation Notes

- Dashboard JSON will be saved to `docs/grafana-dashboard.json` (replacing the file referenced in documentation but actually missing)
- Target Grafana version: 10.x+
- All panels use `$__rate_interval` for rate calculations (Grafana best practice)
- Reserved panels display a "No data" message rather than being hidden; they will automatically display when data becomes available
- `$username` variable note: panels use `username` as the user identifier; the TokenAuth middleware has been updated to set the username
