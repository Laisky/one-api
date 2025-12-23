# OpenTelemetry Operational Manual for One-API

This manual provides instructions for configuring and using OpenTelemetry (otel) for monitoring One-API.

## Menu

- [OpenTelemetry Operational Manual for One-API](#opentelemetry-operational-manual-for-one-api)
  - [Menu](#menu)
  - [Configuration](#configuration)
    - [Example Setup](#example-setup)
  - [Metrics Reference](#metrics-reference)
    - [Relay Metrics](#relay-metrics)
    - [Channel Metrics](#channel-metrics)
    - [User Metrics](#user-metrics)
    - [Dashboard \& Site-wide Metrics](#dashboard--site-wide-metrics)
    - [System Metrics](#system-metrics)
  - [Tracing Integration](#tracing-integration)
  - [Grafana Integration](#grafana-integration)
    - [1. Data Source Setup](#1-data-source-setup)
    - [2. Dashboard Queries (Matching One-API Dashboard)](#2-dashboard-queries-matching-one-api-dashboard)
    - [3. Visualization Tips](#3-visualization-tips)
    - [3. Dashboard Template](#3-dashboard-template)

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

### 2. Dashboard Queries (Matching One-API Dashboard)

- **Total Requests (Overview Card):** `sum(increase(one_api_relay_requests_total[24h]))`
- **Total Quota Used (Overview Card):** `sum(increase(one_api_relay_quota_used_total[24h]))`
- **Top Models by Request Count:** `topk(10, sum(increase(one_api_relay_requests_total[24h])) by (model))`
- **Usage by User (Stacked Chart):** `sum(increase(one_api_relay_requests_total[24h])) by (user_id)`
- **Site Quota Usage Ratio:** `one_api_site_used_quota / one_api_site_total_quota`

### 3. Visualization Tips

- **Request Volume by Model:** `sum(rate(one_api_relay_requests_total[5m])) by (model)`
- **Success Rate by Channel:** `sum(rate(one_api_relay_requests_total{success="true"}[5m])) by (channel_id) / sum(rate(one_api_relay_requests_total[5m])) by (channel_id)`
- **Token Usage Over Time:** `sum(rate(one_api_relay_tokens_total[5m])) by (token_type)`
- **P95 Latency:** `histogram_quantile(0.95, sum(rate(one_api_relay_request_duration_seconds_bucket[5m])) by (le, model))`

### 3. Dashboard Template

You can import the following JSON as a starting point for your One-API dashboard. This dashboard includes both HTTP service metrics and business-specific relay/quota metrics.

<details>
<summary>Click to expand Dashboard JSON</summary>

```json
{
  "annotations": {
    "list": [
      {
        "builtIn": 1,
        "datasource": {
          "type": "grafana",
          "uid": "-- Grafana --"
        },
        "enable": true,
        "hide": true,
        "iconColor": "rgba(0, 211, 255, 1)",
        "name": "Annotations & Alerts",
        "type": "dashboard"
      }
    ]
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "links": [],
  "panels": [
    {
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 100,
      "title": "Site Overview",
      "type": "row"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "none"
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 1 },
      "id": 101,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": {
          "calcs": ["lastNotNull"],
          "fields": "",
          "values": false
        },
        "textMode": "auto"
      },
      "targets": [
        {
          "expr": "one_api_site_total_users",
          "legendFormat": "Total Users",
          "refId": "A"
        }
      ],
      "title": "Total Users",
      "type": "stat"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "none"
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 1 },
      "id": 102,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": {
          "calcs": ["lastNotNull"],
          "fields": "",
          "values": false
        },
        "textMode": "auto"
      },
      "targets": [
        {
          "expr": "one_api_site_active_users",
          "legendFormat": "Active Users",
          "refId": "A"
        }
      ],
      "title": "Active Users",
      "type": "stat"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 70 },
              { "color": "red", "value": 90 }
            ]
          },
          "unit": "percentunit"
        }
      },
      "gridPos": { "h": 4, "w": 12, "x": 12, "y": 1 },
      "id": 103,
      "options": {
        "orientation": "horizontal",
        "reduceOptions": {
          "calcs": ["lastNotNull"],
          "fields": "",
          "values": false
        },
        "showThresholdLabels": false,
        "showThresholdMarkers": true
      },
      "targets": [
        {
          "expr": "one_api_site_used_quota / one_api_site_total_quota",
          "legendFormat": "Quota Usage",
          "refId": "A"
        }
      ],
      "title": "Site Quota Usage Ratio",
      "type": "gauge"
    },
    {
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 5 },
      "id": 200,
      "title": "Relay Performance",
      "type": "row"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 2 },
          "unit": "ops"
        }
      },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 6 },
      "id": 201,
      "options": {
        "legend": { "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "multi" }
      },
      "targets": [
        {
          "expr": "sum(rate(one_api_relay_requests_total{model=~\"$model\"}[5m])) by (model)",
          "legendFormat": "{{model}}",
          "refId": "A"
        }
      ],
      "title": "Relay RPS by Model",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "red", "value": null },
              { "color": "yellow", "value": 95 },
              { "color": "green", "value": 99 }
            ]
          },
          "unit": "percent"
        }
      },
      "gridPos": { "h": 8, "w": 8, "x": 8, "y": 6 },
      "id": 202,
      "options": {
        "legend": { "displayMode": "list", "placement": "bottom" }
      },
      "targets": [
        {
          "expr": "sum(rate(one_api_relay_requests_total{success=\"true\", model=~\"$model\"}[5m])) by (model) / sum(rate(one_api_relay_requests_total{model=~\"$model\"}[5m])) by (model) * 100",
          "legendFormat": "{{model}}",
          "refId": "A"
        }
      ],
      "title": "Relay Success Rate (%)",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "s"
        }
      },
      "gridPos": { "h": 8, "w": 8, "x": 16, "y": 6 },
      "id": 203,
      "options": {
        "legend": { "displayMode": "list", "placement": "bottom" }
      },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(one_api_relay_request_duration_seconds_bucket{model=~\"$model\"}[5m])) by (le, model))",
          "legendFormat": "{{model}} (P95)",
          "refId": "A"
        }
      ],
      "title": "Relay P95 Latency",
      "type": "timeseries"
    },
    {
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 14 },
      "id": 300,
      "title": "Usage & Billing",
      "type": "row"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": { "stacking": { "mode": "normal" } },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 15 },
      "id": 301,
      "options": {
        "legend": { "displayMode": "table", "placement": "right" }
      },
      "targets": [
        {
          "expr": "sum(rate(one_api_relay_tokens_total{model=~\"$model\"}[5m])) by (token_type)",
          "legendFormat": "{{token_type}}",
          "refId": "A"
        }
      ],
      "title": "Token Usage (Prompt vs Completion)",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "decbytes"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 15 },
      "id": 302,
      "options": {
        "legend": { "displayMode": "table", "placement": "right" }
      },
      "targets": [
        {
          "expr": "sum(rate(one_api_relay_quota_used_total{model=~\"$model\"}[5m])) by (model)",
          "legendFormat": "{{model}}",
          "refId": "A"
        }
      ],
      "title": "Quota Consumption by Model",
      "type": "timeseries"
    },
    {
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 23 },
      "id": 400,
      "title": "HTTP Service Overview",
      "type": "row"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "ops"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 24 },
      "id": 1,
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count{service_name=\"one-api\", http_route=~\"$route\"}[5m]))",
          "legendFormat": "RPS",
          "refId": "A"
        }
      ],
      "title": "HTTP Requests per Second",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 5 }
            ]
          },
          "unit": "percent"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 24 },
      "id": 2,
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count{service_name=\"one-api\", http_response_status_code=~\"5..\"}[5m])) / sum(rate(http_server_request_duration_seconds_count{service_name=\"one-api\"}[5m])) * 100",
          "legendFormat": "5xx %",
          "refId": "A"
        }
      ],
      "title": "HTTP Error Rate (%)",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "ms"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 32 },
      "id": 3,
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_sum{service_name=\"one-api\", http_route=~\"$route\"}[5m])) / sum(rate(http_server_request_duration_seconds_count{service_name=\"one-api\", http_route=~\"$route\"}[5m])) * 1000",
          "legendFormat": "{{http_route}}",
          "refId": "A"
        }
      ],
      "title": "Average HTTP Latency (ms)",
      "type": "timeseries"
    },
    {
      "datasource": "VictoriaMetrics",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "unit": "ops"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 32 },
      "id": 6,
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count{service_name=\"one-api\"}[5m])) by (http_response_status_code)",
          "legendFormat": "{{http_response_status_code}}",
          "refId": "A"
        }
      ],
      "title": "Status Code Distribution",
      "type": "timeseries"
    }
  ],
  "refresh": "10s",
  "schemaVersion": 40,
  "tags": ["one-api", "opentelemetry", "business"],
  "templating": {
    "list": [
      {
        "datasource": "VictoriaMetrics",
        "includeAll": true,
        "multi": true,
        "name": "route",
        "query": "label_values(http_server_request_duration_seconds_count{service_name=\"one-api\"}, http_route)",
        "refresh": 1,
        "type": "query"
      },
      {
        "datasource": "VictoriaMetrics",
        "includeAll": true,
        "multi": true,
        "name": "model",
        "query": "label_values(one_api_relay_requests_total, model)",
        "refresh": 1,
        "type": "query"
      }
    ]
  },
  "time": { "from": "now-24h", "to": "now" },
  "title": "One-API Comprehensive Overview",
  "uid": "one-api-comprehensive",
  "version": 1
}
```

</details>
