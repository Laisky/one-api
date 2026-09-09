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
	// TraceOutcomeSpanRecorded counts traces whose local SDK span was recording
	// and was enriched or ended by the OTLP sink. It is not an export-delivery
	// metric: batch processor queue overflow and collector transport failures
	// happen later and must be measured at the exporter boundary.
	TraceOutcomeSpanRecorded = "span_recorded"
	// TraceOutcomeExported is retained as a source-compatible alias. Its metric
	// value deliberately names local span recording so it cannot imply collector
	// persistence the sink is unable to observe.
	TraceOutcomeExported = TraceOutcomeSpanRecorded
	// TraceOutcomeSpanRecordFailed counts traces the OTLP sink could not record
	// at all -- a non-recording provider, a rejected span, or invalid trace
	// data. It does not represent asynchronous collector transport failure.
	TraceOutcomeSpanRecordFailed = "span_record_failed"
	// TraceOutcomeExportFailed is retained as a source-compatible alias. The
	// metric value uses span_record_failed to avoid a transport-delivery claim.
	TraceOutcomeExportFailed = TraceOutcomeSpanRecordFailed
	// TraceOutcomeExcluded counts requests that were never recorded because a
	// configured path prefix excluded them, or because TRACE_SINK is "none".
	//
	// Exclusion is a deliberate configuration choice; conflating it with
	// sampled_out or a drop makes it impossible to tell a configured saving from
	// a capacity problem (W1, "Metrics").
	TraceOutcomeExcluded = "excluded"
	// TraceOutcomeTruncated counts trace records whose retained content was cut
	// down to fit TRACE_MAX_RECORD_BYTES or TRACE_MAX_EXTERNAL_CALLS. The trace
	// is still persisted; only its detail is reduced.
	TraceOutcomeTruncated = "truncated"
	// TraceOutcomeDroppedActiveLimit counts requests that ran without a trace
	// because TRACE_MAX_ACTIVE_RECORDERS was already reached. The request itself
	// is unaffected.
	TraceOutcomeDroppedActiveLimit = "dropped_active_limit"
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

// TraceActiveRecorder is a SECOND optional extension reporting how many
// in-flight requests currently hold a trace recorder.
//
// It is a separate interface rather than a method on TracePipelineRecorder for
// the same reason that interface is separate from MetricsRecorder: adding a
// method to an existing exported interface breaks every out-of-tree
// implementation at compile time.
type TraceActiveRecorder interface {
	// UpdateTraceActiveRecorders publishes in-flight recorder occupancy and the
	// configured admission limit. A limit of 0 means unlimited.
	UpdateTraceActiveRecorders(active, limit float64)
}

// UpdateTraceActive publishes in-flight trace recorder occupancy.
//
// Parameters:
//   - active: number of requests currently holding a recorder.
//   - limit: configured TRACE_MAX_ACTIVE_RECORDERS; 0 means unlimited.
//
// Return values: none.
func UpdateTraceActive(active, limit int) {
	if tr, ok := Recorder().(TraceActiveRecorder); ok {
		tr.UpdateTraceActiveRecorders(float64(active), float64(limit))
	}
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
