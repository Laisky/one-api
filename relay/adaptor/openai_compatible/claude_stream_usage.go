package openai_compatible

import (
	"io"
	"sync"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeUsageReader observes bytes before presentation parsing and downstream
// writes. It adds no read-ahead or background drain and preserves reader errors.
// The existing response owner remains responsible for closing the upstream body.
// The heartbeat reader can abandon a blocked Read when the client disconnects,
// so a late Read may complete on another goroutine after settlement: mu
// serializes Observe with finish, and bytes arriving after finish are ignored
// so the settled snapshot never changes.
type claudeUsageReader struct {
	reader   io.Reader
	mu       sync.Mutex
	observer *relaymodel.ResponseUsageAccumulator
	finished bool
}

// Read retains every byte actually received, including bytes returned with an error.
func (r *claudeUsageReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.mu.Lock()
		if !r.finished {
			r.observer.Observe(p[:n])
		}
		r.mu.Unlock()
	}
	return n, err
}

// finish settles the observed receipt exactly once. Parameters: incomplete
// marks an interrupted stream, promptTokens is the admitted estimate and
// countText counts estimated output text. It returns the settled usage.
func (r *claudeUsageReader) finish(incomplete bool, promptTokens int, countText func(string) int) *relaymodel.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if incomplete {
		r.observer.MarkIncomplete()
	}
	r.finished = true
	return r.observer.Finish(promptTokens, countText)
}
