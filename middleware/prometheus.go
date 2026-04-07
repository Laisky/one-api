package middleware

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/metrics"
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

		// Track active requests
		metrics.GlobalRecorder.RecordHTTPActiveRequest(normalizedPath, method, 1)
		defer metrics.GlobalRecorder.RecordHTTPActiveRequest(normalizedPath, method, -1)

		// Continue processing the request
		c.Next()

		// Record metrics after request completion
		statusCode := strconv.Itoa(c.Writer.Status())
		metrics.GlobalRecorder.RecordHTTPRequest(start, normalizedPath, method, statusCode)
	}
}

// normalizePath normalizes request paths to reduce metric cardinality
func normalizePath(path string) string {
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
	if len(path) > 100 {
		return path[:100] + "..."
	}

	return path
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
