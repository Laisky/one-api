# Logging & Alert Integration Guide (Universal)

This document describes a production-grade, centralized logging methodology that can be adopted by other Go services (Gin-based, but adaptable). It is based on the proven patterns in this repository:

- A **single structured logger** shared across the service.
- **Request-scoped loggers** retrievable via `gmw.GetLogger(...)`.
- **Consistent correlation identifiers** (request ID, trace ID) across logs, traces, and DB audit records.
- Optional **log rotation**, **log retention cleanup**, and **alert push** (webhook) integration.
- A separate **DB-backed audit/usage log** stream (business logs) for administrative and billing/usage events.

The goal is to let multiple teams and projects integrate logging in a consistent way so that centralized management (search, alerting, dashboards, retention) becomes straightforward.

## Menu

- [Logging \& Alert Integration Guide (Universal)](#logging--alert-integration-guide-universal)
  - [Menu](#menu)
  - [1. Terminology: Two Kinds of “Logs”](#1-terminology-two-kinds-of-logs)
    - [1.1 Operational logs (runtime/application logs)](#11-operational-logs-runtimeapplication-logs)
    - [1.2 Business/audit logs (DB persisted)](#12-businessaudit-logs-db-persisted)
  - [2. Design Principles (Standards)](#2-design-principles-standards)
    - [2.1 Structured logs only](#21-structured-logs-only)
    - [2.2 Context-aware logging in request handlers](#22-context-aware-logging-in-request-handlers)
    - [2.3 Error handling contract: handle an error once](#23-error-handling-contract-handle-an-error-once)
    - [2.4 No sensitive data in logs](#24-no-sensitive-data-in-logs)
    - [2.5 Correlation fields are mandatory](#25-correlation-fields-are-mandatory)
  - [3. Installation \& Setup](#3-installation--setup)
    - [3.1 External Dependencies](#31-external-dependencies)
    - [3.2 Required Imports](#32-required-imports)
    - [3.3 Internal Logger Package](#33-internal-logger-package)
  - [4. Components Overview (What You Need)](#4-components-overview-what-you-need)
    - [4.1 Shared logger package](#41-shared-logger-package)
    - [4.2 Gin integration](#42-gin-integration)
    - [4.3 DB audit/usage log stream](#43-db-auditusage-log-stream)
  - [5. Integration Steps (Copy/Paste Checklist)](#5-integration-steps-copypaste-checklist)
    - [5.1 Initialize logging early (before serving requests)](#51-initialize-logging-early-before-serving-requests)
    - [5.2 Configure log directory](#52-configure-log-directory)
    - [5.3 Attach Gin middleware logging](#53-attach-gin-middleware-logging)
    - [5.4 Ensure request IDs exist](#54-ensure-request-ids-exist)
    - [5.5 Use `gmw.GetLogger` inside request handlers](#55-use-gmwgetlogger-inside-request-handlers)
    - [5.6 Log request bodies only when safe](#56-log-request-bodies-only-when-safe)
  - [6. Output Sinks: Stdout, Files, Rotation, Retention](#6-output-sinks-stdout-files-rotation-retention)
    - [6.1 File sink behavior](#61-file-sink-behavior)
    - [6.2 Rotation](#62-rotation)
    - [6.3 Retention](#63-retention)
  - [7. Alert Push Integration (Escalation)](#7-alert-push-integration-escalation)
    - [7.1 Enabling alert push](#71-enabling-alert-push)
    - [7.2 Rate limiting](#72-rate-limiting)
  - [8. DB Audit/Usage Logs (Central Reporting)](#8-db-auditusage-logs-central-reporting)
    - [8.1 Initialize log database (optional separate DB)](#81-initialize-log-database-optional-separate-db)
    - [8.2 Persisting logs](#82-persisting-logs)
    - [8.3 Redaction for management logs](#83-redaction-for-management-logs)
  - [9. Recommended Log Schema (Field Standards)](#9-recommended-log-schema-field-standards)
    - [9.1 Required fields (request path)](#91-required-fields-request-path)
    - [9.2 Strongly recommended fields](#92-strongly-recommended-fields)
    - [9.3 Error fields](#93-error-fields)
  - [10. Testing \& Validation Checklist](#10-testing--validation-checklist)
  - [11. Common Pitfalls (What To Avoid)](#11-common-pitfalls-what-to-avoid)
  - [12. Adoption Template (Minimal Workable Integration)](#12-adoption-template-minimal-workable-integration)

## 1. Terminology: Two Kinds of “Logs”

Most systems need both of the following, and they should remain distinct:

### 1.1 Operational logs (runtime/application logs)

- Purpose: debugging and operating the service (startup/shutdown, errors, upstream calls, performance warnings, middleware logs).
- Output: stdout/stderr, files (with optional rotation/retention), and optional alert push.
- Implementation: the global structured logger in `common/logger`.

### 1.2 Business/audit logs (DB persisted)

- Purpose: record user-visible, queryable events (consumption, topups, admin actions, system events, tests).
- Output: database table(s) (supports dashboards and UI pages).
- Implementation: `model.Log` persisted through `LOG_DB` (initialized by `model.InitLogDB()`), plus record helpers like `model.RecordConsumeLog(...)`, `model.RecordManageLog(...)`, etc.

Operational logs are for operators; business logs are for product/reporting. Correlate them via request ID / trace ID.

## 2. Design Principles (Standards)

These rules are the “contract” teams should follow.

### 2.1 Structured logs only

- Use structured fields (Zap fields), not concatenated strings.
- Every log line should be machine-searchable.

Examples:

```go
lg.Info("upstream request completed",
	zap.String("provider", provider),
	zap.Int("status", resp.StatusCode),
	zap.Duration("latency", latency),
)
```

### 2.2 Context-aware logging in request handlers

- Inside request handlers/middlewares: **always** obtain a request-scoped logger using `gmw.GetLogger(c)` (or `gmw.GetLogger(ctx)` for context-based flows).
- Do not use a global logger directly from request code.

### 2.3 Error handling contract: handle an error once

- Wrap errors with stack/context (`github.com/Laisky/errors/v2`).
- An error must be either:
  - returned (preferred), OR
  - logged and converted into a response,
    but never both.

### 2.4 No sensitive data in logs

- Never log API keys, tokens, passwords, or raw credentials.
- Be careful with request/response bodies: they often contain user content or secrets.
- If you must log payloads for debugging, do it only under debug mode and with truncation/redaction.

### 2.5 Correlation fields are mandatory

At minimum, request-scoped logs should carry:

- `request_id`
- `trace_id` (if tracing is enabled)

Optionally include:

- `user_id`, `username`, `token_id`/`token_name`
- `model`, `channel_id`, `provider`
- `upstream_request_id` (if providers return one)

Correlation is what makes centralized logging “work”.

## 3. Installation & Setup

To integrate this logging stack into a new Go project, you need the following dependencies and internal packages.

### 3.1 External Dependencies

Install the core logging and middleware libraries:

```bash
# Core logging utilities and structured logger (Zap-based)
go get github.com/Laisky/go-utils/v6
go get github.com/Laisky/zap

# Gin middleware for structured logging and request context
go get github.com/Laisky/gin-middlewares/v7

# Error wrapping with context
go get github.com/Laisky/errors/v2
```

### 3.2 Required Imports

Typically, you will need these imports in your server initialization and request handlers:

```go
import (
	"github.com/gin-gonic/gin"
	"github.com/Laisky/zap"
	gmw "github.com/Laisky/gin-middlewares/v7"
	errors "github.com/Laisky/errors/v2"

	// Internal project packages
	"your-project/common/logger"
	"your-project/common/config"
)
```

### 3.3 Internal Logger Package

If you are following the One-API architecture, copy the `common/logger` directory to your project. This package provides the unified `logger.Logger` instance and setup functions (`SetupLogger`, `SetupEnhancedLogger`, etc.).

## 4. Components Overview (What You Need)

### 4.1 Shared logger package

The reference implementation lives in `common/logger`:

- `logger.Logger`: the shared structured logger instance.
- `logger.SetupLogger()`: configures output sinks (stdout + file) and optional rotation.
- `logger.SetupEnhancedLogger(ctx)`: adds alert push hook (if configured) and stable context fields (e.g., host).
- `logger.StartLogRetentionCleaner(ctx, days, logDir)`: deletes old log files (time-based retention).

### 4.2 Gin integration

Gin request logging is handled by `gmw.NewLoggerMiddleware(...)` (from `github.com/Laisky/gin-middlewares/v7`).

Your service should:

- Install that middleware early in the chain.
- Ensure request ID middleware runs as well.

### 4.3 DB audit/usage log stream

The DB audit log stream is backed by `model.Log` and stored via `LOG_DB`.

- Initialize with `model.InitLogDB()`.
- Record with helpers like `RecordConsumeLog`, `RecordManageLog`, etc.
- Include `request_id`/`trace_id` where possible (`RecordLogWithIDs`, `RecordTopupLogWithIDs`, etc).

## 5. Integration Steps (Copy/Paste Checklist)

This section is intended to be a universal “do exactly this” guide.

### 5.1 Initialize logging early (before serving requests)

In your `main()` (or service bootstrap) do the following, in order:

1. Parse configuration + determine log directory
2. Configure logger sinks (stdout + file)
3. Start retention workers (optional)
4. Configure alert hooks + stable fields (optional)

Reference sequence used by this repository:

```go
ctx := context.Background()

common.Init() // parses flags, sets logger.LogDir, etc.
logger.SetupLogger()
logger.StartLogRetentionCleaner(ctx, config.LogRetentionDays, logger.LogDir)
logger.SetupEnhancedLogger(ctx)
```

Key behavior:

- If `logger.LogDir` is empty/blank, `SetupLogger()` keeps stdout/stderr only.
- File logging uses `oneapi.log` by default in the configured directory.

### 5.2 Configure log directory

This repository sets the log directory via a CLI flag and normalizes it to an absolute path:

- CLI flag: `--log-dir` (default: `./logs`)
- Expansion: environment placeholder expansion and path normalization

If you adopt the same approach:

- Set `logger.LogDir` to an absolute, writable directory.
- Create the directory during startup to fail fast on permission issues.

### 5.3 Attach Gin middleware logging

Add the request logger middleware to Gin. The reference wiring is:

```go
logLevel := glog.LevelInfo
if config.DebugEnabled {
	logLevel = glog.LevelDebug
}

server := gin.New()

server.Use(
	gin.Recovery(),
	gmw.NewLoggerMiddleware(
		gmw.WithLoggerMwColored(),
		gmw.WithLevel(logLevel.String()),
		gmw.WithLogger(logger.Logger.Named("gin")),
	),
)
```

Notes:

- Use a named logger for Gin (e.g., `gin`) to separate concerns in centralized searches.
- Avoid introducing a second, unrelated logging framework.

### 5.4 Ensure request IDs exist

Install a request ID middleware so:

- every request has a unique ID
- the ID is returned to clients (often via header)
- the ID is attached to request logs

This repository uses `middleware.RequestId()`.

You should also ensure request IDs are included in error messages returned to clients where appropriate (to reduce support/debug time).

### 5.5 Use `gmw.GetLogger` inside request handlers

Inside handlers, always do:

```go
func SomeHandler(c *gin.Context) {
	lg := gmw.GetLogger(c)
	lg.Info("handler started")
	// ...
}
```

Standards:

- Call `gmw.GetLogger(...)` **once per function** and store it locally.
- Add fields via `With(...)` when you need stable context for a scope:

```go
lg := gmw.GetLogger(c).With(
	zap.String("component", "relay"),
	zap.String("provider", provider),
)
```

### 5.6 Log request bodies only when safe

This repository includes a helper that logs request bytes at debug level:

- `common.UnmarshalBodyReusable(c, v)`

It logs the raw request when the request is not yet tagged with a model key and debug logging is enabled.

Guidance for other teams:

- Default: do **not** log request bodies.
- If debugging requires it:
  - limit to debug mode
  - truncate large payloads
  - redact sensitive fields
  - do not log binary content

## 6. Output Sinks: Stdout, Files, Rotation, Retention

Centralized logging usually starts with stdout, but file sinks are still common in VMs and some container deployments.

### 6.1 File sink behavior

`logger.SetupLogger()` writes to:

- stdout (always)
- one file sink under `logger.LogDir` (when non-empty)

Default filename:

- `oneapi.log`

### 6.2 Rotation

Rotation is enabled by default unless explicitly disabled:

- When `ONLY_ONE_LOG_FILE=true`, rotation is disabled and all logs go to the single file.
- Otherwise, rotation is enabled and driven by `LOG_ROTATION_INTERVAL`.

Rotation intervals supported:

- `hourly`
- `daily` (default)
- `weekly`

Implementation detail (for teams extending this code): rotation is implemented via a Zap custom sink registered under the scheme `oneapi-rotate`.

### 6.3 Retention

Retention is implemented in two layers:

1. **Writer retention** (when rotation is enabled) can use `retention_days` to prune.
2. A **retention cleaner** deletes `.log` files older than a cutoff once every 24h:

```go
logger.StartLogRetentionCleaner(ctx, config.LogRetentionDays, logger.LogDir)
```

Rules:

- `LOG_RETENTION_DAYS <= 0` disables the retention worker.
- Retention uses file modification time (UTC) to decide expiration.

## 7. Alert Push Integration (Escalation)

Operational logs are great for search, but high-severity events should be escalated to humans.

### 7.1 Enabling alert push

Alert push is configured via:

- `LOG_PUSH_API` (webhook URL; empty disables)
- `LOG_PUSH_TYPE` (label for routing on the receiver side)
- `LOG_PUSH_TOKEN` (optional authentication)

When configured, `logger.SetupEnhancedLogger(ctx)` installs a Zap hook that pushes events at `error` level or higher.

### 7.2 Rate limiting

To avoid alert storms, alert push is rate-limited (current reference: ~1 event/second).

Guidance for other projects:

- Start conservative.
- Ensure alert push failures do not crash your service.
- Ensure alert payloads never contain secrets.

## 8. DB Audit/Usage Logs (Central Reporting)

This section describes how to implement queryable “business logs” stored in a database.

### 8.1 Initialize log database (optional separate DB)

This repository supports a separate database for logs:

- If `LogSQLDSN` is empty, logs use the primary DB.
- If `LogSQLDSN` is configured, `model.InitLogDB()` uses it to populate `model.LOG_DB`.

When adopting this pattern, ensure:

- the log DB schema migration is executed on master nodes
- DB connection pool settings are configured similarly to the primary DB

### 8.2 Persisting logs

Use typed, structured “log records” for business events:

- consumption logs
- topup logs
- manage/admin change logs
- system/test logs

Guidelines:

- Include `request_id` and `trace_id` whenever a record relates to a request.
- Sanitize/redact sensitive fields for management logs.
- Keep log content stable so dashboards don’t break.

### 8.3 Redaction for management logs

If you record administrative changes, implement redaction rules.

This repository uses a placeholder for sensitive fields in manage logs (e.g., `"[REDACTED]"`) and keyword-based redaction.

Guidance:

- redact by field name, not only by value
- maintain a short, explicit denylist
- never store secrets in DB logs

## 9. Recommended Log Schema (Field Standards)

To keep logs consistent across services, standardize on these keys.

### 9.1 Required fields (request path)

- `request_id`: unique ID for the request
- `method`: HTTP method
- `path`: HTTP path
- `status`: response status code
- `latency`: request latency

### 9.2 Strongly recommended fields

- `trace_id`: trace identifier (if tracing enabled)
- `client_ip`: caller IP (if safe / allowed)
- `user_id`, `username`: authenticated identity
- `token_id`/`token_name`: auth token
- `model`, `channel_id`, `provider`: AI routing context

### 9.3 Error fields

- `error`: serialized error
- `error_type`: stable classification string (optional)
- `upstream_status`: status code from provider (optional)

## 10. Testing & Validation Checklist

Teams adopting this logging stack should verify:

1. Startup logs appear on stdout.
2. File logs are created when `--log-dir` is set.
3. Rotation works for `hourly/daily/weekly`.
4. Retention removes old files when enabled.
5. Gin request logs appear and include request IDs.
6. `gmw.GetLogger` logs contain request correlation context.
7. Alert push triggers on `error` logs (in a staging environment).
8. DB audit logs are written and queryable by time range.

Operational note: when validating, ensure the system does not leak secrets in logs.

## 11. Common Pitfalls (What To Avoid)

- Mixing logging frameworks (creates fragmented output and inconsistent fields).
- Logging full request/response bodies in production.
- Logging secrets (tokens, passwords, API keys).
- Logging and returning the same error (double handling).
- Missing correlation IDs (hard to debug incidents).

## 12. Adoption Template (Minimal Workable Integration)

If you want the fastest “minimum viable” adoption in another service:

1. Copy (or vendor) a `common/logger` equivalent.
2. Wire `SetupLogger()` and `SetupEnhancedLogger(ctx)` in `main()`.
3. Install Gin logger middleware and request ID middleware.
4. Standardize on `gmw.GetLogger(...)` in all request code.
5. Add DB audit logs only if your product needs user-visible reporting.
