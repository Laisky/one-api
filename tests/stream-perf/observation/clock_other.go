//go:build !linux

package observation

import "github.com/Laisky/errors/v2"

// MonotonicNS rejects unsupported hosts rather than substituting wall-clock timestamps.
func MonotonicNS() (int64, error) {
	return 0, errors.New("SSE correlation requires Linux CLOCK_MONOTONIC")
}
