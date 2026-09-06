package tracing

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
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
