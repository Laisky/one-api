package metrics

// Application-log export metrics (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2).
//
// The optional OTLP application-log bridge is best-effort by construction: it
// must never block a request goroutine, so a collector outage or a saturated
// queue is resolved by discarding log records. Section 9.1 permits that loss
// for telemetry, and requires it to be observable rather than silent. These
// metrics are that accounting.
//
// They deliberately describe the LOCAL side of the pipeline only. "exported"
// means the SDK handed a batch to the exporter without error, exactly as
// TraceOutcomeSpanRecorded refuses to claim collector persistence; whether the
// collector durably stored anything cannot be observed from this process.

// Application-log export outcomes. These are the ONLY values that may be passed
// as the outcome label of RecordAppLogExport. Every value is a compile-time
// constant so the metric stays low-cardinality; never pass a log message,
// logger name, request id, or error string as a label.
const (
	// AppLogExportOutcomeEmitted counts log records admitted into the export
	// pipeline. It is the denominator every other outcome is measured against.
	AppLogExportOutcomeEmitted = "emitted"
	// AppLogExportOutcomeDroppedQueueFull counts records discarded because the
	// bridge's bounded queue was already at its record or byte limit. A
	// sustained non-zero rate means the collector cannot keep pace with the
	// gateway's log volume.
	AppLogExportOutcomeDroppedQueueFull = "dropped_queue_full"
	// AppLogExportOutcomeDroppedNotReady counts records discarded because no
	// real LoggerProvider was installed yet. Startup logs emitted before
	// telemetry initialization fall here; they still reach the file and stdout
	// sinks, so this is a completeness signal for the OTLP copy only.
	AppLogExportOutcomeDroppedNotReady = "dropped_not_ready"
	// AppLogExportOutcomeDroppedShutdown counts records discarded because the
	// logger provider was already shut down. Log lines emitted by later
	// shutdown steps fall here by design: the exporter is closed before the
	// database, so the last few lines are file/stdout only.
	AppLogExportOutcomeDroppedShutdown = "dropped_shutdown"
	// AppLogExportOutcomeExported counts records the SDK handed to the OTLP
	// exporter without a transport error. It is NOT proof of collector
	// persistence.
	AppLogExportOutcomeExported = "exported"
	// AppLogExportOutcomeExportFailed counts records in a batch the exporter
	// rejected or could not deliver. The records are lost; the SDK does not
	// re-queue them.
	AppLogExportOutcomeExportFailed = "export_failed"
)

// LogExportRecorder is the OPTIONAL extension a recorder may implement to
// receive application-log export accounting.
//
// It is deliberately not part of MetricsRecorder: adding methods to that
// interface would stop any out-of-tree implementation from compiling, which the
// project's backward-compatibility rules forbid. Recorders that do not
// implement this interface are skipped.
type LogExportRecorder interface {
	// RecordAppLogExportRecords tallies count log records with the given outcome.
	RecordAppLogExportRecords(outcome string, count int)
	// UpdateAppLogExportQueue publishes the bridge queue's current occupancy and
	// the bounds it is measured against, in records and in bytes.
	UpdateAppLogExportQueue(records, recordLimit, bytes, byteLimit float64)
}

// RecordAppLogExport tallies application log records leaving, or failing to
// leave, the OTLP bridge.
//
// A recorder that does not implement LogExportRecorder is skipped, so an
// out-of-tree recorder keeps working without change.
//
// Parameters:
//   - outcome: one of the AppLogExportOutcome* constants.
//   - count: how many log records the outcome applies to.
//
// Return values: none.
func RecordAppLogExport(outcome string, count int) {
	if count <= 0 {
		return
	}
	if lr, ok := Recorder().(LogExportRecorder); ok {
		lr.RecordAppLogExportRecords(outcome, count)
	}
}

// UpdateAppLogExportQueue publishes the bridge queue's occupancy against its
// configured bounds, so an operator can see saturation before it becomes loss.
//
// Parameters:
//   - records: log records currently resident in the export pipeline.
//   - recordLimit: the configured record ceiling.
//   - bytes: estimated bytes currently resident in the export pipeline.
//   - byteLimit: the configured byte ceiling.
//
// Return values: none.
func UpdateAppLogExportQueue(records, recordLimit, bytes, byteLimit int64) {
	if lr, ok := Recorder().(LogExportRecorder); ok {
		lr.UpdateAppLogExportQueue(float64(records), float64(recordLimit),
			float64(bytes), float64(byteLimit))
	}
}
