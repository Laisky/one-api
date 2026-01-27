# OpenTelemetry Operational Manual for One-API

This manual provides instructions for configuring and using OpenTelemetry (otel) for monitoring One-API.

## Menu

- [OpenTelemetry Operational Manual for One-API](#opentelemetry-operational-manual-for-one-api)
  - [Menu](#menu)
  - [Configuration](#configuration)
    - [Example Setup](#example-setup)
    - [Production Environment (Current)](#production-environment-current)
    - [Data Flow Verification (Admin Checklist)](#data-flow-verification-admin-checklist)
  - [Metrics Reference](#metrics-reference)
    - [Relay Metrics](#relay-metrics)
    - [Channel Metrics](#channel-metrics)
    - [User Metrics](#user-metrics)
    - [Dashboard \& Site-wide Metrics](#dashboard--site-wide-metrics)
    - [System Metrics](#system-metrics)
  - [Tracing Integration](#tracing-integration)
  - [Grafana Integration](#grafana-integration)
    - [1. Data Source Setup](#1-data-source-setup)
    - [2. One-API Observability Dashboard](#2-one-api-observability-dashboard)
      - [Dashboard Coverage Notes](#dashboard-coverage-notes)
    - [3. Logs \& Traces (VictoriaLogs/VictoriaTraces)](#3-logs--traces-victorialogsvictoriatraces)
    - [4. Dashboard Queries (Matching One-API Dashboard)](#4-dashboard-queries-matching-one-api-dashboard)
    - [5. Visualization Tips](#5-visualization-tips)

## Configuration

OpenTelemetry integration is configured via environment variables.

| Variable                      | Description                                  | Default               |
| ----------------------------- | -------------------------------------------- | --------------------- |
| `OTEL_ENABLED`                | Enable OpenTelemetry integration             | `false`               |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP exporter endpoint (HTTP)                | (required if enabled) |
| `OTEL_EXPORTER_OTLP_INSECURE` | Use insecure connection for OTLP             | `true`                |
| `OTEL_SERVICE_NAME`           | Service name for otel                        | `one-api`             |
| `OTEL_ENVIRONMENT`            | Environment name (e.g., production, staging) | `debug`               |

### Example Setup

To enable OpenTelemetry and send data to a local collector:

```bash
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_SERVICE_NAME=one-api-prod
export OTEL_ENVIRONMENT=production
```

### Production Environment (Current)

Use the production configuration below when deploying one-api in production:

```bash
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://100.97.108.34:4318
export OTEL_SERVICE_NAME=one-api
export OTEL_ENVIRONMENT=production
```

### Data Flow Verification (Admin Checklist)

Use this checklist to ensure one-api is emitting enough telemetry data for dashboards and alerts:

1. **Confirm OTEL exporter availability**

- The OTLP HTTP collector must accept data at `OTEL_EXPORTER_OTLP_ENDPOINT`.

2. **Confirm metrics backend availability**

- VictoriaMetrics (or Prometheus-compatible backend) must be reachable by Grafana.
- Grafana data source should point to `http://<victoriametrics-host>:8428`.

3. **Confirm metrics presence**

- Query `one_api_relay_requests_total` and `one_api_http_requests_total` for recent data.

4. **Confirm tracing presence**

- Verify traces appear in the tracing backend (e.g., VictoriaTraces/Jaeger/Tempo).

5. **Confirm log ingestion (optional)**

- one-api logs are stored under `./logs/`. Ship them to VictoriaLogs/Loki to enable log panels.

## Metrics Reference

One-API exports a wide range of business and system metrics via OpenTelemetry.

### Relay Metrics

These metrics track the core functionality of One-API: relaying requests to AI providers.

- **`one_api_relay_requests_total`** (Counter)
  - Description: Total number of API relay requests.
  - Labels: `channel_id`, `channel_type`, `model`, `user_id`, `group`, `token_id`, `api_format`, `api_type`, `success`
- **`one_api_relay_request_duration_seconds`** (Histogram)
  - Description: Duration of API relay requests in seconds.
  - Labels: same as above.
- **`one_api_relay_tokens_total`** (Counter)
  - Description: Total number of tokens used.
  - Labels: `channel_id`, `channel_type`, `model`, `user_id`, `group`, `token_id`, `api_format`, `api_type`, `token_type` (prompt/completion)
- **`one_api_relay_quota_used_total`** (Counter)
  - Description: Total quota used in relay requests.
  - Labels: `channel_id`, `channel_type`, `model`, `user_id`, `group`, `token_id`, `api_format`, `api_type`

### Channel Metrics

- **`one_api_channel_status`** (Gauge): Channel status (1=enabled, 0=disabled, -1=auto_disabled).
- **`one_api_channel_balance_usd`** (Gauge): Channel balance in USD.
- **`one_api_channel_success_rate`** (Gauge): Channel success rate (0-1).
- **`one_api_channel_requests_in_flight`** (UpDownCounter): Number of concurrent requests per channel.

### User Metrics

- **`one_api_user_requests_total`** (Counter): Total requests by user.
- **`one_api_user_quota_used_total`** (Counter): Total quota used by user.
- **`one_api_user_balance`** (Gauge): Current user balance.

### Dashboard & Site-wide Metrics

These metrics provide the high-level overview seen on the One-API dashboard.

- **`one_api_site_total_quota`** (Gauge): Total quota allocated across all users.
- **`one_api_site_used_quota`** (Gauge): Total quota consumed by all users.
- **`one_api_site_total_users`** (Gauge): Total number of registered users (excluding deleted).
- **`one_api_site_active_users`** (Gauge): Number of users with "Enabled" status.

### System Metrics

- **`one_api_http_requests_total`** (Counter): Total HTTP requests to the One-API server.
- **`one_api_errors_total`** (Counter): Total number of internal errors.
- **`one_api_db_queries_total`** (Counter): Database query volume.

## Tracing Integration

One-API integrates its internal request tracing system with OpenTelemetry. Key request lifecycle events are recorded as span events:

- `request_received`: When the request first hits the server.
- `request_forwarded`: When the request is sent to the upstream provider.
- `first_upstream_response`: When the first byte is received from upstream.
- `first_client_response`: When the first byte is sent back to the client.
- `upstream_completed`: When the upstream response is fully received.
- `request_completed`: When the entire request cycle is finished.

Additional attributes like `one_api.trace_id`, `one_api.url`, `one_api.method`, and `one_api.body_size` are also added to the spans.

## Grafana Integration

### 1. Data Source Setup

1.  Install the **OpenTelemetry** or **Prometheus** data source in Grafana (depending on your backend, e.g., VictoriaMetrics or Jaeger).
2.  If using VictoriaMetrics (recommended), add it as a Prometheus data source pointing to `http://<victoriametrics-host>:8428`.
3.  (Optional) Add a tracing data source (VictoriaTraces/Jaeger/Tempo) for span and trace panels.
4.  (Optional) Add a logging data source (VictoriaLogs/Loki) if you want to visualize request logs.

### 2. One-API Observability Dashboard

The Grafana dashboard **One-API Observability (OTel)** provides a unified view of:

- **Tenant usage**: tokens, quota consumption, usage by user and model.
- **Relay performance**: request rates, success ratios, and latency percentiles.
- **HTTP service health**: request counts, error rate, latency, and status code distribution.

Open the dashboard in Grafana and ensure panels are not showing "No data". If they are, verify the data flow checklist above.

#### Dashboard Coverage Notes

The dashboard uses the following core metrics to approximate quota and usage where direct quota metrics are not available per user/model:

- **User usage trend:** `one_api_user_tokens_total` (token consumption over time).
- **User balance (remaining):** `one_api_user_balance` (current balance per user).
- **Model usage trend:** `one_api_relay_tokens_total` (token usage by model).

If you need strict quota accounting per user/model, ensure the backend emits per-user and per-model quota metrics and update the queries accordingly.

### 3. Logs & Traces (VictoriaLogs/VictoriaTraces)

To make request logs and traces visible in Grafana:

1. **Install Grafana data source plugins**

- VictoriaLogs data source plugin (for LogsQL queries).
- VictoriaTraces data source plugin (or expose a Jaeger/Tempo-compatible query endpoint).

2. **Configure data sources**

- VictoriaLogs URL: `http://100.97.108.34:9428`
- VictoriaTraces URL: `http://100.97.108.34:10428`

3. **Retention policy (recommended)**

- Set a retention period in the VictoriaLogs container, e.g. `-retentionPeriod=7d` or as required by compliance.

Once the data sources are available, add panels for:

- Log volume by level (info/warn/error).
- Log search and drill-down (click to view raw log entries).
- Trace overview (request latency distribution, trace samples by endpoint).

### 4. Dashboard Queries (Matching One-API Dashboard)

- **Total Requests (Overview Card):** `sum(increase(one_api_relay_requests_total[24h]))`
- **Total Quota Used (Overview Card):** `sum(increase(one_api_relay_quota_used_total[24h]))`
- **Top Models by Request Count:** `topk(10, sum(increase(one_api_relay_requests_total[24h])) by (model))`
- **Usage by User (Stacked Chart):** `sum(increase(one_api_relay_requests_total[24h])) by (user_id)`
- **Site Quota Usage Ratio:** `one_api_site_used_quota / one_api_site_total_quota`

### 5. Visualization Tips

- **Request Volume by Model:** `sum(rate(one_api_relay_requests_total[5m])) by (model)`
- **Success Rate by Channel:** `sum(rate(one_api_relay_requests_total{success="true"}[5m])) by (channel_id) / sum(rate(one_api_relay_requests_total[5m])) by (channel_id)`
- **Token Usage Over Time:** `sum(rate(one_api_relay_tokens_total[5m])) by (token_type)`
- **P95 Latency:** `histogram_quantile(0.95, sum(rate(one_api_relay_request_duration_seconds_bucket[5m])) by (le, model))`
