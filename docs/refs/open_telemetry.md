# OpenTelemetry Observability Stack for Gin and GORM on a Self-Hosted VPS

**End-to-End Observability for Go Gin + GORM Applications with OpenTelemetry and the VictoriaMetrics Stack**

## Menu

- [OpenTelemetry Observability Stack for Gin and GORM on a Self-Hosted VPS](#opentelemetry-observability-stack-for-gin-and-gorm-on-a-self-hosted-vps)
  - [Menu](#menu)
  - [Introduction](#introduction)
  - [1. Instrumenting a Gin + GORM Go Application with OpenTelemetry](#1-instrumenting-a-gin--gorm-go-application-with-opentelemetry)
    - [1.1. Overview of OpenTelemetry in Go](#11-overview-of-opentelemetry-in-go)
    - [1.2. Instrumenting Gin for Tracing and Metrics](#12-instrumenting-gin-for-tracing-and-metrics)
      - [1.2.1. Required Dependencies](#121-required-dependencies)
      - [1.2.2. Initializing OpenTelemetry](#122-initializing-opentelemetry)
      - [1.2.3. Adding OpenTelemetry Middleware to Gin](#123-adding-opentelemetry-middleware-to-gin)
      - [1.2.4. Customizing Spans and Metrics](#124-customizing-spans-and-metrics)
      - [1.2.5. Emitting Custom Metrics](#125-emitting-custom-metrics)
      - [1.2.6. Automatic HTTP Metrics](#126-automatic-http-metrics)
    - [1.3. Instrumenting GORM for Database Tracing](#13-instrumenting-gorm-for-database-tracing)
      - [1.3.1. Required Dependencies](#131-required-dependencies)
      - [1.3.2. Integrating otelgorm](#132-integrating-otelgorm)
      - [1.3.3. Metrics from GORM](#133-metrics-from-gorm)
    - [1.4. Environment Variables and Configuration](#14-environment-variables-and-configuration)
  - [2. Configuring the OpenTelemetry Collector](#2-configuring-the-opentelemetry-collector)
    - [2.1. Collector Overview](#21-collector-overview)
    - [2.2. Example Collector Configuration](#22-example-collector-configuration)
    - [2.3. Collector Deployment Patterns](#23-collector-deployment-patterns)
    - [2.4. Security and Best Practices](#24-security-and-best-practices)
  - [3. Deploying and Configuring VictoriaMetrics, VictoriaTraces, and VictoriaLogs](#3-deploying-and-configuring-victoriametrics-victoriatraces-and-victorialogs)
    - [3.1. VictoriaMetrics (Metrics Storage)](#31-victoriametrics-metrics-storage)
    - [3.2. VictoriaLogs (Log Storage)](#32-victorialogs-log-storage)
    - [3.3. VictoriaTraces (Trace Storage)](#33-victoriatraces-trace-storage)
    - [3.4. Data Flow Diagram](#34-data-flow-diagram)
  - [4. Connecting the OpenTelemetry Collector to the Victoria\* Stack](#4-connecting-the-opentelemetry-collector-to-the-victoria-stack)
    - [4.1. Exporter Configuration](#41-exporter-configuration)
    - [4.2. Direct Application Export (Optional)](#42-direct-application-export-optional)
  - [5. Setting Up the Observability UI](#5-setting-up-the-observability-ui)
    - [5.1. Grafana with VictoriaMetrics, VictoriaLogs, and VictoriaTraces](#51-grafana-with-victoriametrics-victorialogs-and-victoriatraces)
      - [5.1.1. Installing Grafana](#511-installing-grafana)
      - [5.1.2. Adding Data Sources](#512-adding-data-sources)
      - [5.1.3. Features](#513-features)
    - [5.2. Native Victoria\* UIs](#52-native-victoria-uis)
  - [6. Production Deployment Best Practices](#6-production-deployment-best-practices)
    - [6.1. Security](#61-security)
    - [6.2. Resource Usage and Sizing](#62-resource-usage-and-sizing)
    - [6.3. Data Retention](#63-data-retention)
    - [6.4. Backup and Restore](#64-backup-and-restore)
    - [6.5. PostgreSQL Monitoring](#65-postgresql-monitoring)
    - [6.9. Production Readiness Checklist](#69-production-readiness-checklist)
  - [7. Example: End-to-End Docker Compose Stack](#7-example-end-to-end-docker-compose-stack)
  - [8. Conclusion](#8-conclusion)

## Introduction

Modern web applications demand robust observability to ensure reliability, performance, and rapid troubleshooting. For Go developers building with the **Gin** web framework and **GORM** ORM, integrating full tracing, metrics, and logging is essential for understanding application behavior and database interactions. **OpenTelemetry** has emerged as the industry standard for vendor-neutral instrumentation, enabling unified collection of traces, metrics, and logs.

When paired with a self-hosted observability stack—such as **VictoriaMetrics** (metrics), **VictoriaTraces** (traces), and **VictoriaLogs** (logs)—developers gain a powerful, cost-effective, and scalable solution for end-to-end visibility.

This comprehensive technical guide details how to:

1. Instrument a Gin + GORM Go application with OpenTelemetry.
2. Configure the OpenTelemetry Collector.
3. Deploy and connect the VictoriaMetrics stack.
4. Set up a complete observability UI using Grafana and native Victoria interfaces.
5. Implement production best practices for deployment, security, resource management, and data retention on a self-hosted VPS with PostgreSQL.

## 1. Instrumenting a Gin + GORM Go Application with OpenTelemetry

### 1.1. Overview of OpenTelemetry in Go

OpenTelemetry provides a unified API and SDK for instrumenting Go applications. For Gin and GORM, dedicated instrumentation libraries exist, enabling automatic and manual span creation, context propagation, and metrics emission.

**Key Concepts:**

- **Traces:** Represent end-to-end request flows, including HTTP handlers and database queries.
- **Metrics:** Quantitative measurements (e.g., request counts, latencies, DB query durations).
- **Logs:** Structured or unstructured event records, often correlated with traces.

### 1.2. Instrumenting Gin for Tracing and Metrics

#### 1.2.1. Required Dependencies

Add the following to your `go.mod`:

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
```

#### 1.2.2. Initializing OpenTelemetry

Create an initialization function for OpenTelemetry:

```go
package main

import (
    "context"
    "os"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitOpenTelemetry(ctx context.Context) (func(context.Context) error, func(context.Context) error, error) {
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String("go-gin-app"),
            semconv.ServiceVersionKey.String("1.0.0"),
        ),
    )
    if err != nil {
        return nil, nil, err
    }

    // Trace Exporter
    traceExp, err := otlptracehttp.New(ctx,
        otlptracehttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
        otlptracehttp.WithInsecure(),
    )
    if err != nil {
        return nil, nil, err
    }

    tracerProvider := trace.NewTracerProvider(
        trace.WithBatcher(traceExp),
        trace.WithResource(res),
    )
    otel.SetTracerProvider(tracerProvider)

    // Metric Exporter
    metricExp, err := otlpmetrichttp.New(ctx,
        otlpmetrichttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
        otlpmetrichttp.WithInsecure(),
    )
    if err != nil {
        return nil, nil, err
    }

    meterProvider := metric.NewMeterProvider(
        metric.WithReader(metric.NewPeriodicReader(metricExp)),
        metric.WithResource(res),
    )
    otel.SetMeterProvider(meterProvider)

    return tracerProvider.Shutdown, meterProvider.Shutdown, nil
}
```

> **Explanation:** This function initializes both tracing and metrics providers, exporting data via OTLP HTTP to the endpoint specified in the environment variable `OTEL_EXPORTER_OTLP_ENDPOINT`. In production, use TLS and authentication as appropriate.

#### 1.2.3. Adding OpenTelemetry Middleware to Gin

Instrument Gin with the OpenTelemetry middleware:

```go
import (
    "github.com/gin-gonic/gin"
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
    r := gin.Default()
    r.Use(otelgin.Middleware("go-gin-app"))
    // ... define routes
    r.Run(":8080")
}
```

> **Best Practice:** Set `gin.ContextWithFallback = true` to ensure proper context propagation.

#### 1.2.4. Customizing Spans and Metrics

You can add custom attributes to spans using the OpenTelemetry API:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

func HelloHandler(c *gin.Context) {
    tracer := otel.Tracer("go-gin-app")
    ctx, span := tracer.Start(c.Request.Context(), "hello_handler")
    defer span.End()

    span.SetAttributes(attribute.String("user.name", c.Query("name")))
    c.JSON(200, gin.H{"message": "Hello, " + c.Query("name")})
}
```

#### 1.2.5. Emitting Custom Metrics

Define and record custom metrics:

```go
import (
    "go.opentelemetry.io/otel/metric"
)

var (
    meter = otel.Meter("go-gin-app")
    requestCounter metric.Int64Counter
)

func init() {
    requestCounter, _ = meter.Int64Counter(
        "http.server.requests",
        metric.WithDescription("Total number of HTTP requests received."),
    )
}

func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestCounter.Add(c.Request.Context(), 1)
        c.Next()
    }
}
```

#### 1.2.6. Automatic HTTP Metrics

The `otelgin` middleware emits standard HTTP metrics such as `http.server.duration`, `http.server.request.size`, and `http.server.response.size`.

### 1.3. Instrumenting GORM for Database Tracing

#### 1.3.1. Required Dependencies

Add the GORM OpenTelemetry plugin:

```bash
go get github.com/uptrace/opentelemetry-go-extra/otelgorm
```

#### 1.3.2. Integrating otelgorm

Instrument GORM with the `otelgorm` plugin:

```go
import (
    "gorm.io/gorm"
    "gorm.io/driver/postgres"
    "github.com/uptrace/opentelemetry-go-extra/otelgorm"
)

func ConnectDatabase() (*gorm.DB, error) {
    db, err := gorm.Open(postgres.Open("dsn"), &gorm.Config{})
    if err != nil {
        return nil, err
    }
    if err := db.Use(otelgorm.NewPlugin()); err != nil {
        return nil, err
    }
    return db, nil
}
```

> **Context Propagation:** Always use `db.WithContext(ctx)` to ensure DB spans are linked to the parent trace:
>
> ```go
> db.WithContext(ctx).Find(&books)
> ```

#### 1.3.3. Metrics from GORM

The `otelgorm` plugin emits spans for each query. For granular metrics, consider custom instrumentation or use the OpenTelemetry Collector's PostgreSQL receiver for server-side metrics.

### 1.4. Environment Variables and Configuration

Set the following environment variables for your application:

```bash
export SERVICE_NAME=go-gin-app
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
export INSECURE_MODE=true
```

> **Note:** Use `localhost:4318` for OTLP HTTP; use `localhost:4317` for OTLP gRPC if preferred.

## 2. Configuring the OpenTelemetry Collector

### 2.1. Collector Overview

The OpenTelemetry Collector is a standalone service that receives, processes, and exports telemetry data. It decouples application instrumentation from backend storage.

**Collector Pipeline Components:**

- **Receivers:** Ingest data (e.g., OTLP, Prometheus, filelog, postgresql).
- **Processors:** Transform, batch, sample, or filter data.
- **Exporters:** Send data to backends (VictoriaMetrics, VictoriaLogs, VictoriaTraces).
- **Extensions:** Add auxiliary features (health checks, authentication).

### 2.2. Example Collector Configuration

Below is a sample `otel-collector-config.yaml` for a single-node deployment:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  postgresql:
    endpoint: "localhost:5432"
    transport: tcp
    username: "otel_monitor"
    password: "your_password"
    databases: ["your_db"]
    collection_interval: 10s
    tls:
      insecure: true

processors:
  batch:
    timeout: 10s
  memory_limiter:
    check_interval: 1s
    limit_mib: 400
    spike_limit_mib: 100

exporters:
  otlphttp/victoriametrics:
    metrics_endpoint: "http://victoriametrics:8428/opentelemetry/v1/metrics"
    compression: gzip
    encoding: proto
    tls:
      insecure: true

  otlphttp/victorialogs:
    logs_endpoint: "http://victorialogs:9428/insert/opentelemetry/v1/logs"
    compression: gzip
    encoding: proto
    tls:
      insecure: true

  otlphttp/victoriatraces:
    traces_endpoint: "http://victoriatraces:10428/insert/opentelemetry/v1/traces"
    compression: gzip
    encoding: proto
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlphttp/victoriatraces]
    metrics:
      receivers: [otlp, postgresql]
      processors: [memory_limiter, batch]
      exporters: [otlphttp/victoriametrics]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlphttp/victorialogs]
```

### 2.3. Collector Deployment Patterns

- **Standalone Service:** Run as a systemd service or Docker container on your VPS.
- **Agent Model:** Deploy alongside each application instance.
- **Gateway Model:** Centralized collector.

For a single VPS, use systemd for reliability:

```bash
sudo systemctl enable otelcol
sudo systemctl start otelcol
```

### 2.4. Security and Best Practices

- Bind endpoints to `localhost` or trusted interfaces.
- Use TLS and authentication for external connections.
- Store sensitive configuration (passwords) in environment variables.
- Enable health checks and monitoring for the collector itself.

## 3. Deploying and Configuring VictoriaMetrics, VictoriaTraces, and VictoriaLogs

### 3.1. VictoriaMetrics (Metrics Storage)

**Installation (Docker):**

```bash
docker run -it --rm \
  -v $(pwd)/victoria-metrics-data:/victoria-metrics-data \
  -p 8428:8428 \
  victoriametrics/victoria-metrics:v1.132.0 \
  --storageDataPath=/victoria-metrics-data \
  --opentelemetry.usePrometheusNaming=true
```

**Configuration:**

- **Data Path:** `-storageDataPath`
- **Retention:** `-retentionPeriod=90d` (default 1 year)
- **Prometheus Naming:** `--opentelemetry.usePrometheusNaming=true`
- **Ingestion:** OTLP HTTP via `/opentelemetry/v1/metrics`

### 3.2. VictoriaLogs (Log Storage)

**Installation (Docker):**

```bash
docker run --rm -it -p 9428:9428 \
  -v ./victoria-logs-data:/victoria-logs-data \
  victoriametrics/victoria-logs:v1.41.0 \
  -storageDataPath=victoria-logs-data
```

**Configuration:**

- **Data Path:** `-storageDataPath`
- **Retention:** `-retentionPeriod=7d`
- **Ingestion:** OTLP HTTP via `/insert/opentelemetry/v1/logs`

### 3.3. VictoriaTraces (Trace Storage)

**Installation (Docker):**

```bash
docker run --rm -it -p 10428:10428 \
  -v ./victoria-traces-data:/victoria-traces-data \
  victoriametrics/victoria-traces:latest
```

**Configuration:**

- **Data Path:** `-storageDataPath`
- **Ingestion:** OTLP HTTP via `/insert/opentelemetry/v1/traces`

### 3.4. Data Flow Diagram

```text
+-------------------+         +---------------------+         +---------------------+
| Gin + GORM App    |  --->   | OpenTelemetry       |  --->   | Victoria* Stack     |
| (OTLP HTTP/gRPC)  |         | Collector           |         | (Metrics, Logs,     |
|                   |         | (4317/4318)         |         |  Traces)            |
+-------------------+         +---------------------+         +---------------------+
```

## 4. Connecting the OpenTelemetry Collector to the Victoria\* Stack

### 4.1. Exporter Configuration

The OpenTelemetry Collector uses the `otlphttp` exporter. Ensure your `otel-collector-config.yaml` points to the correct endpoints:

```yaml
exporters:
  otlphttp/victoriametrics:
    metrics_endpoint: "http://victoria-metrics:8428/opentelemetry/v1/metrics"
    # ... options
  otlphttp/victorialogs:
    logs_endpoint: "http://victoria-logs:9428/insert/opentelemetry/v1/logs"
    # ... options
  otlphttp/victoriatraces:
    traces_endpoint: "http://victoria-traces:10428/insert/opentelemetry/v1/traces"
    # ... options
```

### 4.2. Direct Application Export (Optional)

You may export directly from Go to Victoria\* endpoints using the OTLP HTTP exporters, but using the collector is recommended for flexibility and batching.

## 5. Setting Up the Observability UI

### 5.1. Grafana with VictoriaMetrics, VictoriaLogs, and VictoriaTraces

Grafana is the standard UI. The VictoriaMetrics stack provides plugins for metrics and logs, and supports Jaeger-compatible trace queries.

#### 5.1.1. Installing Grafana

```bash
docker run -d -p 3000:3000 grafana/grafana:latest
```

#### 5.1.2. Adding Data Sources

Example `provisioning/datasources/victoriametrics.yml`:

```yaml
apiVersion: 1
datasources:
  - name: VictoriaMetrics
    type: victoriametrics-metrics-datasource
    access: proxy
    url: http://victoria-metrics:8428
    isDefault: true
  - name: VictoriaLogs
    type: victoriametrics-logs-datasource
    access: proxy
    url: http://victoria-logs:9428
  - name: VictoriaTraces
    type: jaeger
    access: proxy
    url: http://victoria-traces:10428/select/jaeger
```

#### 5.1.3. Features

- **MetricsQL:** Superset of PromQL for metrics.
- **LogsQL:** Query language for logs.
- **Trace Correlation:** Jump from logs to traces using trace IDs in Grafana.

### 5.2. Native Victoria\* UIs

- **VictoriaMetrics UI:** `http://<host>:8428/vmui`
- **VictoriaLogs UI:** `http://<host>:9428/select/vmui`
- **VictoriaTraces UI:** `http://<host>:10428/vmui`

## 6. Production Deployment Best Practices

### 6.1. Security

- **Network Exposure:** Bind services to private interfaces/localhost.
- **TLS/Auth:** Enable TLS for production; use mTLS for sensitive telemetry.
- **Least Privilege:** Run services as non-root users.

### 6.2. Resource Usage and Sizing

- **VictoriaMetrics:** Ensure 50% free RAM and 20% free disk space. Disable swap.
- **Collector:** Use `batch` and `memory_limiter` processors.

### 6.3. Data Retention

- **Metrics:** Set `-retentionPeriod`.
- **Logs/Traces:** Adjust retention based on disk capacity (e.g., `-retention.maxDiskUsagePercent`).

### 6.4. Backup and Restore

- **VictoriaMetrics:** Use `vmbackup` and `vmrestore`.
- **VictoriaLogs:** Use partition snapshots.

### 6.5. PostgreSQL Monitoring

- **Client-Side:** Use `otelgorm`.
- **Server-Side:** Use the Collector's PostgreSQL receiver for engine metrics (cache hit ratio, lag).

### 6.9. Production Readiness Checklist

- [ ] All services run as non-root users.
- [ ] All endpoints are bound to trusted interfaces.
- [ ] TLS and authentication are enabled for external connections.
- [ ] Resource usage is monitored and alerting is configured.
- [ ] Data retention is set according to capacity.
- [ ] Regular backups are scheduled.
- [ ] Collector pipelines use batch and memory limiters.
- [ ] Application and DB are fully instrumented.

## 7. Example: End-to-End Docker Compose Stack

Below is a simplified `docker-compose.yml`:

```yaml
version: "3.7"
services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    volumes:
      - ./otel-collector-config.yaml:/etc/otel-collector-config.yaml
    ports:
      - "4317:4317"
      - "4318:4318"
    command: ["--config=/etc/otel-collector-config.yaml"]

  victoria-metrics:
    image: victoriametrics/victoria-metrics:v1.132.0
    volumes:
      - ./victoria-metrics-data:/victoria-metrics-data
    ports:
      - "8428:8428"
    command:
      [
        "--storageDataPath=/victoria-metrics-data",
        "--opentelemetry.usePrometheusNaming=true",
      ]

  victoria-logs:
    image: victoriametrics/victoria-logs:v1.41.0
    volumes:
      - ./victoria-logs-data:/victoria-logs-data
    ports:
      - "9428:9428"
    command: ["-storageDataPath=victoria-logs-data"]

  victoria-traces:
    image: victoriametrics/victoria-traces:latest
    volumes:
      - ./victoria-traces-data:/victoria-traces-data
    ports:
      - "10428:10428"
    command: ["-storageDataPath=victoria-traces-data"]

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - ./grafana/provisioning:/etc/grafana/provisioning
```

## 8. Conclusion

By instrumenting your Go Gin + GORM application with OpenTelemetry and deploying the VictoriaMetrics stack, you achieve comprehensive, vendor-neutral observability. The OpenTelemetry Collector acts as a flexible pipeline, while Grafana and native UIs provide powerful visualization. Adhering to production best practices ensures your stack is secure, efficient, and maintainable.
