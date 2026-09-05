package middleware

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// MetricsAuth protects the /metrics endpoint with a dedicated Bearer token.
// Returns 403 if METRICS_TOKEN is not configured, 401 if the token is invalid.
func MetricsAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := config.MetricsToken
		if token == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "metrics endpoint requires METRICS_TOKEN configuration",
			})
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid metrics token, please check your METRICS_TOKEN configuration",
			})
			c.Abort()
			return
		}

		provided := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid metrics token, please check your METRICS_TOKEN configuration",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// PrometheusMiddleware instruments HTTP endpoints with Prometheus metrics
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Normalize path to avoid high cardinality
		normalizedPath := normalizePath(path)
		if !utf8.ValidString(path) {
			// URL.Path is percent-decoded, so "/%c0" arrives as the raw byte
			// 0xC0. Such values are not valid label values (see
			// metrics.SanitizeLabelValue); log the sanitized form only.
			gmw.GetLogger(c).Debug("request path is not valid UTF-8, sanitized for metrics labels",
				zap.String("path", normalizedPath))
		}
		normalizedPath = pathLabels.bound(c, normalizedPath)

		// Track active requests
		metrics.Recorder().RecordHTTPActiveRequest(normalizedPath, method, 1)
		defer metrics.Recorder().RecordHTTPActiveRequest(normalizedPath, method, -1)

		// Continue processing the request
		c.Next()

		// Record metrics after request completion
		statusCode := strconv.Itoa(c.Writer.Status())
		metrics.Recorder().RecordHTTPRequest(start, normalizedPath, method, statusCode)
	}
}

// maxPathLabelBytes caps the byte length of a path label value; longer paths
// are truncated on a rune boundary and suffixed with "...".
const maxPathLabelBytes = 100

// maxDistinctPathLabels bounds how many distinct normalized paths may become
// metric label values over the lifetime of the process.
//
// Why: every distinct path label creates permanent time series (a histogram
// alone is buckets+2 series per path/method/status) in both the Prometheus
// registry and the cumulative OpenTelemetry aggregators, and vulnerability
// scanners probe thousands of distinct paths (/.env, /backup/.aws/credentials,
// ...). Legitimate traffic uses a few hundred distinct normalized paths at most
// (API routes with ids collapsed, /v1 relay routes collapsed, frontend assets),
// so this cap only engages under abuse.
const maxDistinctPathLabels = 1000

// overflowPathLabel is the path label used once maxDistinctPathLabels distinct
// paths have been seen. It mirrors the existing "/v1/other" placeholder.
const overflowPathLabel = "/other"

// pathLabelSet tracks the distinct path labels emitted so far and folds any
// new path beyond its limit into overflowPathLabel.
type pathLabelSet struct {
	mu       sync.RWMutex
	seen     map[string]struct{}
	limit    int
	warnOnce sync.Once
}

// pathLabels is the process-wide path label set used by PrometheusMiddleware.
var pathLabels = newPathLabelSet(maxDistinctPathLabels)

// newPathLabelSet creates a pathLabelSet that admits at most limit distinct
// labels.
func newPathLabelSet(limit int) *pathLabelSet {
	return &pathLabelSet{
		seen:  make(map[string]struct{}, 64),
		limit: limit,
	}
}

// bound returns the label to record for a normalized path: the path itself
// while the set still has room (or the path was already admitted), otherwise
// overflowPathLabel. The first overflow is logged once at WARN because it
// means the metric series set is being folded and operators may want to look
// at what is hitting the server.
func (s *pathLabelSet) bound(c *gin.Context, normalizedPath string) string {
	s.mu.RLock()
	_, admitted := s.seen[normalizedPath]
	s.mu.RUnlock()
	if admitted {
		return normalizedPath
	}

	s.mu.Lock()
	if _, admitted = s.seen[normalizedPath]; admitted {
		s.mu.Unlock()
		return normalizedPath
	}
	if len(s.seen) < s.limit {
		s.seen[normalizedPath] = struct{}{}
		s.mu.Unlock()
		return normalizedPath
	}
	s.mu.Unlock()

	s.warnOnce.Do(func() {
		gmw.GetLogger(c).Warn("distinct HTTP metrics path labels reached the cap, further new paths are bucketed",
			zap.Int("cap", s.limit),
			zap.String("bucket", overflowPathLabel),
			zap.String("path", normalizedPath))
	})
	return overflowPathLabel
}

// normalizePath normalizes request paths to reduce metric cardinality and
// guarantees the result is valid UTF-8, which Prometheus label values and OTLP
// string attributes both require.
func normalizePath(path string) string {
	// URL.Path is percent-decoded and may carry arbitrary bytes ("/%c0").
	path = metrics.SanitizeLabelValue(path)

	// Handle common patterns to avoid high cardinality

	// Replace UUIDs and IDs with placeholders
	if strings.Contains(path, "/api/") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			// Replace numeric IDs
			if isNumeric(part) {
				parts[i] = ":id"
			}
			// Replace UUIDs (basic pattern)
			if len(part) == 36 && strings.Count(part, "-") == 4 {
				parts[i] = ":uuid"
			}
			// Replace API keys or tokens (longer than 20 chars and alphanumeric)
			if len(part) > 20 && isAlphanumeric(part) {
				parts[i] = ":token"
			}
		}
		path = strings.Join(parts, "/")
	}

	// Handle relay routes
	if strings.HasPrefix(path, "/v1/") {
		// OpenAI API routes
		if strings.HasPrefix(path, "/v1/chat/completions") {
			return "/v1/chat/completions"
		}
		if strings.HasPrefix(path, "/v1/completions") {
			return "/v1/completions"
		}
		if strings.HasPrefix(path, "/v1/embeddings") {
			return "/v1/embeddings"
		}
		if strings.HasPrefix(path, "/v1/moderations") {
			return "/v1/moderations"
		}
		if strings.HasPrefix(path, "/v1/images/") {
			return "/v1/images/:action"
		}
		if strings.HasPrefix(path, "/v1/audio/") {
			return "/v1/audio/:action"
		}
		if strings.HasPrefix(path, "/v1/models") {
			return "/v1/models"
		}
		return "/v1/other"
	}

	// Limit path length to prevent extremely long paths
	if len(path) > maxPathLabelBytes {
		return truncateOnRuneBoundary(path, maxPathLabelBytes) + "..."
	}

	return path
}

// truncateOnRuneBoundary returns the longest prefix of s that is at most
// maxBytes long and does not end in the middle of a multi-byte rune, so a
// truncated valid string stays valid UTF-8.
func truncateOnRuneBoundary(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAlphanumeric checks if a string is alphanumeric
func isAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
