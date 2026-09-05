package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
)

// PrometheusRateLimitMiddleware counts rate-limited responses.
//
// A hit is labeled with the bounded (limit_type, identifier) pair derived from
// the limiter mark the rejecting limiter stored under ctxkey.RateLimitMark, see
// rateLimitLabels. The client IP is deliberately not a label: one series per
// scanner address is unbounded cardinality. Request headers are never consulted
// because they are client-controlled.
func PrometheusRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() != http.StatusTooManyRequests {
			return
		}
		limitType, scope := rateLimitLabels(c.GetString(ctxkey.RateLimitMark))
		metrics.Recorder().RecordRateLimitHit(limitType, scope)
	}
}

// rateLimitLabels maps a limiter mark (the second segment of the limiter's
// storage key, see rate-limit.go) to the metric label pair: limit_type names
// the limiter and identifier names what the limiter keys on. Both value sets
// are fixed at compile time. An empty mark is a 429 produced elsewhere, for
// example an upstream provider's rate limit relayed to the client.
// Parameters: mark is the value stored under ctxkey.RateLimitMark.
// Return values: limit_type and identifier label values.
func rateLimitLabels(mark string) (limitType, identifier string) {
	switch mark {
	case "GW":
		return "web", "ip"
	case "GA":
		return "api", "ip"
	case "CT":
		return "critical", "ip"
	case "DW":
		return "download", "ip"
	case "UP":
		return "upload", "ip"
	case "GR":
		return "relay", "token"
	case "CV":
		return "conversations", "token"
	case "CR":
		return "channel", "token"
	case "LB":
		return "low_balance", "user"
	case "":
		return "other", "none"
	default:
		return "unknown", "unknown"
	}
}
