package controller

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/relay/meta"
)

// inFlightRecorder captures channel in-flight deltas.
type inFlightRecorder struct {
	metrics.NoOpRecorder
	mu     sync.Mutex
	deltas []float64
}

func (r *inFlightRecorder) UpdateChannelRequestsInFlight(_ int, _, _ string, delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltas = append(r.deltas, delta)
}

func (r *inFlightRecorder) snapshot() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]float64(nil), r.deltas...)
}

// TestRecordChannelRequestDoesNotLeaveGoroutineBehind pins that tracking an
// in-flight channel request must not spawn a goroutine that sleeps for up to a
// minute per request: at production request rates that is thousands of parked
// goroutines, and the gauge it maintained measured "started within the last
// minute", not "in flight".
func TestRecordChannelRequestDoesNotLeaveGoroutineBehind(t *testing.T) {
	rec := &inFlightRecorder{}
	previous := metrics.Recorder()
	metrics.SetRecorder(rec)
	t.Cleanup(func() { metrics.SetRecorder(previous) })

	m := &meta.Meta{ChannelId: 7, ChannelType: 1}
	before := runtime.NumGoroutine()
	dones := make([]func(), 0, 50)
	for i := 0; i < 50; i++ {
		dones = append(dones, PrometheusMonitor.RecordChannelRequest(m))
	}
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()

	require.LessOrEqual(t, after, before+2, "in-flight tracking must not park one goroutine per request")
	deltas := rec.snapshot()
	require.Len(t, deltas, 50)
	for _, d := range deltas {
		require.Equal(t, float64(1), d, "only the increment happens at request start")
	}

	for _, done := range dones {
		done()
	}
	require.Len(t, rec.snapshot(), 100, "each completion decrements exactly once")
}

// TestRecordChannelRequestCompletionIsIdempotent verifies the completion
// callback decrements the in-flight gauge exactly once even when a handler
// calls it more than once (for example an explicit call plus a defer).
func TestRecordChannelRequestCompletionIsIdempotent(t *testing.T) {
	rec := &inFlightRecorder{}
	previous := metrics.Recorder()
	metrics.SetRecorder(rec)
	t.Cleanup(func() { metrics.SetRecorder(previous) })

	done := PrometheusMonitor.RecordChannelRequest(&meta.Meta{ChannelId: 3, ChannelType: 1})
	require.Equal(t, []float64{1}, rec.snapshot())
	done()
	done()
	done()
	require.Equal(t, []float64{1, -1}, rec.snapshot())
}
