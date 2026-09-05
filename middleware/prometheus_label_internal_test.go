package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
	promrecorder "github.com/Laisky/one-api/monitor/prometheus"
)

// installPrometheusRecorder swaps the real Prometheus recorder in for the
// duration of the test so label validation is exercised for real.
func installPrometheusRecorder(t *testing.T) {
	t.Helper()
	previous := metrics.Recorder()
	metrics.SetRecorder(&promrecorder.PrometheusRecorder{})
	t.Cleanup(func() { metrics.SetRecorder(previous) })
}

// TestPrometheusMiddlewareNonUTF8PathDoesNotPanic reproduces the production
// incident: GET /%c0 decodes to the raw byte 0xC0 in URL.Path, which is not
// valid UTF-8. prometheus/client_golang validates label values and
// WithLabelValues panics, so gin.Recovery() turned the request into a 500 with
// an ERROR stack trace and the real handler never ran.
func TestPrometheusMiddlewareNonUTF8PathDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installPrometheusRecorder(t)

	for _, rawPath := range []string{"/%c0", "/%ff%fe/.env", "/api/%c0/status", "/v1/%c0"} {
		t.Run(rawPath, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.Recovery(), PrometheusMiddleware())
			handled := false
			r.NoRoute(func(c *gin.Context) {
				handled = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, rawPath, nil)
			require.False(t, utf8.ValidString(req.URL.Path), "precondition: decoded path must be invalid UTF-8")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.True(t, handled, "downstream handler must run; the metrics middleware must never abort a request")
			require.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestNormalizePathTruncationKeepsValidUTF8 covers the second panic path in the
// same function: the 100-byte truncation could slice a multi-byte rune in half,
// producing an invalid label value from a perfectly valid request path.
func TestNormalizePathTruncationKeepsValidUTF8(t *testing.T) {
	path := "/" + strings.Repeat("a", 98) + "é" // 99 bytes + 2-byte rune = 101 bytes
	require.Greater(t, len(path), 100)
	require.True(t, utf8.ValidString(path))

	got := normalizePath(path)
	require.True(t, utf8.ValidString(got), "normalized path %q must be valid UTF-8", got)
	require.True(t, strings.HasSuffix(got, "..."))
	require.LessOrEqual(t, len(got), 100+len("..."))
}

// TestNormalizePathBackwardCompatible pins the pre-existing label scheme so
// dashboards keyed on these values keep working: id/uuid/token placeholders
// under /api/, the fixed /v1 buckets, and plain paths passed through.
func TestNormalizePathBackwardCompatible(t *testing.T) {
	cases := map[string]string{
		"/":                "/",
		"/login":           "/login",
		"/api/status":      "/api/status",
		"/api/channel/123": "/api/channel/:id",
		"/api/token/550e8400-e29b-41d4-a716-446655440000": "/api/token/:uuid",
		"/api/user/self": "/api/user/self",
		"/api/redemption/abcdefghijklmnopqrstuvwxyz": "/api/redemption/:token",
		"/v1/chat/completions":                       "/v1/chat/completions",
		"/v1/completions":                            "/v1/completions",
		"/v1/embeddings":                             "/v1/embeddings",
		"/v1/moderations":                            "/v1/moderations",
		"/v1/images/generations":                     "/v1/images/:action",
		"/v1/audio/transcriptions":                   "/v1/audio/:action",
		"/v1/models":                                 "/v1/models",
		"/v1/models/gpt-4o":                          "/v1/models",
		"/v1/messages":                               "/v1/other",
		"/static/js/main.js":                         "/static/js/main.js",
		"/模型/é":                                      "/模型/é",
		"/\xc0":                                      "/�",
		"/api/\xc0\xc1/status":                       "/api/�/status",
		"/v1/\xc0":                                   "/v1/other",
		"/" + strings.Repeat("a", 120):               "/" + strings.Repeat("a", 99) + "...",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got := normalizePath(in)
			require.Equal(t, want, got)
			require.True(t, utf8.ValidString(got))
		})
	}
}

// TestPathLabelSetBoundsDistinctLabels verifies the cardinality cap: admitted
// paths keep their own label forever, new paths beyond the cap fold into the
// overflow bucket, and the cap never rewrites an already admitted path.
func TestPathLabelSetBoundsDistinctLabels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	set := newPathLabelSet(3)
	require.Equal(t, "/a", set.bound(c, "/a"))
	require.Equal(t, "/b", set.bound(c, "/b"))
	require.Equal(t, "/c", set.bound(c, "/c"))
	require.Equal(t, overflowPathLabel, set.bound(c, "/d"))
	require.Equal(t, overflowPathLabel, set.bound(c, "/e"))
	// Already admitted labels are stable after the cap is reached.
	require.Equal(t, "/a", set.bound(c, "/a"))
	require.Equal(t, "/c", set.bound(c, "/c"))
	require.Equal(t, overflowPathLabel, set.bound(c, "/d"))
}

// TestPrometheusMiddlewareBucketsScannerPathsAfterCap runs real requests
// through the middleware with a tiny cap and asserts, via the Prometheus
// registry, that only cap+1 distinct path labels exist for the request
// counter regardless of how many distinct paths were probed.
func TestPrometheusMiddlewareBucketsScannerPathsAfterCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installPrometheusRecorder(t)

	previous := pathLabels
	pathLabels = newPathLabelSet(2)
	t.Cleanup(func() { pathLabels = previous })

	r := gin.New()
	r.Use(gin.Recovery(), PrometheusMiddleware())
	r.NoRoute(func(c *gin.Context) { c.Status(http.StatusNotFound) })

	probes := []string{"/cap-test/.env", "/cap-test/.aws/credentials", "/cap-test/config.xml", "/cap-test/backup.sql", "/cap-test/%c0"}
	for _, p := range probes {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		require.Equal(t, http.StatusNotFound, w.Code)
	}

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, mf := range families {
		if mf.GetName() != "one_api_http_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "path" && (strings.HasPrefix(lp.GetValue(), "/cap-test/") || lp.GetValue() == overflowPathLabel) {
					seen[lp.GetValue()] = true
				}
			}
		}
	}
	require.Equal(t, map[string]bool{"/cap-test/.env": true, "/cap-test/.aws/credentials": true, overflowPathLabel: true}, seen)
}
