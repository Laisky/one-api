package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestContentGapUsesObservationTimes separates first-content latency from subsequent per-request content gaps.
func TestContentGapUsesObservationTimes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		chunks, cancel int
		times          []time.Duration
		gap, done      float64
	}{
		{"three content frames", 3, 0, []time.Duration{5, 8, 18, 20}, 10, 20},
		{"single content frame", 1, 0, []time.Duration{5, 20}, 0, 20},
		{"cancel after first", 3, 1, []time.Duration{5}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := spec{ID: "gap", Chunks: tc.chunks, Bytes: 128}
			response := httptest.NewRecorder()
			require.NoError(t, writeFixture(response, httptest.NewRequest("POST", "/", nil), fixture, "gpt-4o-mini"))
			start := time.Unix(1700000000, 0)
			calls := 0
			now := func() time.Time {
				require.Less(t, calls, len(tc.times), "role, heartbeat and stop frames must not sample content latency")
				observed := start.Add(tc.times[calls] * time.Millisecond)
				calls++
				return observed
			}
			var observed sample
			require.NoError(t, validateStreamWithClock(strings.NewReader(response.Body.String()), fixture, tc.cancel, start, &observed, now))
			require.Equal(t, len(tc.times), calls)
			require.Equal(t, 5.0, observed.TTFTMS)
			require.Equal(t, tc.done, observed.DoneMS)
			require.Equal(t, tc.gap, observed.MaxInterContentGapMS)
		})
	}
}
