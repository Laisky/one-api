package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/metrics"
)

// PrometheusRateLimitMiddleware tracks rate limiting metrics
func PrometheusRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Track rate limit usage before processing
		c.Next()

		// Check if rate limit was hit (this would need to be integrated with your rate limiting logic)
		// For now, we'll track basic rate limit information

		// Get rate limit information from headers or context if available
		rateLimitType := "api" // default type
		identifier := c.ClientIP()

		// You could extend this to track different types of rate limits
		// based on your middleware setup
		if rateLimitRemaining := c.GetHeader("X-RateLimit-Remaining"); rateLimitRemaining != "" {
			if remaining, err := strconv.Atoi(rateLimitRemaining); err == nil {
				metrics.GlobalRecorder.UpdateRateLimitRemaining(rateLimitType, identifier, remaining)
			}
		}

		// Check if rate limit was exceeded (status 429)
		if c.Writer.Status() == 429 {
			metrics.GlobalRecorder.RecordRateLimitHit(rateLimitType, identifier)
		}
	}
}
