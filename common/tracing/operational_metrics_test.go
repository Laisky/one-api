package tracing

// Operational request-outcome tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3).
//
// The properties pinned here are the ones that make the operational view
// trustworthy rather than merely present: the outcome vocabulary is the metrics
// package's and not the sampler's, a request counts exactly once, and a request
// whose trace is thrown away by the sampler is still counted.

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// requestOutcomeSpy captures the operational per-request metrics.
//
// It embeds traceOutcomeSpy so one installed recorder answers both questions a
// sampling test needs: what the trace pipeline decided, and what the operational
// series recorded. Like that spy it is not safe for concurrent use; every test
// here drives the pipeline from the test goroutine.
type requestOutcomeSpy struct {
	*traceOutcomeSpy

	requests  map[string]int
	durations map[string][]float64
	ttft      map[string][]float64
}

// RecordRequestOutcome implements metrics.RequestOutcomeRecorder.
//
// Parameters:
//   - outcome: one of the metrics.RequestOutcome* constants.
//   - durationMs: the request's total lifetime in milliseconds.
//
// Return values: none.
func (s *requestOutcomeSpy) RecordRequestOutcome(outcome string, durationMs float64) {
	s.requests[outcome]++
	s.durations[outcome] = append(s.durations[outcome], durationMs)
}

// RecordTimeToFirstToken implements metrics.RequestOutcomeRecorder.
//
// Parameters:
//   - outcome: one of the metrics.RequestOutcome* constants.
//   - ttftMs: milliseconds to the first client byte.
//
// Return values: none.
func (s *requestOutcomeSpy) RecordTimeToFirstToken(outcome string, ttftMs float64) {
	s.ttft[outcome] = append(s.ttft[outcome], ttftMs)
}

// total returns how many request outcomes were recorded in all.
//
// Parameters: none.
//
// Return values:
//   - int: the sum across every outcome label.
func (s *requestOutcomeSpy) total() int {
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
		requests:  map[string]int{},
		durations: map[string][]float64{},
		ttft:      map[string][]float64{},
	}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// useNeverSample configures the sampler so no trace is ever persisted: rate 0,
// no error rule, no slow rule. Any operational sample a test still observes
// therefore proves the metric is independent of TRACE_SAMPLE_RATE.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values: none.
func useNeverSample(t *testing.T) {
	t.Helper()
	prevRate, prevErrors, prevSlow := config.TraceSampleRate,
		config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs
	config.TraceSampleRate, config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs = 0, false, 0
	t.Cleanup(func() {
		config.TraceSampleRate, config.TraceAlwaysSampleErrors = prevRate, prevErrors
		config.TraceAlwaysSampleSlowMs = prevSlow
	})
}

// useAlwaysSample configures the sampler to keep every trace, so a test can
// prove the operational sample is not duplicated on the retained path either.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values: none.
func useAlwaysSample(t *testing.T) {
	t.Helper()
	prev := config.TraceSampleRate
	config.TraceSampleRate = 1
	t.Cleanup(func() { config.TraceSampleRate = prev })
	t.Cleanup(SetSinkForTest(nullSink{}))
}

// newTracedTestContext builds a gin context with a live recorder bound to it,
// as TracingMiddleware would.
//
// Parameters:
//   - t: the test, used to register cleanup and fail fast.
//   - path: the request path; it must not be on the never-trace list.
//
// Return values:
//   - *gin.Context: the traced context.
func newTracedTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	withExcludedPrefixes(t)
	prevSinks, prevMode := config.TraceSinks, config.TraceWriteMode
	config.TraceSinks, config.TraceWriteMode = []string{config.TraceSinkDB}, config.TraceWriteModeBatched
	t.Cleanup(func() { config.TraceSinks, config.TraceWriteMode = prevSinks, prevMode })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)

	RecordTraceStart(c)
	require.NotNil(t, recorderFromGin(c), "the test request must be traced")
	return c
}

// TestRequestOutcomePrecedence pins the mapping from (status, FailureKind) onto
// the metrics vocabulary, including the precedence order documented on
// requestOutcome.
//
// The table is the contract: FailureKind values must never reach a metric label,
// so every one of them has an explicit destination here, and the status-only
// rows prove the fallback is not accidentally shadowed by a failure case.
func TestRequestOutcomePrecedence(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		status  int
		failure FailureKind
		want    string
	}{
		{"panic outranks everything", 500, FailurePanic, metrics.RequestOutcomePanic},
		{"panic on a streamed 200", 200, FailurePanic, metrics.RequestOutcomePanic},
		{"timeout outranks a client cancellation", 504, FailureTimeout, metrics.RequestOutcomeTimeout},
		{"timeout after a streamed 200", 200, FailureTimeout, metrics.RequestOutcomeTimeout},
		{"client cancellation outranks upstream", 200, FailureClientCanceled, metrics.RequestOutcomeCanceled},
		{"client cancellation with a 499", 499, FailureClientCanceled, metrics.RequestOutcomeCanceled},
		{"upstream failure after a streamed 200", 200, FailureUpstream, metrics.RequestOutcomeUpstream},
		{"upstream failure outranks a 5xx", 502, FailureUpstream, metrics.RequestOutcomeUpstream},
		{"upstream failure outranks a 4xx", 400, FailureUpstream, metrics.RequestOutcomeUpstream},
		{"unknown failure kind is an upstream failure", 200, FailureKind("something_new"), metrics.RequestOutcomeUpstream},
		{"server error", 500, FailureNone, metrics.RequestOutcomeServerError},
		{"server error at the top of the range", 599, FailureNone, metrics.RequestOutcomeServerError},
		{"client error", 400, FailureNone, metrics.RequestOutcomeClientError},
		{"rate limited client", 429, FailureNone, metrics.RequestOutcomeClientError},
		{"success", 200, FailureNone, metrics.RequestOutcomeSuccess},
		{"redirect is not an error", 302, FailureNone, metrics.RequestOutcomeSuccess},
		{"no recorded status", 0, FailureNone, metrics.RequestOutcomeSuccess},
	} {
		require.Equal(t, tc.want, requestOutcome(tc.status, tc.failure), tc.name)
	}
}

// TestRequestOutcomeIsRecordedWhenTheTraceIsSampledOut is the W3.3 requirement
// that operational metrics are not reduced by TRACE_SAMPLE_RATE.
//
// With the sampler configured to keep nothing, the trace is discarded and the
// pipeline counts a sampled_out record -- and the request is still counted
// exactly once in the operational series. Were the metric recorded after the
// sampling decision, the default 5% rate would silently turn every request-rate
// and error-rate panel into a 5% view of the gateway.
func TestRequestOutcomeIsRecordedWhenTheTraceIsSampledOut(t *testing.T) {
	useNeverSample(t)
	spy := installRequestOutcomeSpy(t)

	c := newTracedTestContext(t, "/v1/chat/completions")
	RecordTraceEnd(c)

	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeSampledOut],
		"the trace itself must have been discarded, or this proves nothing")
	require.Equal(t, 1, spy.requests[metrics.RequestOutcomeSuccess],
		"a sampled-out request is still a request that happened")
	require.Equal(t, 1, spy.total())
}

// TestRequestOutcomeIsRecordedOncePerRequest verifies the single-sample contract
// on the retained path and under a repeated completion hook.
//
// A panicking handler reaches RecordTraceEndWithStatus and then Gin's own
// recovery reaches RecordTraceEnd; Recorder.Finish's ok flag is what keeps that
// from double-counting the request.
func TestRequestOutcomeIsRecordedOncePerRequest(t *testing.T) {
	useAlwaysSample(t)
	spy := installRequestOutcomeSpy(t)

	c := newTracedTestContext(t, "/v1/chat/completions")
	RecordTraceEndWithStatus(c, 500)
	RecordTraceEnd(c)

	require.Equal(t, 1, spy.total(), "one request must produce exactly one sample")
	require.Equal(t, 1, spy.requests[metrics.RequestOutcomeServerError])
}

// TestPostStreamUpstreamFailureIsCountedAsUpstreamError covers the case an HTTP
// status histogram alone cannot see: the client received 200 because the stream
// had already flushed, and the relay failed afterwards.
func TestPostStreamUpstreamFailureIsCountedAsUpstreamError(t *testing.T) {
	useNeverSample(t)
	spy := installRequestOutcomeSpy(t)

	c := newTracedTestContext(t, "/v1/chat/completions")
	RecordTraceTimestamp(c, model.TimestampFirstClientResponse)
	RecordTraceFailure(c, FailureUpstream)
	RecordTraceEnd(c)

	require.Equal(t, 1, spy.requests[metrics.RequestOutcomeUpstream])
	require.Zero(t, spy.requests[metrics.RequestOutcomeSuccess],
		"a failed stream that returned 200 must not be counted as a success")
	require.Len(t, spy.ttft[metrics.RequestOutcomeUpstream], 1,
		"the stream produced a first byte, so its TTFT is known")
}

// TestTimeToFirstTokenIsRecordedOnlyWhenAFirstByteExists verifies the TTFT
// series is not polluted with zeros from requests that never answered.
func TestTimeToFirstTokenIsRecordedOnlyWhenAFirstByteExists(t *testing.T) {
	t.Run("no first client response", func(t *testing.T) {
		useNeverSample(t)
		spy := installRequestOutcomeSpy(t)

		c := newTracedTestContext(t, "/v1/chat/completions")
		RecordTraceEndWithStatus(c, 504)

		require.Equal(t, 1, spy.requests[metrics.RequestOutcomeServerError],
			"the request is still counted; only its TTFT is unknown")
		require.Empty(t, spy.ttft,
			"a request that never answered has no time-to-first-token")
	})

	t.Run("first client response marked", func(t *testing.T) {
		useNeverSample(t)
		spy := installRequestOutcomeSpy(t)

		c := newTracedTestContext(t, "/v1/chat/completions")
		RecordTraceTimestamp(c, model.TimestampFirstClientResponse)
		RecordTraceEnd(c)

		require.Len(t, spy.ttft[metrics.RequestOutcomeSuccess], 1)
		require.GreaterOrEqual(t, spy.ttft[metrics.RequestOutcomeSuccess][0], float64(0))
	})
}

// TestExcludedRequestHasNoMeasuredOutcome documents the deliberate decision
// recorded on recordRequestOutcome: a request tracing skipped has no measured
// lifetime, so it contributes no outcome and no latency sample. It is not
// invisible -- the exclusion is counted in the trace-pipeline series -- so an
// operator can still distinguish "not measured" from "measured and healthy".
func TestExcludedRequestHasNoMeasuredOutcome(t *testing.T) {
	withExcludedPrefixes(t, "/metrics")
	spy := installRequestOutcomeSpy(t)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/metrics", nil)

	RecordTraceStart(c)
	RecordTraceEnd(c)

	require.Zero(t, spy.total(),
		"an excluded request has no measured lifetime to report")
	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeExcluded],
		"the exclusion itself stays visible")
}
