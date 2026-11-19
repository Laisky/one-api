package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/streaming"
)

// bindStreamingUpstreamContext gives one streamed attempt a cancellable
// upstream context, so a billing or bridge abort can stop provider I/O without
// cancelling the client request. It returns the attempt's cleanup, which the
// caller defers: it cancels the upstream context and restores the client
// request context. Restoring matters because the relay retry loop checks
// c.Request.Context() for client liveness; leaving the cancelled attempt
// context in place vetoed failover for every streamed attempt (the loop saw a
// "cancelled" request although the client was still connected). Parameter c is
// the request context. It returns the cleanup function.
func bindStreamingUpstreamContext(c *gin.Context) func() {
	clientCtx := c.Request.Context()
	upstreamCtx, cancelUpstream := context.WithCancel(gmw.Ctx(c))
	c.Request = c.Request.WithContext(upstreamCtx)
	streaming.BindUpstreamCancellation(c, cancelUpstream)
	return func() {
		cancelUpstream()
		c.Request = c.Request.WithContext(clientCtx)
	}
}
