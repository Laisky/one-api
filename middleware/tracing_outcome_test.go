package middleware

// Production-order outcome tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "Outcome": "add production-order tests for errors after streaming HTTP 200,
// upstream timeout, client cancellation and recovery").
//
// Each test drives the real gin middleware chain and asserts what the trace
// pipeline actually persisted, rather than calling the tracing helpers directly.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// traceOutcomeSpy tallies trace-pipeline outcomes so a test can distinguish a
// trace that was never sampled from one that was excluded or dropped.
type traceOutcomeSpy struct {
	*metrics.NoOpRecorder

	mu       sync.Mutex
	outcomes map[string]int
}

// RecordTraceRecord implements metrics.TracePipelineRecorder.
//
// Parameters:
//   - outcome: one of the metrics.TraceOutcome* constants.
//   - count: how many records the outcome applies to.
//
// Return values: none.
func (s *traceOutcomeSpy) RecordTraceRecord(outcome string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes[outcome] += count
}

// UpdateTraceQueueDepth implements metrics.TracePipelineRecorder.
//
// Parameters:
//   - depth: buffered records.
//   - capacity: queue capacity.
//
// Return values: none.
func (s *traceOutcomeSpy) UpdateTraceQueueDepth(depth, capacity float64) {}

// count reads one outcome tally.
//
// Parameters:
//   - outcome: the outcome to read.
//
// Return values:
//   - int: how many records carried it.
func (s *traceOutcomeSpy) count(outcome string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcomes[outcome]
}

// installTraceOutcomeSpy swaps in an outcome-counting metrics recorder.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *traceOutcomeSpy: the installed recorder.
func installTraceOutcomeSpy(t *testing.T) *traceOutcomeSpy {
	t.Helper()
	spy := &traceOutcomeSpy{NoOpRecorder: &metrics.NoOpRecorder{}, outcomes: map[string]int{}}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// useAggressiveSampling configures the sampler so that ONLY an always-keep rule
// can retain a trace: TRACE_SAMPLE_RATE is 0 with error retention on. Any row
// that survives therefore proves an unconditional rule fired.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values: none.
func useAggressiveSampling(t *testing.T) {
	t.Helper()
	prevRate, prevErrors, prevSlow, prevPrefixes :=
		config.TraceSampleRate, config.TraceAlwaysSampleErrors,
		config.TraceAlwaysSampleSlowMs, config.TraceExcludedPathPrefixes
	config.TraceSampleRate, config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs = 0, true, 0
	config.TraceExcludedPathPrefixes = nil
	t.Cleanup(func() {
		config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors
		config.TraceAlwaysSampleSlowMs, config.TraceExcludedPathPrefixes = prevSlow, prevPrefixes
	})
}

// newOutcomeTestEngine builds a gin engine in main.go's middleware order with a
// single traced route.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//   - path: the route path.
//   - handler: the route handler.
//
// Return values:
//   - *gin.Engine: the configured engine.
func newOutcomeTestEngine(t *testing.T, path string, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.POST(path, handler)
	return engine
}

// storedTraces flushes the sink and returns every persisted trace row.
//
// Parameters:
//   - t: the test, used to fail fast on flush or query errors.
//   - db: the isolated trace database.
//
// Return values:
//   - []model.Trace: the persisted rows.
func storedTraces(t *testing.T, db *gorm.DB) []model.Trace {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, tracing.Flush(ctx))

	var rows []model.Trace
	require.NoError(t, db.Find(&rows).Error)
	return rows
}

// streamThenFail writes a streaming chunk, flushes it, and only then tries to
// send an error status. This is the relay shape the audit flagged: the client
// already has HTTP 200 and gin will silently drop the later status.
//
// Parameters:
//   - status: the error status the handler attempts after streaming.
//
// Return values:
//   - gin.HandlerFunc: the handler.
func streamThenFail(status int) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, err := c.Writer.Write([]byte("data: {\"delta\":\"hi\"}\n\n"))
		if err == nil {
			c.Writer.Flush()
		}
		c.JSON(status, gin.H{"error": "upstream exploded"})
	}
}

// TestErrorAfterStreamedOKIsStillRetained is the audit's latent gap, closed.
//
// BEFORE: the trace pipeline read c.Writer.Status(), which is pinned at 200 once
// the stream flushed, so the always-sample-errors rule never fired and the trace
// was sampled out at any rate below 1 -- exactly the traces an operator needs.
// AFTER: TracingMiddleware intercepts the dropped WriteHeader and reports a
// semantic failure, which the sampler honours. The PERSISTED status stays 200,
// because that is genuinely what the client received; the failure is not
// invented into the status column.
func TestErrorAfterStreamedOKIsStillRetained(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installTraceOutcomeSpy(t)

	engine := newOutcomeTestEngine(t, "/v1/chat/completions",
		streamThenFail(http.StatusInternalServerError))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"gin cannot change a status the client already received")

	rows := storedTraces(t, db)
	require.Len(t, rows, 1, "a relay failure after a streamed 200 must not be sampled out")
	require.Equal(t, http.StatusOK, rows[0].Status,
		"the persisted status stays what the client received; the failure is a separate signal")
	require.Zero(t, spy.count(metrics.TraceOutcomeSampledOut))
}

// TestSuccessfulStreamIsStillSampledOut is the control for the test above: the
// same streaming shape WITHOUT a failure must still obey the sample rate, so the
// retention above is attributable to the failure and not to the streaming.
func TestSuccessfulStreamIsStillSampledOut(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installTraceOutcomeSpy(t)

	engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
		_, err := c.Writer.Write([]byte("data: {\"delta\":\"hi\"}\n\n"))
		require.NoError(t, err)
		c.Writer.Flush()
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Empty(t, storedTraces(t, db))
	require.Equal(t, 1, spy.count(metrics.TraceOutcomeSampledOut))
}

// TestUpstreamTimeoutIsRetained covers both shapes of an upstream deadline: one
// that still reaches the client as 504, and one that expires after the response
// already started streaming.
func TestUpstreamTimeoutIsRetained(t *testing.T) {
	t.Run("timeout reaches the client", func(t *testing.T) {
		db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
		useAggressiveSampling(t)

		engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Millisecond)
			defer cancel()
			<-ctx.Done()
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "upstream timeout"})
		})

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusGatewayTimeout, w.Code)

		rows := storedTraces(t, db)
		require.Len(t, rows, 1)
		require.Equal(t, http.StatusGatewayTimeout, rows[0].Status)
	})

	t.Run("timeout after the stream started", func(t *testing.T) {
		db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
		useAggressiveSampling(t)

		engine := newOutcomeTestEngine(t, "/v1/chat/completions",
			streamThenFail(http.StatusGatewayTimeout))

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		rows := storedTraces(t, db)
		require.Len(t, rows, 1, "a post-stream upstream timeout must still be retained")
		require.Equal(t, http.StatusOK, rows[0].Status)
	})

	t.Run("request deadline expires with no error status", func(t *testing.T) {
		db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
		useAggressiveSampling(t)

		engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
			c.String(http.StatusOK, "partial")
			<-c.Request.Context().Done()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody).WithContext(ctx)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		rows := storedTraces(t, db)
		require.Len(t, rows, 1,
			"an expired request deadline is a failure even though the status says 200")
	})
}

// TestClientCancellationStillPersistsTrace covers a client that goes away
// mid-request. Two things must hold: the trace is retained rather than sampled
// out, and it is actually written -- the sink work is accepted on a context
// detached from the request (context.WithoutCancel), so the cancellation must
// not cancel it.
func TestClientCancellationStillPersistsTrace(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installTraceOutcomeSpy(t)

	ctx, cancel := context.WithCancel(context.Background())
	engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
		_, err := c.Writer.Write([]byte("data: {\"delta\":\"hi\"}\n\n"))
		require.NoError(t, err)
		c.Writer.Flush()
		cancel() // the client hangs up mid-stream
		<-c.Request.Context().Done()
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody).WithContext(ctx)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	rows := storedTraces(t, db)
	require.Len(t, rows, 1, "a cancelled request must still produce its trace")
	require.Zero(t, spy.count(metrics.TraceOutcomeSampledOut))

	timestamps, err := rows[0].GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, timestamps.RequestCompleted,
		"the completion mark must survive the client's cancellation")
}
