# Request Tracing System Architecture

## Overview

The request tracing system provides comprehensive tracking of API requests throughout their lifecycle, from initial receipt to completion. It standardizes request identification using TraceID from gin-middlewares and captures key timestamps for performance analysis and debugging.

## Architecture Components

### 1. Core Components

#### TraceID Standardization

- **Source**: gin-middlewares `TraceID(ctx *gin.Context)` function
- **Format**: JaegerTracingID string representation
- **Usage**: Unified across all logging and tracing operations

#### Database Schema

**Traces Table**:

```sql
CREATE TABLE traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid CHAR(36),
    trace_id VARCHAR(64) UNIQUE NOT NULL,
    url VARCHAR(512) NOT NULL,
    method VARCHAR(16) NOT NULL,
    body_size BIGINT DEFAULT 0,
    status INTEGER DEFAULT 0,
    timestamps TEXT,  -- JSON object with key timestamps
    created_at BIGINT,
    updated_at BIGINT,

    -- Per-timestamp projection columns. Written together with `timestamps`,
    -- never instead of it, so a pre-migration binary still reads a complete
    -- document. They exist so latency questions are answerable in SQL.
    ts_request_received BIGINT NULL,
    ts_request_forwarded BIGINT NULL,
    ts_first_upstream_response BIGINT NULL,
    ts_first_client_response BIGINT NULL,
    ts_upstream_completed BIGINT NULL,
    ts_request_completed BIGINT NULL
);
```

**Logs Table Enhancement**:

```sql
ALTER TABLE logs ADD COLUMN trace_id VARCHAR(64);
CREATE INDEX idx_logs_trace_id ON logs(trace_id);
```

#### Timestamp Structure

```json
{
  "request_received": 1640995200000,
  "request_forwarded": 1640995200100,
  "first_upstream_response": 1640995200500,
  "first_client_response": 1640995200520,
  "upstream_completed": 1640995201000,
  "request_completed": 1640995201020
}
```

### 2. Implementation Layers

#### Model Layer (`model/trace.go`)

- `Trace` struct with GORM annotations
- `TraceTimestamps` struct for JSON parsing
- CRUD operations: `CreateTrace`, `UpdateTraceTimestamp`, `UpdateTraceStatus`
- Helper functions: `GetTraceByTraceId`, `GetTraceTimestamps`

#### Helper Layer (`common/helper/helper.go`)

- `GetTraceIDFromContext(ctx context.Context)` - Extract TraceID from standard context
- Integration with existing `GetRequestID` functionality

#### Tracing Utilities (`common/tracing/tracing.go`)

- `GetTraceID(c *gin.Context)` - Extract TraceID from gin context
- `RecordTraceStart`, `RecordTraceTimestamp`, `RecordTraceEnd` - Lifecycle tracking
- `WithTraceID` - Add TraceID to structured logging

#### Middleware Layer (`middleware/tracing.go`)

- `TracingMiddleware()` - Gin middleware for automatic tracing
- Custom response writer to capture first response timing
- Automatic trace lifecycle management

#### Controller Layer (`controller/tracing.go`)

- `GetTraceByTraceId` - API endpoint for trace retrieval
- `GetTraceByLogId` - API endpoint linking logs to traces
- Duration calculations for performance metrics

### 3. Integration Points

#### Request Lifecycle Instrumentation

**Request Start** (`middleware/tracing.go`):

- Allocates the in-memory `Recorder` (no database statement)
- URL, method, and body size capture

**Upstream Forwarding** (`relay/adaptor/common.go`):

- `DoRequestHelper`: Record forwarding timestamp
- `DoRequest`: Record first upstream response timestamp

**Streaming Completion** (`relay/adaptor/openai/main.go`):

- Multiple streaming handlers instrumented
- Upstream completion timestamp recording

**Response Handling** (`middleware/tracing.go`):

- Custom response writer captures first client response
- Final completion and status recording

#### Logging Integration (`model/log.go`)

- All log entries automatically include `trace_id`
- Backward compatibility with existing `request_id`
- Enhanced structured logging with trace context

### 4. Frontend Components

#### Berry Template (`web/berry/src/views/Log/`)

- `TracingModal.js` - Material-UI based modal
- `TableRow.js` - Clickable rows with hover effects
- Chinese localization and modern design

#### Air Template (`web/air/src/components/`)

- `TracingModal.js` - Semi-UI based modal
- `LogsTable.js` - Semi Design table integration
- Consistent API integration across templates

## API Endpoints

### GET /api/trace/:trace_id

Retrieve tracing information by trace ID.

**Response**:

```json
{
  "success": true,
  "data": {
    "uuid": "018f0000-0000-7000-8000-000000000001",
    "trace_id": "01234567-89ab-cdef-0123-456789abcdef",
    "url": "/v1/chat/completions",
    "method": "POST",
    "body_size": 1024,
    "status": 200,
    "timestamps": { ... },
    "created_at": 1640995200000,
    "updated_at": 1640995201000
  }
}
```

### GET /api/trace/log/:log_id

Retrieve tracing information for a specific log entry by log UUID.

**Response**:

```json
{
  "success": true,
  "data": {
    "trace_id": "01234567-89ab-cdef-0123-456789abcdef",
    "timestamps": { ... },
    "durations": {
      "processing_time": 100,
      "upstream_response_time": 400,
      "response_processing_time": 20,
      "streaming_time": 480,
      "total_time": 1020
    },
    "log": {
      "uuid": "018f0000-0000-7000-8000-000000000123",
      "username": "user123",
      "content": "Request processed successfully"
    }
  }
}
```

When the authenticated caller owns the log but its trace was sampled out or
sent only to an external sink, the log-based endpoint returns a successful
availability response instead of treating expected local absence as a server
failure:

```json
{
  "success": true,
  "data": {
    "availability": "not_retained_locally",
    "trace_id": "01234567-89ab-cdef-0123-456789abcdef"
  }
}
```

Ordinary users may read only traces correlated with their own log rows;
administrators and root users may inspect all traces.

## Write Pipeline

Since the Phase-1 work of
[Observability Data Tiering](../proposals/20260905_observability-data-tiering.md),
a trace is accumulated in memory and written **once**, at request end.

### Before

Every lifecycle mark issued its own statements on the request goroutine:
one `INSERT`, then a `SELECT` + whole-document-rewriting `UPDATE` per timestamp,
then a status `UPDATE` — about **12 statements per request**.

### Now

```
request start ──▶ tracing.RecordTraceStart
                    └─ allocates a *tracing.Recorder in the gin context
                       (ctxkey.TraceRecorder); issues NO SQL

lifecycle marks ──▶ tracing.RecordTraceTimestamp / RecordTraceExternalCall
                    └─ mutate the in-memory document, add an OTel span event

request end ────▶ tracing.RecordTraceEnd
                    ├─ Recorder.Finish() — single-shot, returns a snapshot
                    ├─ tracing.SampleDecision(status, durationMs, forced)
                    └─ TraceSink.Submit(row)
                                │
              ┌─────────────────┼──────────────────┐
              ▼                 ▼                  ▼
        sqlSink (batched)   otlpSink (span)   nullSink (drop)
              │
     bounded queue ──▶ N writers ──▶ model.InsertTraces (multi-row INSERT)
```

### Components

| Component | File | Responsibility |
| --- | --- | --- |
| `Recorder` | `common/tracing/recorder.go` | in-memory, mutex-guarded accumulation; single-shot `Finish` |
| `SampleDecision` | `common/tracing/sampling.go` | head-plus-tail sampling evaluated at request end |
| `TraceSink` | `common/tracing/sink.go` | sink interface, selection, fan-out, process-wide handle |
| `sqlSink` | `common/tracing/sink_sql.go` | bounded queue, batched multi-row `INSERT`, drain on shutdown |
| `otlpSink` | `common/tracing/sink_otlp.go` | one span per request; zero SQL writes |
| `NewTraceRow` / `InsertTraces` | `model/trace_batch.go` | row building (sanitization, bounds) and batched persistence |

### Sampling

The decision is made when the request ends, so status and duration are already
known. Traces are always kept when the status is >= 400
(`TRACE_ALWAYS_SAMPLE_ERRORS`), when the duration reaches
`TRACE_ALWAYS_SAMPLE_SLOW_MS`, or when a call site invoked
`tracing.ForceTraceSample(c)`. Everything else is subject to
`TRACE_SAMPLE_RATE`.

### Capacity policy

The writer queue is bounded (`TRACE_QUEUE_SIZE`). A saturated queue drops the
newest record and increments
`oneapi_trace_records_total{outcome="dropped_queue_full"}`; it never blocks the
relay. Watch that counter together with `oneapi_trace_queue_depth`: a depth that
tracks `oneapi_trace_queue_capacity` means the database cannot keep pace with
trace ingestion.

### Shutdown

`main` calls `tracing.Shutdown(shutdownCtx)` after the HTTP server stops
accepting requests and before the database handle closes, so accepted traces are
not lost on a graceful restart.

### Behavior notes

- **Read-after-write, and in-flight traces.** Under `TRACE_WRITE_MODE=batched`
  a trace exists only after its batch is written. `GET /api/trace/:id` and
  `GET /api/trace/log/:log_id` flush the writer once on a miss before giving up,
  so a completed request is always findable. A request that is still RUNNING has
  no row at all, however, so its partial timeline is not queryable the way it was
  under `sync`. That is why `sync` remains the standalone default: batching is
  opt-in through `OBSERVABILITY_PROFILE=scaled` or `TRACE_WRITE_MODE=batched`.
- **Panics are traced.** `RecordTraceEnd` runs from a `defer`, so a handler that
  panics still produces a complete trace; `gin.Recovery()` is registered earlier
  in the chain and still receives the panic.
- **Sampling is not client-controllable.** `tracing.ForceTraceSample(c)` is
  available to server-side call sites that know a request is interesting. It is
  deliberately not wired to a request header: a client-settable flag would let
  any caller defeat sampling and re-inflate trace volume on demand.
- **Untraced contexts are free.** Synthetic `gin.Context` values (channel
  testing, for example) carry no recorder. Under `batched` mode the lifecycle
  helpers become no-ops for them instead of issuing a `SELECT` that could only
  miss.
- **Excluded paths cost nothing.** `TracingMiddleware` is registered globally,
  before any route grouping, so without a skip list every static SPA asset,
  health probe, and metrics scrape produced a trace row. Every lifecycle helper
  honors the list, not just `RecordTraceStart`, so an excluded path issues no
  statement in `sync` mode either.

### Retention

The `traces` sweeper runs every `RETENTION_SWEEP_INTERVAL_MINUTES` and deletes
in bounded chunks (`model.ChunkedDelete`), pausing between them. An unbounded
`DELETE ... WHERE created_at < ?` over a table with hundreds of millions of
expired rows opens one enormous transaction: a multi-terabyte WAL burst plus
table bloat on PostgreSQL, a gap-locking stall on MySQL, and a minutes-long
write lock on SQLite.

The bounded statement differs per engine, because the three disagree about how
to limit a `DELETE`:

| Engine | Form |
| --- | --- |
| MySQL | `DELETE ... WHERE <pred> LIMIT n` (native; a subquery naming the target table is forbidden) |
| PostgreSQL | `DELETE ... WHERE ctid IN (SELECT ctid ... LIMIT n)` (no native `DELETE ... LIMIT`) |
| SQLite | `DELETE ... WHERE rowid IN (SELECT rowid ... LIMIT n)` (the embedded driver is not built with `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`) |

The same helper backs the `logs` purge (`model.DeleteOldLog`) and the async-task
binding sweeper. A cancelled sweep is not an error: the predicate is time-based,
so the next tick resumes where the last one stopped.

### Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `OBSERVABILITY_PROFILE` | `standalone` | preset for every knob below (`standalone`, `scaled`, `external`) |
| `TRACE_SINK` | `db` (`otlp` under `external`) | comma-separated: `db`, `otlp`, `none` |
| `TRACE_WRITE_MODE` | `sync` (standalone), `batched` (scaled/external) | batched accumulates in memory and writes once; sync is the pre-Phase-1 per-mutation path |
| `TRACE_SAMPLE_RATE` | `1.0` (`0.05` under `scaled`) | probability an ordinary trace is kept |
| `TRACE_ALWAYS_SAMPLE_ERRORS` | `true` | always keep status >= 400 |
| `TRACE_ALWAYS_SAMPLE_SLOW_MS` | `0` (`5000` under `scaled`) | always keep slow requests; 0 disables |
| `TRACE_BATCH_SIZE` | `500` | rows per `INSERT` |
| `TRACE_FLUSH_INTERVAL_MS` | `1000` | maximum age of a partial batch |
| `TRACE_QUEUE_SIZE` | `20000` (`50000` under `scaled`) | bounded backlog |
| `TRACE_WRITER_COUNT` | `2` (`4` under `scaled`) | writer goroutines |
| `TRACE_EXCLUDED_PATH_PREFIXES` | empty (standalone); `/api/status,/metrics,/health,/static,/assets,/favicon` (scaled/external) | never-traced path prefixes; `-` disables the list |
| `TRACE_RETENTION_DAYS` | `30`; `0` disables | age cutoff for the `traces` sweeper |
| `RETENTION_DELETE_BATCH_SIZE` | `5000` | rows removed per retention `DELETE` |
| `RETENTION_DELETE_PAUSE_MS` | `10` | pause between retention chunks (measured optimum) |
| `RETENTION_SWEEP_INTERVAL_MINUTES` | `60` | how often retention workers run |

`TRACE_WRITE_MODE=sync` preserves the legacy behavior in which an in-flight
request is visible through SQL and every lifecycle mark is persisted
synchronously. Because completion-time sampling and non-SQL fan-out cannot be
implemented without changing that contract, `sync` is valid only with exactly
`TRACE_SINK=db` (or `none`) and `TRACE_SAMPLE_RATE=1`. Use `batched` for
sampling, OTLP, or multiple sinks. Startup rejects incompatible combinations.

## Performance Considerations

### Database Optimization

- Indexed `trace_id` columns for fast lookups
- One batched `INSERT` per `TRACE_BATCH_SIZE` traces instead of ~12 statements
  per request
- JSON timestamps for flexible schema evolution, projected onto columns for
  SQL-queryable latency analysis
- Automatic cleanup policies for old trace data

### Memory Usage

- Minimal memory footprint with structured timestamps
- Efficient JSON marshaling/unmarshaling
- Context-aware logging to prevent memory leaks

### Network Overhead

- Lazy loading of trace data in frontend
- Compressed JSON responses
- Efficient API design with minimal round trips

## Security and Privacy

### Access Control

- User authentication required for trace access
- Users can only access traces for their own requests
- Admin users have full trace visibility

### Data Retention

- Configurable trace data retention policies (`TRACE_RETENTION_DAYS`, default 30; set to 0 to disable cleanup)
- Automatic cleanup of old trace records via the daily trace retention worker
- Privacy-compliant data handling

## Monitoring and Observability

### Metrics Collection

- Trace creation success/failure rates
- API endpoint performance metrics
- Frontend modal usage analytics

### Error Handling

- Graceful degradation when tracing fails
- Comprehensive error logging
- User-friendly error messages in UI

### Debugging Support

- Detailed trace information for troubleshooting
- Request correlation across system components
- Performance bottleneck identification

## Future Enhancements

### Distributed Tracing

- Integration with OpenTelemetry
- Cross-service trace correlation
- Jaeger/Zipkin compatibility

### Advanced Analytics

- Performance trend analysis
- Anomaly detection
- Automated alerting

### Enhanced UI Features

- Real-time trace updates
- Advanced filtering and search
- Export capabilities for trace data
