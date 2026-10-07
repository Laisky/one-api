package controller

import (
	"context"
	"time"

	"github.com/Laisky/one-api/model"
)

const channelTestPersistenceTimeout = 5 * time.Second

// persistChannelTestResponseTime stores a finished probe's latency independently
// of HTTP request completion, cancellation, or its inherited deadline.
// Parameters: ctx is the Gin-free request context carrying logging and identity,
// channel is the tested channel, and milliseconds is its measured latency.
// Returns: no values; the model logs persistence failures with the original identity.
func persistChannelTestResponseTime(ctx context.Context, channel *model.Channel, milliseconds int64) {
	persistenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelTestPersistenceTimeout)
	defer cancel()
	channel.UpdateResponseTimeWithContext(persistenceCtx, milliseconds)
}
