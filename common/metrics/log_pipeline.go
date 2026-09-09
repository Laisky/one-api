package metrics

// Application-log containment metrics (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.4).
//
// The disk guard is allowed to discard application log lines to keep a gateway
// alive, but a loss that nothing counts is indistinguishable from a logger that
// silently stopped working. These metrics make the emergency policy observable:
// how much was suppressed, why, and whether the process is currently degraded.

// Log suppression reasons. These are the ONLY values that may be passed as the
// reason label of RecordLogSuppression. Every value is a compile-time constant
// so the metric stays low-cardinality; never pass a message, path, or error
// string as a label.
const (
	// LogSuppressReasonDiskPressure counts lines dropped because free space on
	// the log volume fell below LOG_MIN_FREE_DISK_MB and the emergency byte
	// budget was exhausted.
	LogSuppressReasonDiskPressure = "disk_pressure"
	// LogSuppressReasonActiveFileCap counts lines dropped because the active log
	// file reached LOG_MAX_ACTIVE_FILE_SIZE_MB and could not be rotated.
	LogSuppressReasonActiveFileCap = "active_file_cap"
	// LogSuppressReasonWriterFailure counts lines dropped because the underlying
	// writer itself returned an error. These are NOT re-logged: logging a
	// logging failure through the failing logger is how a process spins.
	LogSuppressReasonWriterFailure = "writer_failure"
)

// LogPipelineRecorder is the OPTIONAL extension a recorder may implement to
// receive application-log containment accounting.
//
// It is deliberately not part of MetricsRecorder: adding methods to that
// interface would stop any out-of-tree implementation from compiling, which the
// project's backward-compatibility rules forbid. Recorders that do not
// implement this interface are skipped.
type LogPipelineRecorder interface {
	// RecordLogSuppression tallies suppressed log lines and their bytes.
	RecordLogSuppression(reason string, lines int, bytes int64)
	// UpdateLogDiskPressure publishes whether the emergency policy is active
	// (1) or not (0), so an alert can fire on a degraded logging pipeline.
	UpdateLogDiskPressure(active float64)
}

// RecordLogSuppression tallies application log lines discarded by the disk
// guard's emergency policy.
//
// A recorder that does not implement LogPipelineRecorder is skipped, so an
// out-of-tree recorder keeps working without change.
//
// Parameters:
//   - reason: one of the LogSuppressReason* constants.
//   - lines: how many log lines were discarded.
//   - bytes: how many bytes those lines would have written.
//
// Return values: none.
func RecordLogSuppression(reason string, lines int, bytes int64) {
	if lines <= 0 {
		return
	}
	if lr, ok := Recorder().(LogPipelineRecorder); ok {
		lr.RecordLogSuppression(reason, lines, bytes)
	}
}

// UpdateLogDiskPressure publishes whether application logging is currently
// operating under the emergency policy.
//
// Parameters:
//   - active: true when the emergency policy is engaged.
//
// Return values: none.
func UpdateLogDiskPressure(active bool) {
	lr, ok := Recorder().(LogPipelineRecorder)
	if !ok {
		return
	}
	if active {
		lr.UpdateLogDiskPressure(1)
		return
	}
	lr.UpdateLogDiskPressure(0)
}
