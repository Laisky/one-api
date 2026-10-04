package openai_compatible

import (
	relaymodel "github.com/Laisky/one-api/relay/model"
	"io"
)

// claudeUsageReader observes bytes before presentation parsing and downstream
// writes. It adds no read-ahead or background drain and preserves reader errors.
// The existing response owner remains responsible for closing the upstream body.
type claudeUsageReader struct {
	reader   io.Reader
	observer *relaymodel.ResponseUsageAccumulator
}

// Read retains every byte actually received, including bytes returned with an error.
func (r *claudeUsageReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.observer.Observe(p[:n])
	}
	return n, err
}
