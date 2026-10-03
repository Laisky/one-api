package middleware

// Production-order operational metric tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3).
//
// The mapping from a request's fate onto the operational outcome label is unit
// tested in common/tracing; what can only be checked here is that the real
// middleware chain actually produces the fate the mapping expects -- a panic
// reaching gin.Recovery(), a client hanging up mid-stream, an upstream error
// arriving after a streamed 200.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// requestOutcomeSpy captures the operational per-request metrics alongside the
// trace-pipeline outcomes.
//
// The mutex is not decorative: the batched sink's writer goroutines record trace
// outcomes while the request goroutine records the request outcome.
type requestOutcomeSpy struct {
	*traceOutcomeSpy

	mu       sync.Mutex
	requests map[string]int
	ttft     map[string]int
}

// RecordRequestOutcome implements metrics.RequestOutcomeRecorder.
//
// Parameters:
//   - outcome: one of the metrics.RequestOutcome* constants.
//   - durationMs: the request's total lifetime in milliseconds.
//
// Return values: none.
func (s *requestOutcomeSpy) RecordRequestOutcome(outcome string, durationMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[outcome]++
}

// RecordTimeToFirstToken implements metrics.RequestOutcomeRecorder.
//
// Parameters:
//   - outcome: one of the metrics.RequestOutcome* constants.
//   - ttftMs: milliseconds to the first client byte.
//
// Return values: none.
func (s *requestOutcomeSpy) RecordTimeToFirstToken(outcome string, ttftMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttft[outcome]++
}

// requestCount reads one outcome tally.
//
// Parameters:
//   - outcome: the outcome to read.
//
// Return values:
//   - int: how many requests carried it.
func (s *requestOutcomeSpy) requestCount(outcome string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[outcome]
}

// requestTotal reads how many request outcomes were recorded in all.
//
// Parameters: none.
//
// Return values:
//   - int: the sum across every outcome label.
func (s *requestOutcomeSpy) requestTotal() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum := 0
	for _, n := range s.requests {
		sum += n
	}
	return sum
}

// installRequestOutcomeSpy swaps in a recorder capturing operational outcomes.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *requestOutcomeSpy: the installed recorder.
func installRequestOutcomeSpy(t *testing.T) *requestOutcomeSpy {
	t.Helper()
	spy := &requestOutcomeSpy{
		traceOutcomeSpy: &traceOutcomeSpy{
			NoOpRecorder: &metrics.NoOpRecorder{},
			outcomes:     map[string]int{},
		},
		requests: map[string]int{},
		ttft:     map[string]int{},
	}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// TestOperationalOutcomeSurvivesSampling proves the operational series is not a
// sampled view of traffic: with TRACE_SAMPLE_RATE at 0 and no always-keep rule
// the trace is discarded, and the request is still counted exactly once.
func TestOperationalOutcomeSurvivesSampling(t *testing.T) {
	db, _ := setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	prevErrors := config.TraceAlwaysSampleErrors
	config.TraceAlwaysSampleErrors = false
	t.Cleanup(func() { config.TraceAlwaysSampleErrors = prevErrors })
	spy := installRequestOutcomeSpy(t)

	engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	require.Empty(t, storedTraces(t, db), "the trace itself must have been discarded")
	require.Equal(t, 1, spy.count(metrics.TraceOutcomeSampledOut))
	require.Equal(t, 1, spy.requestCount(metrics.RequestOutcomeSuccess),
		"a sampled-out request is still a request that happened")
	require.Equal(t, 1, spy.requestTotal())
}

// TestOperationalOutcomeForPostStreamedFailure covers the case an HTTP status
// histogram cannot see: the client got 200 because the stream had flushed, and
// the relay failed afterwards.
func TestOperationalOutcomeForPostStreamedFailure(t *testing.T) {
	setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installRequestOutcomeSpy(t)

	engine := newOutcomeTestEngine(t, "/v1/chat/completions",
		streamThenFail(http.StatusInternalServerError))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, 1, spy.requestCount(metrics.RequestOutcomeUpstream))
	require.Zero(t, spy.requestCount(metrics.RequestOutcomeSuccess))
	require.Equal(t, 1, spy.ttft[metrics.RequestOutcomeUpstream],
		"the stream produced a first byte, so its TTFT is known")
}

// TestOperationalOutcomeForClientCancellation verifies a caller hanging up is
// counted as canceled rather than inflating the gateway's own error rate.
func TestOperationalOutcomeForClientCancellation(t *testing.T) {
	setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installRequestOutcomeSpy(t)

	ctx, cancel := context.WithCancel(context.Background())
	engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
		_, err := c.Writer.Write([]byte("data: {\"delta\":\"hi\"}\n\n"))
		require.NoError(t, err)
		c.Writer.Flush()
		cancel()
		<-c.Request.Context().Done()
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody).WithContext(ctx)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	require.Equal(t, 1, spy.requestCount(metrics.RequestOutcomeCanceled))
	require.Zero(t, spy.requestCount(metrics.RequestOutcomeServerError))
}

// TestOperationalOutcomeForPanicIsCountedOnce verifies a panicking handler is
// counted as a panic, and only once: the tracing middleware ends the trace
// before re-panicking, and gin.Recovery() then unwinds through the same defer.
func TestOperationalOutcomeForPanicIsCountedOnce(t *testing.T) {
	setupTracingTestEnv(t, 1, 1, 20*time.Millisecond)
	useAggressiveSampling(t)
	spy := installRequestOutcomeSpy(t)

	engine := newOutcomeTestEngine(t, "/v1/chat/completions", func(c *gin.Context) {
		panic("handler exploded")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	require.Equal(t, 1, spy.requestCount(metrics.RequestOutcomePanic))
	require.Equal(t, 1, spy.requestTotal(), "one request, one sample")
	require.Empty(t, spy.ttft, "a panic before any write has no time-to-first-token")
}
