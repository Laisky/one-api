package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// statementCounter tallies the SQL statements gorm issues, split by kind, so a
// test can assert what the request path costs the database.
type statementCounter struct {
	creates atomic.Int64
	queries atomic.Int64
	updates atomic.Int64
	deletes atomic.Int64
	raws    atomic.Int64
}

// total returns every counted statement.
//
// Parameters: none.
//
// Return values:
//   - int64: the sum across all statement kinds.
func (s *statementCounter) total() int64 {
	return s.creates.Load() + s.queries.Load() + s.updates.Load() + s.deletes.Load() + s.raws.Load()
}

// reset zeroes every counter.
//
// Parameters: none.
//
// Return values: none.
func (s *statementCounter) reset() {
	s.creates.Store(0)
	s.queries.Store(0)
	s.updates.Store(0)
	s.deletes.Store(0)
	s.raws.Store(0)
}

// attachStatementCounter registers gorm callbacks that count issued statements.
//
// Parameters:
//   - t: the test or benchmark, used to fail fast on registration errors.
//   - db: the handle to instrument.
//
// Return values:
//   - *statementCounter: the live counter.
func attachStatementCounter(t testing.TB, db *gorm.DB) *statementCounter {
	t.Helper()
	counter := &statementCounter{}

	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("test:count_create", func(*gorm.DB) { counter.creates.Add(1) }))
	require.NoError(t, db.Callback().Query().Before("gorm:query").
		Register("test:count_query", func(*gorm.DB) { counter.queries.Add(1) }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("test:count_update", func(*gorm.DB) { counter.updates.Add(1) }))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").
		Register("test:count_delete", func(*gorm.DB) { counter.deletes.Add(1) }))
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").
		Register("test:count_raw", func(*gorm.DB) { counter.raws.Add(1) }))

	return counter
}

// setupTracingTestEnv installs an isolated trace database and Phase-1 trace
// configuration for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - batchSize: how many rows the sink writes per INSERT.
//   - writerCount: how many writer goroutines drain the queue.
//   - flushInterval: maximum age of a partially filled batch.
//
// Return values:
//   - *gorm.DB: the isolated handle.
//   - *statementCounter: the live statement counter.
func setupTracingTestEnv(t *testing.T, batchSize, writerCount int, flushInterval time.Duration) (*gorm.DB, *statementCounter) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))

	prevDB := model.DB
	prevSQLite := common.UsingSQLite.Load()
	model.DB = db
	common.UsingSQLite.Store(true)

	prev := struct {
		sinks      []string
		writeMode  string
		sampleRate float64
		queue      int
		batch      int
		writers    int
		interval   int
	}{
		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate,
		config.TraceQueueSize, config.TraceBatchSize, config.TraceWriterCount, config.TraceFlushIntervalMs,
	}

	config.TraceSinks = []string{config.TraceSinkDB}
	config.TraceWriteMode = config.TraceWriteModeBatched
	config.TraceSampleRate = 1.0
	config.TraceQueueSize = 1000
	config.TraceBatchSize = batchSize
	config.TraceWriterCount = writerCount
	config.TraceFlushIntervalMs = int(flushInterval / time.Millisecond)

	require.NoError(t, tracing.InitSinks(context.Background()))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tracing.Shutdown(ctx)
		tracing.SetSinkForTest(nil)()

		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate = prev.sinks, prev.writeMode, prev.sampleRate
		config.TraceQueueSize, config.TraceBatchSize = prev.queue, prev.batch
		config.TraceWriterCount, config.TraceFlushIntervalMs = prev.writers, prev.interval

		model.DB = prevDB
		common.UsingSQLite.Store(prevSQLite)
	})

	return db, attachStatementCounter(t, db)
}

// newTracingTestEngine builds a gin engine wired the way main.go wires it, with
// a handler that emits the same lifecycle marks a relay request emits.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//
// Return values:
//   - *gin.Engine: the configured engine.
func newTracingTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		// Mirror the relay lifecycle: forwarded, first upstream byte, upstream
		// completed, then the response write that marks the first client byte.
		tracing.RecordTraceTimestamp(c, model.TimestampRequestForwarded)
		tracing.RecordTraceTimestamp(c, model.TimestampFirstUpstreamResponse)
		tracing.RecordTraceExternalCall(c, model.TraceExternalCall{Source: "mcp", Tool: "web_search"})
		tracing.RecordTraceTimestamp(c, model.TimestampUpstreamCompleted)
		c.String(http.StatusOK, "ok")
	})
	return engine
}

// TestTracingMiddlewareIssuesNoStatementsOnRequestPath is the Phase-1 acceptance
// test: the pre-proposal path issued one INSERT, five SELECT plus UPDATE pairs,
// and one status UPDATE per request. The batched path must issue none of them on
// the request goroutine, and must then persist every trace in a single INSERT.
func TestTracingMiddlewareIssuesNoStatementsOnRequestPath(t *testing.T) {
	const requests = 3

	// The batch is deliberately larger than the request count and the flush
	// ticker is an hour away, so NO write can happen while the requests run.
	// That is what makes the assertion below exact: any statement counted before
	// the explicit flush must have come from the request goroutine. Sizing the
	// batch to exactly `requests` would let the writer flush concurrently and
	// the test would then be measuring goroutine scheduling.
	db, counter := setupTracingTestEnv(t, requests*10, 1, time.Hour)
	engine := newTracingTestEngine(t)

	counter.reset()

	for range requests {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	require.Zero(t, counter.total(),
		"the request path must issue no trace statements; got creates=%d queries=%d updates=%d",
		counter.creates.Load(), counter.queries.Load(), counter.updates.Load())

	// The drain writes everything accumulated as a single multi-row INSERT.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	require.Equal(t, int64(1), counter.creates.Load(), "the whole batch must be one INSERT")
	require.Zero(t, counter.queries.Load(), "the batched path must never SELECT a trace row")
	require.Zero(t, counter.updates.Load(), "the batched path must never UPDATE a trace row")

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(requests), count)
}

// TestTracingMiddlewarePersistsCompleteTrace verifies the single write carries
// everything the six per-mutation writes used to carry.
func TestTracingMiddlewarePersistsCompleteTrace(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, time.Hour)
	engine := newTracingTestEngine(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?turnstile=secret-token", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var stored model.Trace
	require.NoError(t, db.First(&stored).Error)

	require.NotEmpty(t, stored.TraceId)
	require.NotEmpty(t, stored.UUID)
	require.Equal(t, http.MethodPost, stored.Method)
	require.Equal(t, http.StatusOK, stored.Status)
	require.NotContains(t, stored.URL, "secret-token", "sanitization must survive the batched path")
	require.Positive(t, stored.CreatedAt)

	timestamps, err := stored.GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, timestamps.RequestReceived)
	require.NotNil(t, timestamps.RequestForwarded)
	require.NotNil(t, timestamps.FirstUpstreamResponse)
	require.NotNil(t, timestamps.FirstClientResponse)
	require.NotNil(t, timestamps.UpstreamCompleted)
	require.NotNil(t, timestamps.RequestCompleted)
	require.Len(t, timestamps.ExternalCalls, 1)
	require.Equal(t, "web_search", timestamps.ExternalCalls[0].Tool)

	// The per-timestamp columns must be populated alongside the JSON document.
	require.NotNil(t, stored.TsRequestReceived)
	require.NotNil(t, stored.TsRequestCompleted)
	require.GreaterOrEqual(t, *stored.TsRequestCompleted, *stored.TsRequestReceived)
}

// TestTracingMiddlewareSamplingDropsTraces verifies a drop-everything sample
// rate keeps the request working while writing nothing.
func TestTracingMiddlewareSamplingDropsTraces(t *testing.T) {
	db, counter := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)

	prevRate, prevErrors := config.TraceSampleRate, config.TraceAlwaysSampleErrors
	config.TraceSampleRate, config.TraceAlwaysSampleErrors = 0, false
	t.Cleanup(func() { config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors })

	engine := newTracingTestEngine(t)
	counter.reset()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Zero(t, count)
	require.Zero(t, counter.creates.Load())
}

// TestTracingMiddlewareAlwaysKeepsErrors verifies the always-sample rule keeps a
// failing request even at a zero sample rate, which is what makes aggressive
// sampling safe to enable.
func TestTracingMiddlewareAlwaysKeepsErrors(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)

	prevRate, prevErrors := config.TraceSampleRate, config.TraceAlwaysSampleErrors
	config.TraceSampleRate, config.TraceAlwaysSampleErrors = 0, true
	t.Cleanup(func() { config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.GET("/boom", func(c *gin.Context) { c.String(http.StatusInternalServerError, "boom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var stored model.Trace
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, http.StatusInternalServerError, stored.Status)
}

// TestTracingMiddlewareSyncModeKeepsLegacyWrites verifies TRACE_WRITE_MODE=sync
// still produces the pre-proposal per-mutation statements, so the rollback
// escape hatch is real rather than nominal.
func TestTracingMiddlewareSyncModeKeepsLegacyWrites(t *testing.T) {
	db, counter := setupTracingTestEnv(t, 10, 1, time.Hour)

	prevMode := config.TraceWriteMode
	config.TraceWriteMode = config.TraceWriteModeSync
	t.Cleanup(func() { config.TraceWriteMode = prevMode })

	engine := newTracingTestEngine(t)
	counter.reset()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Positive(t, counter.queries.Load(), "sync mode must still read the row per mutation")
	require.Positive(t, counter.updates.Load(), "sync mode must still update the row per mutation")

	var stored model.Trace
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, http.StatusOK, stored.Status)

	timestamps, err := stored.GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, timestamps.RequestCompleted)
}

// TestTracingMiddlewareDisabledWritesNothing verifies TRACE_SINK=none removes
// the trace pipeline entirely instead of falling back to the legacy path.
func TestTracingMiddlewareDisabledWritesNothing(t *testing.T) {
	db, counter := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)

	prevSinks := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = prevSinks })

	engine := newTracingTestEngine(t)
	counter.reset()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Zero(t, counter.total())

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestTracingMiddlewareForceSampleKeepsTrace verifies tracing.ForceTraceSample
// pins a trace through a drop-everything sample rate. This is the hook a call
// site uses to keep a trace it knows is interesting; it is deliberately not
// wired to a client-supplied header, because that would let any caller defeat
// sampling and re-inflate trace volume on demand.
func TestTracingMiddlewareForceSampleKeepsTrace(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)

	prevRate, prevErrors := config.TraceSampleRate, config.TraceAlwaysSampleErrors
	config.TraceSampleRate, config.TraceAlwaysSampleErrors = 0, false
	t.Cleanup(func() { config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.GET("/interesting", func(c *gin.Context) {
		tracing.ForceTraceSample(c)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/interesting", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "a forced trace must survive a zero sample rate")
}

// TestTracingMiddlewarePanicStillRecordsTrace verifies a handler panic produces
// a trace rather than losing it: RecordTraceEnd runs from a defer, and
// gin.Recovery still receives the panic.
func TestTracingMiddlewarePanicStillRecordsTrace(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	prevRate, prevErrors := config.TraceSampleRate, config.TraceAlwaysSampleErrors
	config.TraceSampleRate, config.TraceAlwaysSampleErrors = 0, true
	t.Cleanup(func() {
		config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/panic", http.NoBody)
	w := httptest.NewRecorder()
	require.NotPanics(t, func() { engine.ServeHTTP(w, req) })
	require.Equal(t, http.StatusInternalServerError, w.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var stored model.Trace
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, http.StatusInternalServerError, stored.Status,
		"panic traces must carry the status Gin's recovery will return")
	timestamps, err := stored.GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, timestamps.RequestCompleted,
		"a panicking request must still record its completion mark")
}

// TestTracingMiddlewareSkipsExcludedPaths verifies the never-trace list keeps
// static assets, health probes, and metrics scrapes from creating trace rows.
// TracingMiddleware is registered globally, before any route grouping, so
// without the list every SPA asset request produced one.
func TestTracingMiddlewareSkipsExcludedPaths(t *testing.T) {
	db, counter := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)

	prevPrefixes := config.TraceExcludedPathPrefixes
	config.TraceExcludedPathPrefixes = []string{"/api/status", "/metrics", "/assets"}
	t.Cleanup(func() { config.TraceExcludedPathPrefixes = prevPrefixes })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.GET("/api/status", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/assets/index.js", func(c *gin.Context) { c.String(http.StatusOK, "js") })
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		tracing.RecordTraceTimestamp(c, model.TimestampRequestForwarded)
		c.String(http.StatusOK, "ok")
	})

	counter.reset()

	for _, target := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/status"},
		{http.MethodGet, "/assets/index.js"},
	} {
		req := httptest.NewRequest(target.method, target.path, http.NoBody)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	// Read the statement tally before querying the table, so the verification
	// query itself is not counted.
	require.Zero(t, counter.total(), "an excluded path must issue no trace statements")

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Zero(t, count, "excluded paths must not create trace rows")

	// A relay path on the same engine is still traced.
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFlush()
	require.NoError(t, tracing.Flush(flushCtx))

	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
