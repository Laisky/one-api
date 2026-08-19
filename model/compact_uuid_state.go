package model

// This file owns the database-derived state machine and the process-local health gate
// (AUTO-008, proposal sections 8.2 and 8.6).
//
// The health gate is the safety interlock behind every compact read. Runtime lookups consult
// it, and it is deliberately conservative: a process defaults to legacy-safe behavior until its
// own first audit passes, health expires after twice the idle interval, and an expired or
// failed audit immediately forces legacy predicates for the affected role through one atomic
// swap — never a half-updated struct that a concurrent reader could observe mid-write.

import (
	"sync/atomic"
	"time"
)

// compactState is one role's current migration state, derived from database facts.
type compactState string

const (
	// compactStateWaitingPrerequisite means an applicable v3 external-UUID marker is absent.
	compactStateWaitingPrerequisite compactState = "waiting_prerequisite"
	// compactStateExpanding means compact columns or synchronization objects are missing.
	compactStateExpanding compactState = "expanding"
	// compactStateBackfilling means actionable gaps or mismatches remain.
	compactStateBackfilling compactState = "backfilling"
	// compactStateIndexing means an expanded target lacks a valid compact index.
	compactStateIndexing compactState = "indexing"
	// compactStateValidating means schema, data, indexes, and triggers appear complete.
	compactStateValidating compactState = "validating"
	// compactStateReady means markers exist and the current audit is healthy.
	compactStateReady compactState = "ready"
	// compactStateDegraded means a marker exists but drift, a gap, or a mismatch was detected.
	compactStateDegraded compactState = "degraded"
	// compactStateBlockedValidation means invalid authoritative data or a privilege/version blocker.
	compactStateBlockedValidation compactState = "blocked_validation"
	// compactStateRetryWait means a transient database, lock, or DDL failure is backing off.
	compactStateRetryWait compactState = "retry_wait"
	// compactStatePassiveLegacy means migration is incomplete with no eligible local owner, or
	// automatic work is paused.
	compactStatePassiveLegacy compactState = "passive_legacy"
)

// compactAllStates returns every state value in precedence order.
//
// The order is the proposal's normative precedence: blocked_validation, degraded, retry_wait,
// the active phases, ready, then passive_legacy. It is also what the state gauge iterates so
// exactly one series is 1 per role.
// Parameters: none.
//
// Return values:
//   - []compactState: all states, most severe first.
func compactAllStates() []compactState {
	return []compactState{
		compactStateBlockedValidation,
		compactStateDegraded,
		compactStateRetryWait,
		compactStateWaitingPrerequisite,
		compactStateExpanding,
		compactStateIndexing,
		compactStateBackfilling,
		compactStateValidating,
		compactStateReady,
		compactStatePassiveLegacy,
	}
}

// compactStateSeverity returns a state's precedence rank, lower being more severe.
//
// Precedence exists so a lower state cannot mask a higher-severity condition: a cycle that is
// technically "backfilling" while invalid data blocks completion must report
// blocked_validation, not backfilling.
// Parameters:
//   - state: state to rank.
//
// Return values:
//   - int: precedence rank; unknown states rank last.
func compactStateSeverity(state compactState) int {
	for rank, candidate := range compactAllStates() {
		if candidate == state {
			return rank
		}
	}
	return len(compactAllStates())
}

// compactMoreSevere returns whichever of two states has precedence.
// Parameters:
//   - left: first candidate state.
//   - right: second candidate state.
//
// Return values:
//   - compactState: the state with the higher severity.
func compactMoreSevere(left compactState, right compactState) compactState {
	if compactStateSeverity(left) <= compactStateSeverity(right) {
		return left
	}
	return right
}

// compactHealth is one process's immutable audit result for one role.
//
// It is immutable by contract: the gate swaps a whole new value in rather than mutating fields,
// so a reader either sees the entire previous audit or the entire next one. A reader that could
// observe a half-updated health value might use compact predicates against objects the audit
// had just found missing.
type compactHealth struct {
	// state is the role's last observed state.
	state compactState
	// compactReadable reports whether verified compact predicates may be used.
	compactReadable bool
	// observedAt is when this audit completed, in UTC.
	observedAt time.Time
	// reason is a bounded, value-free description of why compact reads are disabled.
	reason string
}

// expired reports whether an audit is too old to trust.
// Parameters:
//   - now: current time.
//
// Return values:
//   - bool: true when the audit is older than the health TTL.
func (health compactHealth) expired(now time.Time) bool {
	if health.observedAt.IsZero() {
		return true
	}
	return now.Sub(health.observedAt) > compactHealthTTL()
}

// compactHealthGate publishes per-role health atomically for the whole process.
//
// Each role has its own atomic pointer, so a log-database audit failing never forces primary
// reads onto the legacy path, and vice versa.
var compactHealthGate = struct {
	// primary holds the primary role's latest health value.
	primary atomic.Pointer[compactHealth]
	// log holds the log role's latest health value.
	log atomic.Pointer[compactHealth]
}{}

// compactHealthSlot returns the atomic pointer backing one role's health.
// Parameters:
//   - role: database role.
//
// Return values:
//   - *atomic.Pointer[compactHealth]: the role's slot.
func compactHealthSlot(role uuidDBRole) *atomic.Pointer[compactHealth] {
	if role == uuidRoleLog {
		return &compactHealthGate.log
	}
	return &compactHealthGate.primary
}

// publishCompactHealth atomically installs one role's audit result.
// Parameters:
//   - role: database role the audit covers.
//   - health: complete audit result to publish.
//
// Return values: none.
func publishCompactHealth(role uuidDBRole, health compactHealth) {
	value := health
	compactHealthSlot(role).Store(&value)
}

// currentCompactHealth returns one role's health, applying expiry.
//
// Expiry is applied on read rather than by a timer: a process whose audit loop has stalled or
// died would otherwise keep serving compact predicates on evidence that stopped being refreshed,
// and no timer fires in a goroutine that is stuck.
// Parameters:
//   - role: database role to inspect.
//
// Return values:
//   - compactHealth: the role's health, downgraded when expired or never audited.
func currentCompactHealth(role uuidDBRole) compactHealth {
	health := compactHealthSlot(role).Load()
	if health == nil {
		// A process that has not completed its first audit has no evidence at all, so it
		// starts legacy-safe rather than optimistic.
		return compactHealth{
			state:  compactStatePassiveLegacy,
			reason: "no compact audit has completed in this process yet",
		}
	}
	if health.expired(time.Now().UTC()) {
		return compactHealth{
			state:      health.state,
			observedAt: health.observedAt,
			reason:     "compact audit is older than twice the idle interval",
		}
	}
	return *health
}

// compactReadsEnabled reports whether one role may use verified compact predicates.
//
// This is the single gate every runtime lookup consults before touching a compact column.
// Parameters:
//   - role: database role to check.
//
// Return values:
//   - bool: true when a fresh healthy audit permits compact predicates.
//   - string: bounded, value-free reason when they are disabled.
func compactReadsEnabled(role uuidDBRole) (bool, string) {
	health := currentCompactHealth(role)
	if !health.compactReadable || health.reason != "" {
		reason := health.reason
		if reason == "" {
			reason = "compact reads are not enabled for this role"
		}
		return false, reason
	}
	return true, ""
}

// disableCompactReads forces one role onto legacy predicates immediately.
//
// It is called the moment a lookup fallback or an audit failure proves the shadow cannot be
// trusted. The publish is a whole-value swap, so the disable takes effect for every reader at
// once rather than racing a concurrent update.
// Parameters:
//   - role: database role to downgrade.
//   - state: state to record alongside the downgrade.
//   - reason: bounded, value-free reason.
//
// Return values: none.
func disableCompactReads(role uuidDBRole, state compactState, reason string) {
	publishCompactHealth(role, compactHealth{
		state:           state,
		compactReadable: false,
		observedAt:      time.Now().UTC(),
		reason:          reason,
	})
}

// resetCompactHealthForTest clears the process-local gate between tests.
// Parameters: none.
//
// Return values: none.
func resetCompactHealthForTest() {
	compactHealthGate.primary.Store(nil)
	compactHealthGate.log.Store(nil)
}
