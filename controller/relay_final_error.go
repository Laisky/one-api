package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// writeRelayFinalError serializes an API error only before downstream output has
// committed. It returns whether it wrote a response. Late errors still reach the
// caller's billing and monitoring paths, but cannot append JSON to JSON or SSE.
func writeRelayFinalError(c *gin.Context, apiErr *model.ErrorWithStatusCode) bool {
	if c == nil || apiErr == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.Error})
	return true
}
