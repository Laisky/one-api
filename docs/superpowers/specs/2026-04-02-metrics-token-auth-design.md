# Metrics Endpoint Token Authentication Design

## Overview

Protect the `/metrics` Prometheus endpoint with a dedicated Bearer token to prevent unauthorized access to sensitive system telemetry data (usernames, model names, channel IDs, quota data, system internals).

## Previous State

- `/metrics` is registered without any auth middleware (`main.go:217`)
- Enabled by default via `ENABLE_PROMETHEUS_METRICS=true`
- Exposes all `one_api_*` metrics plus Go runtime metrics to anyone who can reach the endpoint

## Design

### Configuration

| Variable | Location | Type | Default | Description |
|----------|----------|------|---------|-------------|
| `METRICS_TOKEN` | Environment variable | `string` | `""` (empty) | Bearer token for `/metrics` authentication |

- Read at startup in `common/config/config.go` via `env.String()`
- Not stored in DB — consistent with other security credentials like `SESSION_SECRET`
- Not hot-reloadable — requires restart to change (matches Prometheus scrape config lifecycle)

### Middleware: `MetricsAuth()`

New function in `middleware/prometheus.go`:

```go
MetricsAuth() gin.HandlerFunc
```

Logic:

1. If `config.MetricsToken` is empty (not configured):
   - Return `403` with `{"error": "metrics endpoint requires METRICS_TOKEN configuration"}`
   - This is the default — metrics are blocked until explicitly configured
2. Extract token from `Authorization: Bearer <token>` header:
   - If header is missing or malformed → `401` with `{"error": "invalid metrics token, please check your METRICS_TOKEN configuration"}`
   - If token does not match `config.MetricsToken` → `401` with same message
   - Use constant-time comparison (`subtle.ConstantTimeCompare`) to prevent timing attacks
3. Token matches → `c.Next()`

### Route Registration

Change in `main.go`:

```go
// Before
server.GET("/metrics", gin.WrapH(promhttp.Handler()))

// After
server.GET("/metrics", middleware.MetricsAuth(), gin.WrapH(promhttp.Handler()))
```

### Prometheus Scrape Configuration

Example for `prometheus.yml`:

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

### Docker Compose Example

```yaml
services:
  oneapi:
    environment:
      - METRICS_TOKEN=your-secure-token-here
```

## Out of Scope

- IP whitelist — can be layered on later if needed
- Rate limiting on `/metrics` — Prometheus scrape interval is fixed
- Modifications to existing `TokenAuth` / `AdminAuth` middleware
- Token rotation without restart — keep it simple

## Files to Modify

| File | Change |
|------|--------|
| `common/config/config.go` | Add `MetricsToken` variable, read from `METRICS_TOKEN` env |
| `middleware/prometheus.go` | Add `MetricsAuth()` function |
| `main.go` | Add `middleware.MetricsAuth()` to `/metrics` route |
| `docs/manuals/PROMETHEUS.md` | Add Bearer token auth configuration examples |
