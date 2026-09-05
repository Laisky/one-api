package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// rateLimitRecorder captures the rate-limit metric calls made by the middleware.
type rateLimitRecorder struct {
	metrics.NoOpRecorder
	mu        sync.Mutex
	hits      [][2]string
	remaining []string
}

func (r *rateLimitRecorder) RecordRateLimitHit(limitType, identifier string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits = append(r.hits, [2]string{limitType, identifier})
}

func (r *rateLimitRecorder) UpdateRateLimitRemaining(limitType, identifier string, _ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.remaining = append(r.remaining, limitType+"/"+identifier)
}

func installRateLimitRecorder(t *testing.T) *rateLimitRecorder {
	t.Helper()
	rec := &rateLimitRecorder{}
	previous := metrics.Recorder()
	metrics.SetRecorder(rec)
	t.Cleanup(func() { metrics.SetRecorder(previous) })
	return rec
}

// TestRateLimitMetricsIgnoreClientSuppliedHeader pins that the metrics
// middleware never trusts a request header: X-RateLimit-Remaining is sent by
// the client, so feeding it into a gauge let any caller write arbitrary values
// into the metrics under their own IP.
func TestRateLimitMetricsIgnoreClientSuppliedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := installRateLimitRecorder(t)

	r := gin.New()
	r.Use(PrometheusRateLimitMiddleware())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("X-RateLimit-Remaining", "7")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, rec.remaining, "client-supplied header must not populate the remaining gauge")
	require.Empty(t, rec.hits)
}

// TestRateLimitHitLabelsAreBounded runs the real in-memory web limiter and
// verifies a 429 is counted under the bounded (limit_type, identifier) label
// pair rather than the client IP, which created one series per scanner
// address. The memory limiter is driven directly so the test does not depend
// on whether the environment has Redis configured.
func TestRateLimitHitLabelsAreBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := installRateLimitRecorder(t)
	inMemoryRateLimiter.Init(config.RateLimitKeyExpirationDuration)

	r := gin.New()
	r.Use(PrometheusRateLimitMiddleware())
	r.Use(func(c *gin.Context) { memoryRateLimiter(c, 1, 60, "GW") })
	r.GET("/page", func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func() int {
		req := httptest.NewRequest(http.MethodGet, "/page", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.42") // unique to this test; limiter state is process-wide
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	require.Equal(t, http.StatusOK, do())
	require.Equal(t, http.StatusTooManyRequests, do())

	require.Equal(t, [][2]string{{"web", "ip"}}, rec.hits)
	require.Empty(t, rec.remaining)
}

// TestRateLimitLabelsCoverEveryLimiterMark pins the mark-to-label mapping for
// every limiter mark used by the factories in rate-limit.go, so adding a limiter
// without extending rateLimitLabels shows up as "unknown" here rather than in a
// dashboard.
func TestRateLimitLabelsCoverEveryLimiterMark(t *testing.T) {
	want := map[string][2]string{
		"GW": {"web", "ip"},
		"GA": {"api", "ip"},
		"CT": {"critical", "ip"},
		"DW": {"download", "ip"},
		"UP": {"upload", "ip"},
		"GR": {"relay", "token"},
		"CV": {"conversations", "token"},
		"CR": {"channel", "token"},
		"LB": {"low_balance", "user"},
		"":   {"other", "none"},
		"ZZ": {"unknown", "unknown"},
	}
	for mark, labels := range want {
		limitType, identifier := rateLimitLabels(mark)
		require.Equal(t, labels, [2]string{limitType, identifier}, "mark %q", mark)
	}
}

// TestRateLimitMetricsCountUpstream429AsOther verifies a 429 that no limiter
// produced (an upstream provider's rate limit relayed to the client) is counted
// under the "other" type and never with a client-derived identifier.
func TestRateLimitMetricsCountUpstream429AsOther(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := installRateLimitRecorder(t)

	r := gin.New()
	r.Use(PrometheusRateLimitMiddleware())
	r.GET("/relay", func(c *gin.Context) { c.Status(http.StatusTooManyRequests) })

	req := httptest.NewRequest(http.MethodGet, "/relay", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, [][2]string{{"other", "none"}}, rec.hits)
}
