package model

// This file publishes the compact migration's bounded observability (AUTO-012).
//
// Every label value below is a compile-time constant or comes from the compile-time registry.
// No ID, UUID, DSN, error message, row value, or fingerprint byte can reach a label or a field:
// an unbounded label explodes series cardinality, and a UUID in a metric is exactly the leak
// the proposal forbids.

import (
	"time"

	"github.com/Laisky/one-api/common/metrics"
)

// Compact action labels name one durable side effect for the actions counter.
const (
	// compactActionCycle counts whole coordinator cycles.
	compactActionCycle = "cycle"
	// compactActionAudit counts read-only health audits.
	compactActionAudit = "audit"
)

// Compact operation labels name one timed operation for the duration histogram.
const (
	// compactOperationLock times ownership acquisition.
	compactOperationLock = "lock"
	// compactOperationCycle times one whole coordinator cycle.
	compactOperationCycle = "cycle"
	// compactOperationAudit times one read-only health audit.
	compactOperationAudit = "audit"
)

// Compact backlog kind labels classify a bounded backlog observation.
const (
	// compactBacklogGap counts rows whose shadow is missing.
	compactBacklogGap = "gap"
	// compactBacklogMismatch counts rows whose shadow disagrees with its text.
	compactBacklogMismatch = "mismatch"
	// compactBacklogBlocker counts rows blocked by invalid authoritative data.
	compactBacklogBlocker = "blocker"
)

// Compact lookup fallback reason labels classify why a lookup left the compact path.
const (
	// compactFallbackMissing means the compact predicate returned no row.
	compactFallbackMissing = "missing"
	// compactFallbackMismatch means the candidate's authoritative text disagreed.
	compactFallbackMismatch = "mismatch"
	// compactFallbackExpiredHealth means this process has no fresh healthy audit.
	compactFallbackExpiredHealth = "expired_health"
	// compactFallbackCapability means the compact column or index was not usable.
	compactFallbackCapability = "capability"
)

// compactMetricsRecorder returns the active recorder, never nil.
// Parameters: none.
//
// Return values:
//   - metrics.MetricsRecorder: the process recorder, or a no-op when metrics are disabled.
func compactMetricsRecorder() metrics.MetricsRecorder {
	// metrics.Recorder() already substitutes a no-op when nothing is installed.
	return metrics.Recorder()
}

// publishCompactStateMetrics sets exactly one active state gauge per role.
//
// Every known state is written, not just the active one, which is what makes "exactly one
// current state per role" true rather than aspirational: a stale 1 left behind by a previous
// state would otherwise make two states look simultaneously current.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - state: the role's current state.
//
// Return values: none.
func publishCompactStateMetrics(topology *databaseTopology, state compactState) {
	recorder := compactMetricsRecorder()
	for _, role := range topology.targetRoles() {
		for _, candidate := range compactAllStates() {
			recorder.UpdateCompactUUIDState(string(role), string(candidate), candidate == state)
		}
	}
}

// recordCompactAction publishes one side effect's outcome.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - action: compile-time action label.
//   - result: compile-time result label.
//
// Return values: none.
func recordCompactAction(topology *databaseTopology, action string, result string) {
	recorder := compactMetricsRecorder()
	for _, role := range topology.targetRoles() {
		recorder.RecordCompactUUIDAction(string(role), action, result)
	}
}

// recordCompactDuration publishes one timed operation.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - operation: compile-time operation label.
//   - duration: measured wall-clock duration.
//
// Return values: none.
func recordCompactDuration(topology *databaseTopology, operation string, duration time.Duration) {
	recorder := compactMetricsRecorder()
	for _, role := range topology.targetRoles() {
		recorder.RecordCompactUUIDDuration(string(role), operation, duration)
	}
}

// recordCompactProgress stamps the last durable progress time in UTC.
// Parameters:
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func recordCompactProgress(topology *databaseTopology) {
	recorder := compactMetricsRecorder()
	now := float64(time.Now().UTC().Unix())
	for _, role := range topology.targetRoles() {
		recorder.UpdateCompactUUIDLastProgress(string(role), now)
	}
}

// recordCompactBacklog publishes one target's bounded backlog observation.
//
// The value is explicitly the last bounded observation, not a claimed global total: the
// coordinator never counts a whole table to fill in a gauge.
// Parameters:
//   - target: registry target the observation covers.
//   - progress: the reconciliation progress just observed.
//
// Return values: none.
func recordCompactBacklog(target compactTarget, progress compactTargetProgress) {
	recorder := compactMetricsRecorder()
	role := string(target.role)
	recorder.UpdateCompactUUIDBacklog(role, target.id(), compactBacklogGap, float64(progress.updated))
	recorder.UpdateCompactUUIDBacklog(role, target.id(), compactBacklogBlocker, float64(progress.blockers))
}

// recordCompactLookupFallback publishes one runtime lookup fallback.
// Parameters:
//   - role: database role the lookup targeted.
//   - reason: compile-time fallback reason label.
//
// Return values: none.
func recordCompactLookupFallback(role uuidDBRole, reason string) {
	compactMetricsRecorder().RecordCompactUUIDLookupFallback(string(role), reason)
}

// recordCompactMismatchBacklog publishes a bounded mismatch observation for one target.
// Parameters:
//   - target: registry target the observation covers.
//   - rows: bounded mismatch count observed.
//
// Return values: none.
func recordCompactMismatchBacklog(target compactTarget, rows int) {
	compactMetricsRecorder().UpdateCompactUUIDBacklog(
		string(target.role), target.id(), compactBacklogMismatch, float64(rows))
}
