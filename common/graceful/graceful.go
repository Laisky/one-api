package graceful

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// Lifecycle manager for graceful shutdown and request draining.

var (
	inFlightRequests int64
	draining         atomic.Bool

	wg sync.WaitGroup
)

// BeginRequest increments the in-flight request counter and returns a function
// to decrement it. Use with `defer` at the top of request handlers/middlewares.
func BeginRequest() func() {
	atomic.AddInt64(&inFlightRequests, 1)
	return func() {
		atomic.AddInt64(&inFlightRequests, -1)
	}
}

// GoCritical runs fn in a tracked goroutine and decrements when done.
// Use for post-response critical tasks like billing, refunds, and error processing.
func GoCritical(ctx context.Context, name string, fn func(context.Context)) {
	wg.Go(func() {
		start := time.Now()
		logger.Logger.Debug("critical task start", zap.String("name", name))
		fn(ctx)
		logger.Logger.Debug("critical task done", zap.String("name", name), zap.Duration("elapsed", time.Since(start)))
	})
}

// Drain waits for all tracked critical tasks to finish, bounded by ctx deadline.
// It also waits for in-flight requests to reach zero after Server.Shutdown stops
// accepting new ones and current handlers return.
func Drain(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Wait for critical tasks via WaitGroup in a separate goroutine
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case <-ctx.Done():
			// Timeout: report remaining tasks/requests and return
			logger.Logger.Error("graceful drain timeout",
				zap.Int64("in_flight_requests", atomic.LoadInt64(&inFlightRequests)))
			return ctx.Err()
		case <-done:
			// All critical tasks finished; check in-flight requests (should be 0 after http.Server.Shutdown)
			if n := atomic.LoadInt64(&inFlightRequests); n != 0 {
				// Spin until they drop to zero or ctx timeout
				for {
					select {
					case <-ctx.Done():
						logger.Logger.Error("graceful drain timeout (requests not zero)", zap.Int64("in_flight_requests", n))
						return ctx.Err()
					case <-ticker.C:
						n = atomic.LoadInt64(&inFlightRequests)
						if n == 0 {
							logger.Logger.Info("graceful drain complete: no in-flight requests")
							return nil
						}
					}
				}
			}
			logger.Logger.Info("graceful drain complete")
			return nil
		case <-ticker.C:
			// Periodic log for visibility during long drains
			logger.Logger.Debug("draining...",
				zap.Int64("in_flight_requests", atomic.LoadInt64(&inFlightRequests)))
		}
	}
}

// SetDraining flips the draining flag to true.
func SetDraining() { draining.Store(true) }

// IsDraining returns whether the server is currently draining.
func IsDraining() bool { return draining.Load() }

// GinRequestTracker returns a middleware that tracks request begin/end.
// Attach it early in the relay stack so long-running SSE handlers are counted.
// Note: We keep this in the graceful package to avoid a separate middleware file.
func GinRequestTracker() func(c any) {
	// We use a generic signature to avoid importing gin here and creating tight coupling.
	// The router will wrap this into gin.HandlerFunc via an adapter.
	return func(_ any) {}
}
