package model

import (
	"time"

	"github.com/Laisky/one-api/common/metrics"
)

// UUID migration metric label values. Every label is a compile-time constant or a value
// taken from the compile-time target registry, so no ID, UUID, DSN, error message, or other
// unbounded value can ever become a metric label.
const (
	// uuidResultSuccess marks a cycle or finalizer attempt that completed.
	uuidResultSuccess = "success"
	// uuidResultFailure marks a cycle or finalizer attempt that failed.
	uuidResultFailure = "failure"
	// uuidRowResultUpdated counts target rows a batch successfully wrote.
	uuidRowResultUpdated = "updated"
	// uuidRowResultUnresolved counts examined rows whose reference could not be resolved.
	uuidRowResultUnresolved = "unresolved"
)

// uuidMetricsRecorder returns the active recorder, never nil.
// Parameters: none.
//
// Return values:
//   - metrics.MetricsRecorder: the process recorder, or a no-op when metrics are disabled.
func uuidMetricsRecorder() metrics.MetricsRecorder {
	if metrics.GlobalRecorder == nil {
		return &metrics.NoOpRecorder{}
	}
	return metrics.GlobalRecorder
}

// recordUUIDRows publishes how many rows one batch updated and left unresolved.
// Parameters:
//   - role: authoritative database role for the target.
//   - phase: registry phase name.
//   - target: registry table and column identifier.
//   - updated: rows written by the batch.
//   - unresolved: examined rows whose reference could not be resolved.
//
// Return values: none.
func recordUUIDRows(role uuidDBRole, phase string, target string, updated int, unresolved int) {
	recorder := uuidMetricsRecorder()
	recorder.RecordUUIDBackfillRows(string(role), phase, target, uuidRowResultUpdated, updated)
	recorder.RecordUUIDBackfillRows(string(role), phase, target, uuidRowResultUnresolved, unresolved)
}

// recordUUIDCatchUpBacklog publishes whether reconcilable work remains after a cycle.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - result: observed cycle result.
//
// Return values: none.
func recordUUIDCatchUpBacklog(topology *databaseTopology, result uuidMigrationResult) {
	backlog := 0.0
	if result.updated > 0 || result.budgetExhausted {
		backlog = 1
	}
	for _, role := range topology.markerRoles() {
		uuidMetricsRecorder().UpdateUUIDBackfillBacklog(string(role), "all", backlog)
	}
}

// recordUUIDCycle publishes the duration and outcome of one coordinator cycle.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - mode: catch-up or finalizer mode.
//   - result: success or failure label.
//   - duration: wall-clock duration of the cycle.
//
// Return values: none.
func recordUUIDCycle(topology *databaseTopology, mode uuidMigrationMode, result string, duration time.Duration) {
	for _, role := range topology.markerRoles() {
		uuidMetricsRecorder().RecordUUIDBackfillCycle(string(role), string(mode), result, duration)
	}
}

// recordUUIDFinalizerResult publishes the finalizer outcome for every physical database role.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - success: whether the finalizer run completed.
//
// Return values: none.
func recordUUIDFinalizerResult(topology *databaseTopology, success bool) {
	result := uuidResultFailure
	if success {
		result = uuidResultSuccess
	}
	for _, role := range topology.markerRoles() {
		uuidMetricsRecorder().RecordUUIDBackfillFinalizer(string(role), result)
	}
}
