# One API Grafana Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a complete Grafana Dashboard JSON configuration file, containing 7 Rows, 54 panels, top-level variables, and currency conversion logic.

**Architecture:** A single `docs/grafana-dashboard.json` file, in Grafana 10.x+ Dashboard JSON Model format. Built incrementally by Row, with each Task responsible for one Row's panel definitions. Uses Grafana's `templating` for variable filtering, and Transformations + Math expression for currency conversion.

**Tech Stack:** Grafana 10.x+ Dashboard JSON Model, Prometheus PromQL

**Design spec:** `docs/superpowers/specs/2026-04-01-grafana-dashboard-design.md`

---

## File Structure

- **Create:** `docs/grafana-dashboard.json` — Complete Grafana Dashboard JSON (replaces the missing file referenced in documentation)

---

## Key Conventions

The following conventions are reused in every Task and will not be repeated:

### Currency Conversion PromQL Pattern

All monetary panels use two queries:
- Query A (USD): `<base expression> / 500000`
- Query B (CNY): `<base expression> / 500000 * $exchange_rate`

Visibility is controlled via Grafana panel `overrides` based on the `$currency` variable, or using a single PromQL:
```text
<base expression> / 500000 * (1 + ($currency == "CNY") * ($exchange_rate - 1))
```

Since PromQL does not support string comparison, the actual implementation uses two queries + hide conditions.

### Panel Size Grid

Grafana 24-column grid:
- Stat cards: `w:4, h:4` (6 per row)
- Time series / Bar charts: `w:12, h:8` (2 per row)
- Pie charts: `w:8, h:8` (3 per row)
- Tables: `w:24, h:8` (full width)
- Gauge: `w:6, h:6`

### Common datasource

```json
{ "type": "prometheus", "uid": "${DS_PROMETHEUS}" }
```

All panels use this value for the `datasource` field, allowing dynamic binding on import.

---

## Task 1: Dashboard Skeleton and Top-Level Variables

**Files:**
- Create: `docs/grafana-dashboard.json`

- [ ] **Step 1: Create Dashboard JSON skeleton**

Create `docs/grafana-dashboard.json` with the following top-level structure:

```json
{
  "__inputs": [
    {
      "name": "DS_PROMETHEUS",
      "label": "Prometheus",
      "description": "Prometheus data source",
      "type": "datasource",
      "pluginId": "prometheus",
      "pluginName": "Prometheus"
    }
  ],
  "__requires": [
    { "type": "grafana", "id": "grafana", "name": "Grafana", "version": "10.0.0" },
    { "type": "datasource", "id": "prometheus", "name": "Prometheus", "version": "1.0.0" },
    { "type": "panel", "id": "timeseries", "name": "Time series", "version": "" },
    { "type": "panel", "id": "stat", "name": "Stat", "version": "" },
    { "type": "panel", "id": "gauge", "name": "Gauge", "version": "" },
    { "type": "panel", "id": "barchart", "name": "Bar chart", "version": "" },
    { "type": "panel", "id": "piechart", "name": "Pie chart", "version": "" },
    { "type": "panel", "id": "table", "name": "Table", "version": "" }
  ],
  "annotations": { "list": [] },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [],
  "schemaVersion": 39,
  "tags": ["one-api", "llm-gateway"],
  "templating": { "list": [] },
  "time": { "from": "now-6h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "One API Dashboard",
  "uid": "one-api-main",
  "version": 1
}
```

- [ ] **Step 2: Add 7 top-level variables to `templating.list`**

Add the following variable definitions in order to the `templating.list` array:

```json
[
  {
    "name": "channel_id",
    "label": "Channel ID",
    "type": "query",
    "query": "label_values(one_api_relay_requests_total, channel_id)",
    "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "refresh": 2,
    "includeAll": true,
    "multi": true,
    "allValue": ".*",
    "current": { "selected": true, "text": "All", "value": "$__all" },
    "sort": 1
  },
  {
    "name": "channel_type",
    "label": "Channel Type",
    "type": "query",
    "query": "label_values(one_api_relay_requests_total, channel_type)",
    "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "refresh": 2,
    "includeAll": true,
    "multi": true,
    "allValue": ".*",
    "current": { "selected": true, "text": "All", "value": "$__all" },
    "sort": 1
  },
  {
    "name": "model",
    "label": "Model",
    "type": "query",
    "query": "label_values(one_api_relay_requests_total, model)",
    "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "refresh": 2,
    "includeAll": true,
    "multi": true,
    "allValue": ".*",
    "current": { "selected": true, "text": "All", "value": "$__all" },
    "sort": 1
  },
  {
    "name": "username",
    "label": "Username",
    "type": "query",
    "query": "label_values(one_api_user_requests_total, username)",
    "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "refresh": 2,
    "includeAll": true,
    "multi": true,
    "allValue": ".*",
    "current": { "selected": true, "text": "All", "value": "$__all" },
    "sort": 3
  },
  {
    "name": "group",
    "label": "User Group",
    "type": "query",
    "query": "label_values(one_api_user_requests_total, group)",
    "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
    "refresh": 2,
    "includeAll": true,
    "multi": true,
    "allValue": ".*",
    "current": { "selected": true, "text": "All", "value": "$__all" },
    "sort": 1
  },
  {
    "name": "currency",
    "label": "Currency",
    "type": "custom",
    "query": "USD,CNY",
    "current": { "selected": true, "text": "USD", "value": "USD" },
    "includeAll": false,
    "multi": false
  },
  {
    "name": "exchange_rate",
    "label": "Exchange Rate",
    "type": "textbox",
    "query": "8",
    "current": { "text": "8", "value": "8" }
  }
]
```

- [ ] **Step 3: Validate JSON**

Run: `python3 -c "import json; json.load(open('docs/grafana-dashboard.json'))"`

Expected: no output (successful parse)

- [ ] **Step 4: Commit**

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard skeleton with template variables"
```

---

## Task 2: Row 1 — Overview

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append Row + 8 panels to the `panels` array

- [ ] **Step 1: Add Row 1 header and 6 Stat cards**

Append the following panels to the `panels` array (`id` starts at 1 and increments):

**Row panel:**
```json
{
  "id": 1,
  "type": "row",
  "title": "Overview",
  "collapsed": false,
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
  "panels": []
}
```

**Stat 1 — System Uptime** (gridPos: x:0, y:1, w:4, h:4):
```json
{
  "id": 2,
  "type": "stat",
  "title": "System Uptime",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 0, "y": 1 },
  "targets": [{
    "expr": "time() - one_api_system_start_time_seconds",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "s",
      "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Stat 2 — Total Request Rate** (x:4, y:1, w:4, h:4):
```json
{
  "id": 3,
  "type": "stat",
  "title": "Total Request Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 4, "y": 1 },
  "targets": [{
    "expr": "sum(rate(one_api_http_requests_total[$__rate_interval]))",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "reqps",
      "decimals": 2,
      "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "area" }
}
```

**Stat 3 — Relay Success Rate** (x:8, y:1, w:4, h:4):
```json
{
  "id": 4,
  "type": "stat",
  "title": "Relay Success Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 8, "y": 1 },
  "targets": [{
    "expr": "sum(rate(one_api_relay_requests_total{success=\"true\"}[$__rate_interval])) / sum(rate(one_api_relay_requests_total[$__rate_interval]))",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "decimals": 2,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "red", "value": null },
          { "color": "yellow", "value": 0.95 },
          { "color": "green", "value": 0.99 }
        ]
      }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Stat 4 — Active Users** (x:12, y:1, w:4, h:4):
```json
{
  "id": 5,
  "type": "stat",
  "title": "Active Users",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 12, "y": 1 },
  "targets": [{
    "expr": "one_api_site_active_users",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "none",
      "thresholds": { "mode": "absolute", "steps": [{ "color": "blue", "value": null }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Stat 5 — Site-wide Quota Usage** (x:16, y:1, w:4, h:4):
```json
{
  "id": 6,
  "type": "stat",
  "title": "Site-wide Quota Usage",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 16, "y": 1 },
  "targets": [{
    "expr": "one_api_site_used_quota / one_api_site_total_quota",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "decimals": 2,
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "green", "value": null },
          { "color": "yellow", "value": 0.6 },
          { "color": "red", "value": 0.8 }
        ]
      }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "background", "graphMode": "none" }
}
```

**Stat 6 — Auth Failure Rate** (x:20, y:1, w:4, h:4):
```json
{
  "id": 7,
  "type": "stat",
  "title": "Auth Failure Rate",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 20, "y": 1 },
  "targets": [{
    "expr": "sum(rate(one_api_token_auth_attempts_total{success=\"false\"}[$__rate_interval])) / sum(rate(one_api_token_auth_attempts_total[$__rate_interval]))",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "decimals": 2,
      "noValue": "No data",
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "green", "value": null },
          { "color": "yellow", "value": 0.01 },
          { "color": "red", "value": 0.05 }
        ]
      }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

- [ ] **Step 2: Add 2 time series panels**

**Time Series 1 — HTTP Request Rate (by Status Code)** (x:0, y:5, w:12, h:8):
```json
{
  "id": 8,
  "type": "timeseries",
  "title": "HTTP Request Rate (by Status Code)",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 5 },
  "targets": [{
    "expr": "sum(rate(one_api_http_requests_total[$__rate_interval])) by (status_code)",
    "legendFormat": "{{status_code}}",
    "refId": "A"
  }],
  "fieldConfig": {
    "defaults": {
      "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10, "pointSize": 5, "stacking": { "mode": "none" } },
      "unit": "reqps"
    },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Time Series 2 — Error Overview** (x:12, y:5, w:12, h:8):
```json
{
  "id": 9,
  "type": "timeseries",
  "title": "Error Overview",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 5 },
  "targets": [{
    "expr": "sum(rate(one_api_errors_total[$__rate_interval])) by (component)",
    "legendFormat": "{{component}}",
    "refId": "A"
  }],
  "fieldConfig": {
    "defaults": {
      "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 },
      "unit": "short",
      "noValue": "No data"
    },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

- [ ] **Step 3: Validate JSON**

Run: `python3 -c "import json; d=json.load(open('docs/grafana-dashboard.json')); print(f'panels: {len(d[\"panels\"])}')"` 

Expected: `panels: 9` (1 Row + 6 Stat + 2 time series)

- [ ] **Step 4: Commit**

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 1 - Overview panels"
```

---

## Task 3: Row 2 — Relay Requests

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append Row + 7 panels to the `panels` array

- [ ] **Step 1: Add Row 2 header**

```json
{
  "id": 10,
  "type": "row",
  "title": "Relay Requests",
  "collapsed": true,
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 13 },
  "panels": [...]
}
```

Row 2 has `collapsed: true`, all child panels are placed inside this Row's `panels` array. Child panel `gridPos.y` starts from 14.

- [ ] **Step 2: Add Relay QPS time series**

id:11, x:0, y:14, w:12, h:8
```json
{
  "id": 11,
  "type": "timeseries",
  "title": "Relay QPS",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 14 },
  "targets": [{
    "expr": "sum(rate(one_api_relay_requests_total{channel_id=~\"$channel_id\",channel_type=~\"$channel_type\",model=~\"$model\"}[$__rate_interval])) by (model)",
    "legendFormat": "{{model}}",
    "refId": "A"
  }],
  "fieldConfig": {
    "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "reqps" },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max", "last"] } }
}
```

- [ ] **Step 3: Add Relay Success Rate time series**

id:12, x:12, y:14, w:12, h:8
```json
{
  "id": 12,
  "type": "timeseries",
  "title": "Relay Success Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 14 },
  "targets": [{
    "expr": "sum(rate(one_api_relay_requests_total{channel_id=~\"$channel_id\",model=~\"$model\",success=\"true\"}[$__rate_interval])) by (model) / sum(rate(one_api_relay_requests_total{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (model)",
    "legendFormat": "{{model}}",
    "refId": "A"
  }],
  "fieldConfig": {
    "defaults": {
      "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 },
      "unit": "percentunit",
      "min": 0, "max": 1,
      "thresholds": { "mode": "absolute", "steps": [{ "color": "red", "value": null }, { "color": "yellow", "value": 0.95 }, { "color": "green", "value": 0.99 }] }
    },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "min", "last"] } }
}
```

- [ ] **Step 4: Add Relay Latency P50/P95/P99 time series**

id:13, x:0, y:22, w:24, h:8 (full width, 3 percentile lines)
```json
{
  "id": 13,
  "type": "timeseries",
  "title": "Relay Latency P50/P95/P99",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 22 },
  "targets": [
    {
      "expr": "histogram_quantile(0.5, sum(rate(one_api_relay_request_duration_seconds_bucket{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (le, model))",
      "legendFormat": "{{model}} P50",
      "refId": "A"
    },
    {
      "expr": "histogram_quantile(0.95, sum(rate(one_api_relay_request_duration_seconds_bucket{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (le, model))",
      "legendFormat": "{{model}} P95",
      "refId": "B"
    },
    {
      "expr": "histogram_quantile(0.99, sum(rate(one_api_relay_request_duration_seconds_bucket{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (le, model))",
      "legendFormat": "{{model}} P99",
      "refId": "C"
    }
  ],
  "fieldConfig": {
    "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 5 }, "unit": "s" },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

- [ ] **Step 5: Add Token Consumption Rate and Quota Consumption Rate time series**

**Token Consumption Rate** id:14, x:0, y:30, w:12, h:8:
```json
{
  "id": 14,
  "type": "timeseries",
  "title": "Token Consumption Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 30 },
  "targets": [{
    "expr": "sum(rate(one_api_relay_tokens_total{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (model, token_type)",
    "legendFormat": "{{model}}-{{token_type}}",
    "refId": "A"
  }],
  "fieldConfig": {
    "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10, "stacking": { "mode": "normal" } }, "unit": "short" },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Quota Consumption Rate** id:15, x:12, y:30, w:12, h:8:
```json
{
  "id": 15,
  "type": "timeseries",
  "title": "Quota Consumption Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 30 },
  "targets": [
    {
      "expr": "sum(rate(one_api_relay_quota_used_total{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (model) / 500000",
      "legendFormat": "{{model}} (USD)",
      "refId": "A",
      "hide": false
    },
    {
      "expr": "sum(rate(one_api_relay_quota_used_total{channel_id=~\"$channel_id\",model=~\"$model\"}[$__rate_interval])) by (model) / 500000 * $exchange_rate",
      "legendFormat": "{{model}} (CNY)",
      "refId": "B",
      "hide": false
    }
  ],
  "fieldConfig": {
    "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "currencyUSD" },
    "overrides": []
  },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } },
  "transformations": []
}
```

Note: The Quota consumption panel provides both USD and CNY queries; users select which to focus on based on the `$currency` variable.

- [ ] **Step 6: Add 2 pie charts**

**Request Distribution (by Channel Type)** id:16, x:0, y:38, w:12, h:8:
```json
{
  "id": 16,
  "type": "piechart",
  "title": "Request Distribution (by Channel Type)",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 38 },
  "targets": [{
    "expr": "sum(one_api_relay_requests_total{channel_id=~\"$channel_id\"}) by (channel_type)",
    "legendFormat": "{{channel_type}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": {}, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "legend": { "displayMode": "table", "placement": "right", "values": ["value", "percent"] }, "pieType": "donut" }
}
```

**Request Distribution (by API Format)** id:17, x:12, y:38, w:12, h:8:
```json
{
  "id": 17,
  "type": "piechart",
  "title": "Request Distribution (by API Format)",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 38 },
  "targets": [{
    "expr": "sum(one_api_relay_requests_total{channel_id=~\"$channel_id\"}) by (api_format)",
    "legendFormat": "{{api_format}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": {}, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "legend": { "displayMode": "table", "placement": "right", "values": ["value", "percent"] }, "pieType": "donut" }
}
```

- [ ] **Step 7: Validate JSON and commit**

Run: `python3 -c "import json; d=json.load(open('docs/grafana-dashboard.json')); print(f'panels: {len(d[\"panels\"])}')"` 

Expected: `panels: 10` (9 + 1 collapsed Row, where Row 2 contains 7 child panels)

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 2 - Relay panels"
```

---

## Task 4: Row 3 — Channel Health

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append collapsed Row + 6 panels to the `panels` array

- [ ] **Step 1: Add Row 3 header (collapsed) and 6 child panels**

Row id:18, y:46. Child panels id:19-24.

**Channel Status Table** id:19, x:0, y:47, w:24, h:8, type: table:
```json
{
  "id": 19,
  "type": "table",
  "title": "Channel Status Table",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 47 },
  "targets": [{
    "expr": "one_api_channel_status",
    "refId": "A",
    "instant": true,
    "format": "table"
  }],
  "fieldConfig": {
    "defaults": { "noValue": "No data" },
    "overrides": [
      {
        "matcher": { "id": "byName", "options": "Value" },
        "properties": [{
          "id": "custom.cellOptions",
          "value": { "type": "color-background" }
        }, {
          "id": "thresholds",
          "value": { "mode": "absolute", "steps": [{ "color": "red", "value": null }, { "color": "yellow", "value": -1 }, { "color": "red", "value": 0 }, { "color": "green", "value": 1 }] }
        }]
      }
    ]
  },
  "options": { "showHeader": true },
  "transformations": [{ "id": "organize", "options": { "excludeByName": { "Time": true, "__name__": true }, "renameByName": { "channel_id": "Channel ID", "channel_name": "Channel Name", "channel_type": "Channel Type", "Value": "Status" } } }]
}
```

**Channel In-Flight Requests** id:20, x:0, y:55, w:12, h:8:
```json
{
  "id": 20,
  "type": "timeseries",
  "title": "Channel In-Flight Requests",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 55 },
  "targets": [{
    "expr": "one_api_channel_requests_in_flight{channel_id=~\"$channel_id\"}",
    "legendFormat": "ch{{channel_id}}-{{channel_type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max", "last"] } }
}
```

**Channel Response Time** id:21, x:12, y:55, w:12, h:8:
```json
{
  "id": 21,
  "type": "timeseries",
  "title": "Channel Response Time",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 55 },
  "targets": [{
    "expr": "one_api_channel_response_time_ms{channel_id=~\"$channel_id\"}",
    "legendFormat": "ch{{channel_id}}-{{channel_type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ms", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Channel Success Rate** id:22, x:0, y:63, w:12, h:8:
```json
{
  "id": 22,
  "type": "timeseries",
  "title": "Channel Success Rate",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 63 },
  "targets": [{
    "expr": "one_api_channel_success_rate{channel_id=~\"$channel_id\"}",
    "legendFormat": "ch{{channel_id}}-{{channel_type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "percentunit", "min": 0, "max": 1, "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "min"] } }
}
```

**Channel Balance** id:23, x:12, y:63, w:12, h:8:
```json
{
  "id": 23,
  "type": "stat",
  "title": "Channel Balance",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 63 },
  "targets": [
    {
      "expr": "one_api_channel_balance_usd{channel_id=~\"$channel_id\"}",
      "legendFormat": "ch{{channel_id}} (USD)",
      "refId": "A"
    },
    {
      "expr": "one_api_channel_balance_usd{channel_id=~\"$channel_id\"} * $exchange_rate",
      "legendFormat": "ch{{channel_id}} (CNY)",
      "refId": "B"
    }
  ],
  "fieldConfig": { "defaults": { "unit": "currencyUSD", "noValue": "No data" }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none", "textMode": "auto" }
}
```

**Channel Request Volume Top10** id:24, x:0, y:71, w:24, h:8:
```json
{
  "id": 24,
  "type": "barchart",
  "title": "Channel Request Volume Top10",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 71 },
  "targets": [{
    "expr": "topk(10, sum(rate(one_api_relay_requests_total{channel_id=~\"$channel_id\"}[$__rate_interval])) by (channel_id, channel_type))",
    "legendFormat": "ch{{channel_id}}-{{channel_type}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": { "unit": "reqps" }, "overrides": [] },
  "options": { "orientation": "horizontal", "showValue": "always", "stacking": "none", "legend": { "displayMode": "list", "placement": "right" } }
}
```

- [ ] **Step 2: Validate JSON and commit**

Run: `python3 -c "import json; d=json.load(open('docs/grafana-dashboard.json')); print(f'panels: {len(d[\"panels\"])}')"` 

Expected: `panels: 11` (previous 10 + 1 collapsed Row 3, containing 6 child panels)

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 3 - Channel Health panels"
```

---

## Task 5: Row 4 — Users & Quota

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append collapsed Row + 8 panels to the `panels` array

- [ ] **Step 1: Add Row 4 header (collapsed) and 8 child panels**

Row id:25, y:79. Child panels id:26-33.

**Site-wide Quota Usage (Gauge)** id:26, x:0, y:80, w:6, h:6:
```json
{
  "id": 26,
  "type": "gauge",
  "title": "Site-wide Quota Usage",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 6, "w": 6, "x": 0, "y": 80 },
  "targets": [{
    "expr": "one_api_site_used_quota / one_api_site_total_quota",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "min": 0, "max": 1,
      "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }, { "color": "yellow", "value": 0.6 }, { "color": "red", "value": 0.8 }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "showThresholdMarkers": true, "showThresholdLabels": false }
}
```

**Site-wide Used Quota** id:27, x:6, y:80, w:6, h:6:
```json
{
  "id": 27,
  "type": "stat",
  "title": "Site-wide Used Quota",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 6, "w": 6, "x": 6, "y": 80 },
  "targets": [
    { "expr": "one_api_site_used_quota / 500000", "legendFormat": "USD", "refId": "A", "instant": true },
    { "expr": "one_api_site_used_quota / 500000 * $exchange_rate", "legendFormat": "CNY", "refId": "B", "instant": true }
  ],
  "fieldConfig": { "defaults": { "unit": "currencyUSD", "decimals": 2 }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Site-wide Total Quota** id:28, x:12, y:80, w:6, h:6:
```json
{
  "id": 28,
  "type": "stat",
  "title": "Site-wide Total Quota",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 6, "w": 6, "x": 12, "y": 80 },
  "targets": [
    { "expr": "one_api_site_total_quota / 500000", "legendFormat": "USD", "refId": "A", "instant": true },
    { "expr": "one_api_site_total_quota / 500000 * $exchange_rate", "legendFormat": "CNY", "refId": "B", "instant": true }
  ],
  "fieldConfig": { "defaults": { "unit": "currencyUSD", "decimals": 2 }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**User Group Distribution (Pie Chart)** id:29, x:18, y:80, w:6, h:6:
```json
{
  "id": 29,
  "type": "piechart",
  "title": "User Group Distribution",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 6, "w": 6, "x": 18, "y": 80 },
  "targets": [{
    "expr": "sum(one_api_user_requests_total) by (group)",
    "legendFormat": "{{group}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": {}, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "legend": { "displayMode": "table", "placement": "right", "values": ["value", "percent"] }, "pieType": "donut" }
}
```

**User Request Volume Top10** id:30, x:0, y:86, w:12, h:8:
```json
{
  "id": 30,
  "type": "barchart",
  "title": "User Request Volume Top10",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 86 },
  "targets": [{
    "expr": "topk(10, sum(one_api_user_requests_total{username=~\"$username\",group=~\"$group\"}) by (username, group))",
    "legendFormat": "{{username}}({{group}})",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": { "unit": "short" }, "overrides": [] },
  "options": { "orientation": "horizontal", "showValue": "always", "legend": { "displayMode": "list", "placement": "right" } }
}
```

**User Quota Consumption Top10** id:31, x:12, y:86, w:12, h:8:
```json
{
  "id": 31,
  "type": "barchart",
  "title": "User Quota Consumption Top10",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 86 },
  "targets": [
    {
      "expr": "topk(10, sum(rate(one_api_billing_quota_processed_total{username=~\"$username\"}[$__rate_interval])) by (username)) / 500000",
      "legendFormat": "{{username}} (USD)",
      "refId": "A",
      "instant": true
    },
    {
      "expr": "topk(10, sum(rate(one_api_billing_quota_processed_total{username=~\"$username\"}[$__rate_interval])) by (username)) / 500000 * $exchange_rate",
      "legendFormat": "{{username}} (CNY)",
      "refId": "B",
      "instant": true
    }
  ],
  "fieldConfig": { "defaults": { "unit": "currencyUSD" }, "overrides": [] },
  "options": { "orientation": "horizontal", "showValue": "always", "legend": { "displayMode": "list", "placement": "right" } }
}
```

**User Request Rate Trend** id:32, x:0, y:94, w:12, h:8:
```json
{
  "id": 32,
  "type": "timeseries",
  "title": "User Request Rate Trend",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 94 },
  "targets": [{
    "expr": "sum(rate(one_api_user_requests_total{username=~\"$username\",group=~\"$group\"}[$__rate_interval])) by (username, group)",
    "legendFormat": "{{username}}({{group}})",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "reqps" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**User Token Consumption Trend** id:33, x:12, y:94, w:12, h:8:
```json
{
  "id": 33,
  "type": "timeseries",
  "title": "User Token Consumption Trend",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 94 },
  "targets": [{
    "expr": "sum(rate(one_api_user_tokens_total{username=~\"$username\"}[$__rate_interval])) by (username, token_type)",
    "legendFormat": "{{username}}-{{token_type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10, "stacking": { "mode": "normal" } }, "unit": "short" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

- [ ] **Step 2: Validate JSON and commit**

Run: `python3 -c "import json; d=json.load(open('docs/grafana-dashboard.json')); print(f'panels: {len(d[\"panels\"])}')"` 

Expected: `panels: 12` (previous 11 + 1 collapsed Row 4, containing 8 child panels)

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 4 - Users & Quota panels"
```

---

## Task 6: Row 5 — Billing

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append collapsed Row + 7 panels to the `panels` array

- [ ] **Step 1: Add Row 5 header (collapsed) and 7 child panels**

Row id:34, y:102. Child panels id:35-41.

**Billing Operation Rate** id:35, x:0, y:103, w:12, h:8:
```json
{
  "id": 35,
  "type": "timeseries",
  "title": "Billing Operation Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 103 },
  "targets": [{
    "expr": "sum(rate(one_api_billing_operations_total[$__rate_interval])) by (operation, success)",
    "legendFormat": "{{operation}}-{{success}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Billing P95 Latency** id:36, x:12, y:103, w:12, h:8:
```json
{
  "id": 36,
  "type": "timeseries",
  "title": "Billing P95 Latency",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 103 },
  "targets": [{
    "expr": "histogram_quantile(0.95, sum(rate(one_api_billing_operation_duration_seconds_bucket[$__rate_interval])) by (le, operation))",
    "legendFormat": "{{operation}} P95",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "s" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Billing Success Rate** id:37, x:0, y:111, w:4, h:4:
```json
{
  "id": 37,
  "type": "stat",
  "title": "Billing Success Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 0, "y": 111 },
  "targets": [{
    "expr": "sum(rate(one_api_billing_operations_total{success=\"true\"}[$__rate_interval])) / sum(rate(one_api_billing_operations_total[$__rate_interval]))",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit", "decimals": 2,
      "thresholds": { "mode": "absolute", "steps": [{ "color": "red", "value": null }, { "color": "yellow", "value": 0.95 }, { "color": "green", "value": 0.99 }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Billing Statistics Overview (3 Stat cards)** id:38-40, x:4/8/12, y:111, w:4, h:4:
```json
{
  "id": 38,
  "type": "stat",
  "title": "Total Billing Operations",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 4, "y": 111 },
  "targets": [{ "expr": "one_api_billing_stats{stat_type=\"total_operations\"}", "refId": "A", "instant": true }],
  "fieldConfig": { "defaults": { "unit": "short", "noValue": "No data" }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```
```json
{
  "id": 39,
  "type": "stat",
  "title": "Successful Billing Operations",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 8, "y": 111 },
  "targets": [{ "expr": "one_api_billing_stats{stat_type=\"successful_operations\"}", "refId": "A", "instant": true }],
  "fieldConfig": { "defaults": { "unit": "short", "noValue": "No data", "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] } }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```
```json
{
  "id": 40,
  "type": "stat",
  "title": "Failed Billing Operations",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 12, "y": 111 },
  "targets": [{ "expr": "one_api_billing_stats{stat_type=\"failed_operations\"}", "refId": "A", "instant": true }],
  "fieldConfig": { "defaults": { "unit": "short", "noValue": "No data", "thresholds": { "mode": "absolute", "steps": [{ "color": "red", "value": null }] } }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Billing Timeouts** id:41, x:16, y:111, w:8, h:4:
```json
{
  "id": 41,
  "type": "timeseries",
  "title": "Billing Timeouts",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 8, "x": 16, "y": 111 },
  "targets": [{
    "expr": "sum(rate(one_api_billing_timeouts_total[$__rate_interval])) by (model_name)",
    "legendFormat": "{{model_name}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Billing Error Distribution** id:42, x:0, y:115, w:12, h:8:
```json
{
  "id": 42,
  "type": "timeseries",
  "title": "Billing Error Distribution",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 115 },
  "targets": [{
    "expr": "sum(rate(one_api_billing_errors_total[$__rate_interval])) by (error_type, operation)",
    "legendFormat": "{{error_type}}-{{operation}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Quota Processed Amount Trend** id:43, x:12, y:115, w:12, h:8:
```json
{
  "id": 43,
  "type": "timeseries",
  "title": "Quota Processed Amount Trend",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 115 },
  "targets": [
    {
      "expr": "sum(rate(one_api_billing_quota_processed_total[$__rate_interval])) by (model_name) / 500000",
      "legendFormat": "{{model_name}} (USD)",
      "refId": "A"
    },
    {
      "expr": "sum(rate(one_api_billing_quota_processed_total[$__rate_interval])) by (model_name) / 500000 * $exchange_rate",
      "legendFormat": "{{model_name}} (CNY)",
      "refId": "B"
    }
  ],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "currencyUSD" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

- [ ] **Step 2: Validate JSON and commit**

Run the validation command then:

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 5 - Billing panels"
```

---

## Task 7: Row 6 — Infrastructure DB & Redis

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append collapsed Row + 8 panels to the `panels` array

- [ ] **Step 1: Add Row 6 header (collapsed) and 8 child panels**

Row id:44, y:123. Child panels id:45-52.

**DB Connection Pool** id:45, x:0, y:124, w:12, h:8:
```json
{
  "id": 45,
  "type": "timeseries",
  "title": "DB Connection Pool",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 124 },
  "targets": [
    { "expr": "one_api_db_connections_in_use", "legendFormat": "In Use", "refId": "A" },
    { "expr": "one_api_db_connections_idle", "legendFormat": "Idle", "refId": "B" }
  ],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**DB Query Rate** id:46, x:12, y:124, w:12, h:8:
```json
{
  "id": 46,
  "type": "timeseries",
  "title": "DB Query Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 124 },
  "targets": [{
    "expr": "sum(rate(one_api_db_queries_total[$__rate_interval])) by (operation, table)",
    "legendFormat": "{{operation}}-{{table}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**DB Query P95 Latency** id:47, x:0, y:132, w:12, h:8:
```json
{
  "id": 47,
  "type": "timeseries",
  "title": "DB Query P95 Latency",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 132 },
  "targets": [{
    "expr": "histogram_quantile(0.95, sum(rate(one_api_db_query_duration_seconds_bucket[$__rate_interval])) by (le, table))",
    "legendFormat": "{{table}} P95",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "s" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**DB Query Failure Rate** id:48, x:12, y:132, w:12, h:8:
```json
{
  "id": 48,
  "type": "timeseries",
  "title": "DB Query Failure Rate",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 132 },
  "targets": [{
    "expr": "sum(rate(one_api_db_queries_total{success=\"false\"}[$__rate_interval])) by (table)",
    "legendFormat": "{{table}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**DB Hot Tables Top5** id:49, x:0, y:140, w:24, h:8:
```json
{
  "id": 49,
  "type": "barchart",
  "title": "DB Hot Tables Top5",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 140 },
  "targets": [{
    "expr": "topk(5, sum(rate(one_api_db_queries_total[$__rate_interval])) by (table))",
    "legendFormat": "{{table}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": { "unit": "ops" }, "overrides": [] },
  "options": { "orientation": "horizontal", "showValue": "always", "legend": { "displayMode": "list", "placement": "right" } }
}
```

**Redis Active Connections** id:50, x:0, y:148, w:8, h:8:
```json
{
  "id": 50,
  "type": "timeseries",
  "title": "Redis Active Connections",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 0, "y": 148 },
  "targets": [{ "expr": "one_api_redis_connections_active", "legendFormat": "Active Connections", "refId": "A" }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short" }, "overrides": [] },
  "options": { "tooltip": { "mode": "single" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Redis Command Rate** id:51, x:8, y:148, w:8, h:8:
```json
{
  "id": 51,
  "type": "timeseries",
  "title": "Redis Command Rate",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 8, "y": 148 },
  "targets": [{
    "expr": "sum(rate(one_api_redis_commands_total[$__rate_interval])) by (command)",
    "legendFormat": "{{command}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Redis Command P95 Latency** id:52, x:16, y:148, w:8, h:8:
```json
{
  "id": 52,
  "type": "timeseries",
  "title": "Redis Command P95 Latency",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 16, "y": 148 },
  "targets": [{
    "expr": "histogram_quantile(0.95, sum(rate(one_api_redis_command_duration_seconds_bucket[$__rate_interval])) by (le, command))",
    "legendFormat": "{{command}} P95",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "s", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

- [ ] **Step 2: Validate JSON and commit**

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 6 - Infrastructure panels"
```

---

## Task 8: Row 7 — Security & Rate Limiting + Go Runtime

**Files:**
- Modify: `docs/grafana-dashboard.json` — Append collapsed Row + 10 panels to the `panels` array

- [ ] **Step 1: Add Row 7 header (collapsed) and 10 child panels**

Row id:53, y:156. Child panels id:54-63.

**Auth Success/Failure Trend** id:54, x:0, y:157, w:12, h:8:
```json
{
  "id": 54,
  "type": "timeseries",
  "title": "Auth Success/Failure Trend",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 157 },
  "targets": [{
    "expr": "sum(rate(one_api_token_auth_attempts_total[$__rate_interval])) by (success)",
    "legendFormat": "success={{success}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Auth Failure Rate** id:55, x:12, y:157, w:4, h:4:
```json
{
  "id": 55,
  "type": "stat",
  "title": "Auth Failure Rate",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 4, "w": 4, "x": 12, "y": 157 },
  "targets": [{
    "expr": "sum(rate(one_api_token_auth_attempts_total{success=\"false\"}[$__rate_interval])) / sum(rate(one_api_token_auth_attempts_total[$__rate_interval]))",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit", "decimals": 2, "noValue": "No data",
      "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }, { "color": "yellow", "value": 0.01 }, { "color": "red", "value": 0.05 }] }
    },
    "overrides": []
  },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "none" }
}
```

**Rate Limit Trigger Rate** id:56, x:16, y:157, w:8, h:8:
```json
{
  "id": 56,
  "type": "timeseries",
  "title": "Rate Limit Trigger Rate",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 16, "y": 157 },
  "targets": [{
    "expr": "sum(rate(one_api_rate_limit_hits_total[$__rate_interval])) by (type)",
    "legendFormat": "{{type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Error Distribution** id:57, x:0, y:165, w:12, h:8:
```json
{
  "id": 57,
  "type": "timeseries",
  "title": "Error Distribution",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 165 },
  "targets": [{
    "expr": "sum(rate(one_api_errors_total[$__rate_interval])) by (error_type, component)",
    "legendFormat": "{{component}}-{{error_type}}",
    "refId": "A"
  }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "ops", "noValue": "No data" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi", "sort": "desc" }, "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["mean", "max"] } }
}
```

**Error Top5 Components** id:58, x:12, y:165, w:12, h:8:
```json
{
  "id": 58,
  "type": "barchart",
  "title": "Error Top5 Components",
  "description": "Reserved: no data available yet",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 165 },
  "targets": [{
    "expr": "topk(5, sum(rate(one_api_errors_total[$__rate_interval])) by (component))",
    "legendFormat": "{{component}}",
    "refId": "A",
    "instant": true
  }],
  "fieldConfig": { "defaults": { "unit": "ops", "noValue": "No data" }, "overrides": [] },
  "options": { "orientation": "horizontal", "showValue": "always", "legend": { "displayMode": "list", "placement": "right" } }
}
```

**Goroutines** id:59, x:0, y:173, w:8, h:8:
```json
{
  "id": 59,
  "type": "timeseries",
  "title": "Goroutines",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 0, "y": 173 },
  "targets": [{ "expr": "go_goroutines", "legendFormat": "goroutines", "refId": "A" }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "short" }, "overrides": [] },
  "options": { "tooltip": { "mode": "single" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Heap Memory** id:60, x:8, y:173, w:8, h:8:
```json
{
  "id": 60,
  "type": "timeseries",
  "title": "Heap Memory",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 8, "y": 173 },
  "targets": [
    { "expr": "go_memstats_heap_inuse_bytes", "legendFormat": "In Use", "refId": "A" },
    { "expr": "go_memstats_heap_idle_bytes", "legendFormat": "Idle", "refId": "B" }
  ],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10, "stacking": { "mode": "normal" } }, "unit": "bytes" }, "overrides": [] },
  "options": { "tooltip": { "mode": "multi" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**GC Pause Time** id:61, x:16, y:173, w:8, h:8:
```json
{
  "id": 61,
  "type": "timeseries",
  "title": "GC Pause Time",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 8, "x": 16, "y": 173 },
  "targets": [{ "expr": "rate(go_gc_duration_seconds_sum[$__rate_interval])", "legendFormat": "GC Duration", "refId": "A" }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "s" }, "overrides": [] },
  "options": { "tooltip": { "mode": "single" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Process CPU** id:62, x:0, y:181, w:12, h:8:
```json
{
  "id": 62,
  "type": "timeseries",
  "title": "Process CPU",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 181 },
  "targets": [{ "expr": "rate(process_cpu_seconds_total[$__rate_interval])", "legendFormat": "CPU Usage", "refId": "A" }],
  "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "lineWidth": 2, "fillOpacity": 10 }, "unit": "percentunit" }, "overrides": [] },
  "options": { "tooltip": { "mode": "single" }, "legend": { "displayMode": "list", "placement": "bottom" } }
}
```

**Process RSS Memory** id:63, x:12, y:181, w:12, h:8:
```json
{
  "id": 63,
  "type": "stat",
  "title": "Process RSS Memory",
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 181 },
  "targets": [{ "expr": "process_resident_memory_bytes", "refId": "A", "instant": true }],
  "fieldConfig": { "defaults": { "unit": "bytes", "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] } }, "overrides": [] },
  "options": { "reduceOptions": { "calcs": ["lastNotNull"] }, "colorMode": "value", "graphMode": "area" }
}
```

- [ ] **Step 2: Validate JSON and commit**

Run: `python3 -c "import json; d=json.load(open('docs/grafana-dashboard.json')); total=len(d['panels']); collapsed=[p for p in d['panels'] if p.get('collapsed')]; inner=sum(len(p.get('panels',[])) for p in collapsed); print(f'top-level: {total}, collapsed inner: {inner}, total panels: {total+inner}')"` 

Expected: `top-level: 16, collapsed inner: 49, total panels: 65` (16 = 1 Row (expanded) + 8 panels + 6 collapsed Rows + 1 expanded panel... exact number depends on structure, the key point is JSON parses successfully and panels are not empty)

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: add Grafana dashboard Row 7 - Security & Go Runtime panels"
```

---

## Task 9: Final Validation and Cleanup

**Files:**
- Modify: `docs/grafana-dashboard.json` — Final check

- [ ] **Step 1: Validate complete JSON structure**

```bash
python3 -c "
import json
with open('docs/grafana-dashboard.json') as f:
    d = json.load(f)
print(f'Title: {d[\"title\"]}')
print(f'Variables: {len(d[\"templating\"][\"list\"])}')
top = d['panels']
print(f'Top-level panels: {len(top)}')
rows = [p for p in top if p['type'] == 'row']
print(f'Rows: {len(rows)}')
collapsed = [r for r in rows if r.get('collapsed')]
inner = sum(len(r.get('panels', [])) for r in collapsed)
non_row_top = len([p for p in top if p['type'] != 'row'])
print(f'Non-row top panels: {non_row_top}')
print(f'Collapsed inner panels: {inner}')
print(f'Total panels (excl rows): {non_row_top + inner}')
ids = []
for p in top:
    ids.append(p['id'])
    for sub in p.get('panels', []):
        ids.append(sub['id'])
print(f'Unique IDs: {len(set(ids))}, duplicates: {len(ids) - len(set(ids))}')
"
```

Expected:
- Title: One API Dashboard
- Variables: 7
- Rows: 7
- Total panels (excl rows): ~54
- duplicates: 0

- [ ] **Step 2: Format JSON**

```bash
python3 -c "
import json
with open('docs/grafana-dashboard.json') as f:
    d = json.load(f)
with open('docs/grafana-dashboard.json', 'w') as f:
    json.dump(d, f, indent=2, ensure_ascii=False)
"
```

- [ ] **Step 3: Final commit**

```bash
git add docs/grafana-dashboard.json
git commit -m "feat: complete One API Grafana dashboard with 7 rows and 54 panels

Includes:
- Overview with key stats and HTTP request trends
- Relay request metrics (QPS, latency, tokens, quota)
- Channel health monitoring
- User & quota tracking with currency conversion (USD/CNY)
- Billing operation metrics
- DB & Redis infrastructure panels
- Security, rate limiting & Go runtime panels

Refs: docs/superpowers/specs/2026-04-01-grafana-dashboard-design.md"
```
