package model

import (
	"sort"
	"testing"
	"time"
)

// Scale-protocol helpers: repeated independent fixtures and median statistics used by
// the T27 qualification run.

// runUUIDScaleFixtures runs one fixture size several times, resetting it independently each
// time so the recorded numbers are not contaminated by a warmed cache or a previous run's heap.
// Parameters:
//   - t: test handle used for assertions.
//   - dialect: sqlite, mysql, or postgres.
//   - logRows: number of legacy log rows.
//   - tokenRows: number of legacy token rows.
//   - repeats: number of independent fixtures to run.
//
// Return values:
//   - []uuidScaleRun: recorded evidence for each run.
func runUUIDScaleFixtures(t *testing.T, dialect string, logRows int, tokenRows int, repeats int) []uuidScaleRun {
	t.Helper()
	runs := make([]uuidScaleRun, 0, repeats)
	for i := 0; i < repeats; i++ {
		runs = append(runs, runUUIDScaleFixture(t, dialect, logRows, tokenRows))
	}
	return runs
}

// uuidScaleHeaps projects the peak heap of each run.
// Parameters:
//   - runs: recorded runs.
//
// Return values:
//   - []uint64: peak heap bytes per run.
func uuidScaleHeaps(runs []uuidScaleRun) []uint64 {
	values := make([]uint64, 0, len(runs))
	for _, run := range runs {
		values = append(values, run.peakHeapBytes)
	}
	return values
}

// uuidScaleDurations projects the wall time of each run.
// Parameters:
//   - runs: recorded runs.
//
// Return values:
//   - []time.Duration: wall time per run.
func uuidScaleDurations(runs []uuidScaleRun) []time.Duration {
	values := make([]time.Duration, 0, len(runs))
	for _, run := range runs {
		values = append(values, run.duration)
	}
	return values
}

// medianUint64 returns the median of the supplied values.
// Parameters:
//   - values: at least one value.
//
// Return values:
//   - uint64: median value.
func medianUint64(values []uint64) uint64 {
	sorted := append([]uint64(nil), values...)
	sort.Slice(sorted, func(i int, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// medianDuration returns the median of the supplied durations.
// Parameters:
//   - values: at least one duration.
//
// Return values:
//   - time.Duration: median duration.
func medianDuration(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i int, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}
