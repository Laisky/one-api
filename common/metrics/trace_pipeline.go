package metrics

// Trace-pipeline metric label vocabulary (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1.3).
//
// IMPORTANT: these are the ONLY values that may be passed as the outcome label
// of RecordTraceRecord. Every value is a compile-time constant so the metric
// stays low-cardinality; never pass a trace id, URL, user id, or error message
// as a label.
const (
	// TraceOutcomeSampledOut counts traces the sampler decided not to persist.
	TraceOutcomeSampledOut = "sampled_out"
	// TraceOutcomeQueued counts traces accepted into the writer queue.
	TraceOutcomeQueued = "queued"
	// TraceOutcomeDroppedQueueFull counts traces discarded because the writer
	// queue was saturated. A non-zero rate means the database cannot keep pace.
	TraceOutcomeDroppedQueueFull = "dropped_queue_full"
	// TraceOutcomeDroppedClosed counts traces discarded because the sink was
	// already shut down.
	TraceOutcomeDroppedClosed = "dropped_closed"
	// TraceOutcomeWritten counts trace rows durably written by a sink.
	TraceOutcomeWritten = "written"
	// TraceOutcomeWriteFailed counts trace rows a sink failed to write.
	TraceOutcomeWriteFailed = "write_failed"
	// TraceOutcomeExported counts traces handed to the OTLP sink.
	TraceOutcomeExported = "exported"
)

// TracePipelineRecorder is the OPTIONAL extension a recorder may implement to
// receive trace-pipeline accounting.
//
// It is deliberately not part of MetricsRecorder: adding methods to that
// interface would stop any out-of-tree implementation from compiling, which the
// project's backward-compatibility rules forbid. Recorders that do not
// implement this interface are skipped.
type TracePipelineRecorder interface {
	// RecordTraceRecord tallies count trace records with the given outcome.
	RecordTraceRecord(outcome string, count int)
	// UpdateTraceQueueDepth publishes writer-queue occupancy and capacity.
	UpdateTraceQueueDepth(depth, capacity float64)
}

// RecordTraceOutcome is a convenience wrapper over Recorder() for trace
// pipeline accounting. Callers pass only the compile-time constants above.
//
// A recorder that does not implement TracePipelineRecorder is skipped, so an
// out-of-tree recorder keeps working without change.
//
// Parameters:
//   - outcome: one of the TraceOutcome* constants.
//   - count: how many trace records the outcome applies to.
//
// Return values: none.
func RecordTraceOutcome(outcome string, count int) {
	if count <= 0 {
		return
	}
	if tr, ok := Recorder().(TracePipelineRecorder); ok {
		tr.RecordTraceRecord(outcome, count)
	}
}

// UpdateTraceQueue publishes the current writer-queue occupancy.
//
// Parameters:
//   - depth: number of completed traces currently buffered.
//   - capacity: configured queue capacity.
//
// Return values: none.
func UpdateTraceQueue(depth, capacity int) {
	if tr, ok := Recorder().(TracePipelineRecorder); ok {
		tr.UpdateTraceQueueDepth(float64(depth), float64(capacity))
	}
}
