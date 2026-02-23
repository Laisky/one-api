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
    trace_id VARCHAR(64) UNIQUE NOT NULL,
    url VARCHAR(512) NOT NULL,
    method VARCHAR(16) NOT NULL,
    body_size BIGINT DEFAULT 0,
    status INTEGER DEFAULT 0,
    timestamps TEXT,  -- JSON object with key timestamps
    created_at BIGINT,
    updated_at BIGINT
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

- Automatic trace creation with initial timestamp
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

#### Default Template (`web/default/src/components/`)

- `TracingModal.js` - Modern React modal with Semantic UI
- `LogsTable.js` - Clickable rows with tracing integration
- Timeline visualization with duration calculations

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
    "id": 1,
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

Retrieve tracing information for a specific log entry.

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
      "id": 123,
      "username": "user123",
      "content": "Request processed successfully"
    }
  }
}
```

## Performance Considerations

### Database Optimization

- Indexed `trace_id` columns for fast lookups
- JSON timestamps for flexible schema evolution
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
