package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Optional extension interfaces are how this package adds instrumentation
// without breaking out-of-tree MetricsRecorder implementations. That mechanism
// has one sharp edge: the helper functions in this package reach a recorder
// through a type assertion, so a fan-out recorder that does NOT implement an
// extension silently swallows every sample routed through it.
//
// MultiRecorder is exactly that fan-out, and it is what a deployment running
// both Prometheus and OTel ends up with. When TraceActiveRecorder was first
// added, MultiRecorder did not implement it, so oneapi_trace_active_recorders
// would have read zero in production while trace admission was working
// correctly -- a metric that lies is worse than a missing one.
//
// These assertions are compile-time. They fail the build the moment a new
// optional interface is added to this package without a matching MultiRecorder
// fan-out method, which is the only reliable way to catch it: no runtime test
// of the helper functions can distinguish "no recorder implements this" from
// "this recorder chose not to".
var (
	_ MetricsRecorder       = (*MultiRecorder)(nil)
	_ TracePipelineRecorder = (*MultiRecorder)(nil)
	_ TraceActiveRecorder   = (*MultiRecorder)(nil)
	_ LogPipelineRecorder   = (*MultiRecorder)(nil)
)

// countingRecorder is a child recorder that implements every optional
// extension and records what it was handed.
type countingRecorder struct {
	MetricsRecorder

	traceOutcomes  map[string]int
	activeCalls    [][2]float64
	suppressions   map[string]int
	pressureValues []float64
}

// newCountingRecorder builds a countingRecorder with initialized maps.
//
// Parameters: none.
//
// Return values:
//   - *countingRecorder: a recorder ready to accept samples.
func newCountingRecorder() *countingRecorder {
	return &countingRecorder{
		traceOutcomes: map[string]int{},
		suppressions:  map[string]int{},
	}
}

// RecordTraceRecord implements TracePipelineRecorder.
//
// Parameters:
//   - outcome: the trace pipeline outcome.
//   - count: how many records it applies to.
//
// Return values: none.
func (c *countingRecorder) RecordTraceRecord(outcome string, count int) {
	c.traceOutcomes[outcome] += count
}

// UpdateTraceQueueDepth implements TracePipelineRecorder.
//
// Parameters:
//   - depth, capacity: queue occupancy and capacity.
//
// Return values: none.
func (c *countingRecorder) UpdateTraceQueueDepth(depth, capacity float64) {}

// UpdateTraceActiveRecorders implements TraceActiveRecorder.
//
// Parameters:
//   - active, limit: in-flight recorder occupancy and admission limit.
//
// Return values: none.
func (c *countingRecorder) UpdateTraceActiveRecorders(active, limit float64) {
	c.activeCalls = append(c.activeCalls, [2]float64{active, limit})
}

// RecordLogSuppression implements LogPipelineRecorder.
//
// Parameters:
//   - reason: the suppression reason.
//   - lines: discarded line count.
//   - bytes: discarded byte count.
//
// Return values: none.
func (c *countingRecorder) RecordLogSuppression(reason string, lines int, bytes int64) {
	c.suppressions[reason] += lines
}

// UpdateLogDiskPressure implements LogPipelineRecorder.
//
// Parameters:
//   - active: 1 when the emergency policy is engaged.
//
// Return values: none.
func (c *countingRecorder) UpdateLogDiskPressure(active float64) {
	c.pressureValues = append(c.pressureValues, active)
}

// TestMultiRecorderFansOutOptionalExtensions proves the fan-out actually
// forwards, not merely that it satisfies the interface. A method body that
// silently did nothing would still pass the compile-time assertions above.
func TestMultiRecorderFansOutOptionalExtensions(t *testing.T) {
	first, second := newCountingRecorder(), newCountingRecorder()
	multi := &MultiRecorder{Recorders: []MetricsRecorder{first, second}}

	multi.RecordTraceRecord(TraceOutcomeDroppedActiveLimit, 3)
	multi.UpdateTraceActiveRecorders(17, 200000)
	multi.RecordLogSuppression(LogSuppressReasonDiskPressure, 5, 4096)
	multi.UpdateLogDiskPressure(1)

	for name, rec := range map[string]*countingRecorder{"first": first, "second": second} {
		require.Equal(t, 3, rec.traceOutcomes[TraceOutcomeDroppedActiveLimit],
			"%s child must receive the trace outcome", name)
		require.Equal(t, [][2]float64{{17, 200000}}, rec.activeCalls,
			"%s child must receive active-recorder occupancy", name)
		require.Equal(t, 5, rec.suppressions[LogSuppressReasonDiskPressure],
			"%s child must receive log suppression counts", name)
		require.Equal(t, []float64{1}, rec.pressureValues,
			"%s child must receive the disk-pressure gauge", name)
	}
}

// TestMultiRecorderSkipsChildrenWithoutExtensions proves the whole point of the
// optional-interface pattern: a child that predates an extension is skipped
// rather than breaking the fan-out for the children that do implement it.
func TestMultiRecorderSkipsChildrenWithoutExtensions(t *testing.T) {
	legacy := &struct{ MetricsRecorder }{}
	modern := newCountingRecorder()
	multi := &MultiRecorder{Recorders: []MetricsRecorder{legacy, modern}}

	require.NotPanics(t, func() {
		multi.UpdateTraceActiveRecorders(42, 0)
		multi.RecordLogSuppression(LogSuppressReasonWriterFailure, 1, 128)
		multi.UpdateLogDiskPressure(0)
	}, "a child without the extension must be skipped, not panic")

	require.Equal(t, [][2]float64{{42, 0}}, modern.activeCalls,
		"the implementing child must still receive the sample")
	require.Equal(t, 1, modern.suppressions[LogSuppressReasonWriterFailure])
}
