package tracing

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// withExcludedPrefixes installs a never-trace list for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - prefixes: the prefixes to install.
//
// Return values: none.
func withExcludedPrefixes(t *testing.T, prefixes ...string) {
	t.Helper()
	prev := config.TraceExcludedPathPrefixes
	config.TraceExcludedPathPrefixes = prefixes
	t.Cleanup(func() { config.TraceExcludedPathPrefixes = prev })
}

// TestPathExcluded verifies prefix matching against the never-trace list.
func TestPathExcluded(t *testing.T) {
	withExcludedPrefixes(t, "/api/status", "/metrics", "/assets")

	require.True(t, PathExcluded("/api/status"))
	require.True(t, PathExcluded("/metrics"))
	require.True(t, PathExcluded("/assets/index-abc123.js"))
	require.False(t, PathExcluded("/v1/chat/completions"))
	require.False(t, PathExcluded("/api/user/self"))
}

// TestPathExcludedEmptyList verifies an empty list traces everything, which is
// what TRACE_EXCLUDED_PATH_PREFIXES="-" configures.
func TestPathExcludedEmptyList(t *testing.T) {
	withExcludedPrefixes(t)

	require.False(t, PathExcluded("/metrics"))
	require.False(t, PathExcluded("/v1/chat/completions"))
}

// TestRequestExcludedHandlesMalformedContexts verifies a context without a
// request is treated as excluded rather than panicking.
func TestRequestExcludedHandlesMalformedContexts(t *testing.T) {
	withExcludedPrefixes(t, "/metrics")

	require.True(t, requestExcluded(nil))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, requestExcluded(c), "a context with no request cannot be traced")

	c.Request = httptest.NewRequest("GET", "/metrics", nil)
	require.True(t, requestExcluded(c))

	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	require.False(t, requestExcluded(c))
}

// TestRequestExcludedWhenTracingDisabled verifies TRACE_SINK=none excludes
// every request regardless of path.
func TestRequestExcludedWhenTracingDisabled(t *testing.T) {
	withExcludedPrefixes(t)

	prev := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = prev })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	require.True(t, requestExcluded(c))
}

// newExcludedTestContext builds a gin context for one request path.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//   - path: the request path.
//
// Return values:
//   - *gin.Context: a context carrying a POST request for that path.
func newExcludedTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	return c
}

// TestExcludedPathIsCountedOnce verifies a path exclusion is observable.
//
// Before this, requestExcluded returned from every hook with no metric at all,
// so an operator could not tell a configured saving (an excluded health probe)
// from a capacity problem (a dropped trace). The tally must be per request, not
// per hook: RecordTraceStart and RecordTraceEnd both reach the exclusion branch.
func TestExcludedPathIsCountedOnce(t *testing.T) {
	withExcludedPrefixes(t, "/metrics")
	spy := installTraceOutcomeSpy(t)

	c := newExcludedTestContext(t, "/metrics")
	RecordTraceStart(c)
	RecordTraceTimestamp(c, "request_forwarded")
	RecordTraceEnd(c)

	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeExcluded],
		"one excluded request must produce exactly one excluded sample")
	require.Zero(t, spy.outcomes[metrics.TraceOutcomeSampledOut],
		"an exclusion is not a sampling decision")
	require.Zero(t, spy.outcomes[metrics.TraceOutcomeQueued])
}

// TestTraceSinkNoneIsCountedAsExcluded verifies TRACE_SINK=none is reported as a
// deliberate exclusion rather than silently producing nothing.
func TestTraceSinkNoneIsCountedAsExcluded(t *testing.T) {
	withExcludedPrefixes(t)
	prev := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = prev })

	spy := installTraceOutcomeSpy(t)

	c := newExcludedTestContext(t, "/v1/chat/completions")
	RecordTraceStart(c)
	RecordTraceEnd(c)

	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeExcluded])
}

// TestTracedPathIsNotCountedAsExcluded verifies the tally does not fire for a
// request that is actually traced.
func TestTracedPathIsNotCountedAsExcluded(t *testing.T) {
	withExcludedPrefixes(t, "/metrics")
	prevSinks, prevMode := config.TraceSinks, config.TraceWriteMode
	config.TraceSinks, config.TraceWriteMode = []string{config.TraceSinkDB}, config.TraceWriteModeBatched
	t.Cleanup(func() { config.TraceSinks, config.TraceWriteMode = prevSinks, prevMode })

	spy := installTraceOutcomeSpy(t)

	RecordTraceStart(newExcludedTestContext(t, "/v1/chat/completions"))

	require.Zero(t, spy.outcomes[metrics.TraceOutcomeExcluded])
}

// TestExcludedTallyIgnoresRequestlessContexts verifies a synthetic context with
// no HTTP request is not counted: no request was excluded there.
func TestExcludedTallyIgnoresRequestlessContexts(t *testing.T) {
	withExcludedPrefixes(t)
	spy := installTraceOutcomeSpy(t)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	noteRequestExcluded(nil)
	noteRequestExcluded(c)

	require.Empty(t, spy.outcomes)
}
