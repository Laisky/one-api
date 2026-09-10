package model

import (
	"sync"
)

// uuidCursorKey identifies one candidate scan: a single target column under one
// missing-value predicate on one authoritative database.
//
// The predicate is part of the identity because a target is scanned once per predicate
// returned by missingStringPredicates, and those passes advance independently.
type uuidCursorKey struct {
	// role is the authoritative database role owning the target table.
	role uuidDBRole
	// phase is the registry phase name that owns the scan.
	phase string
	// table is the trusted target table name.
	table string
	// column is the trusted target column name.
	column string
	// predicate is the missing-value predicate for this pass, or an empty string for a
	// key that identifies a target rather than one of its scans.
	predicate string
}

// uuidCatchUpProgress carries catch-up state across the bounded cycles of one pass.
//
// A "cycle" is one coordinator invocation, bounded by the row and time budget. A "pass" is a
// complete traversal of every registry scan from its smallest id to the end of its candidate
// set, and legitimately spans several cycles. Keeping the keyset cursor per pass rather than
// per cycle is what makes catch-up incremental: a backlog larger than one cycle is traversed
// exactly once per pass instead of restarting at id 0 every few seconds.
//
// Convergence depends on it too. Permanently unresolvable rows — a log row whose token was
// hard-deleted, an ambiguous historical token name — are examined but never updated, so a
// cycle-scoped cursor makes a deployment with more such rows than one cycle's row budget
// report "backlog" forever, never observe a quiescent pass, and never finalize.
//
// The schema, index, and target-presence memos are deliberately NOT pass-scoped. They describe
// the database's shape, which does not change while the process runs; re-reading them every
// cycle is what made an idle cycle cost roughly 150 catalog statements. Any error clears them
// so the next cycle re-runs the failed step.
//
// One worker owns one progress value. The mutex is there because a topology is process-wide
// and tests drive concurrent coordinators against it; correctness never depends on two cycles
// sharing a cursor, because completion is written only by the finalizer, which traverses
// everything from id 0 under no budget at all.
type uuidCatchUpProgress struct {
	mu sync.Mutex
	// pass counts completed traversals, for logs and metrics.
	pass uint64
	// restart marks that the next cycle must begin a fresh pass.
	restart bool
	// cursors holds the last examined id per scan for the pass in flight.
	cursors map[uuidCursorKey]int
	// exhausted marks scans whose candidate query already returned zero rows this pass.
	exhausted map[uuidCursorKey]bool
	// updated counts rows written across every cycle of the pass in flight.
	updated int
	// schemaValidated records that topology schema validation already succeeded.
	schemaValidated bool
	// indexesEnsured records that candidate-index assurance already succeeded.
	indexesEnsured bool
	// presence memoizes whether a target's table and column exist.
	presence map[uuidCursorKey]bool
}

// newUUIDCatchUpProgress builds progress positioned at the start of a first pass.
// Parameters: none.
//
// Return values:
//   - *uuidCatchUpProgress: initialized progress.
func newUUIDCatchUpProgress() *uuidCatchUpProgress {
	return &uuidCatchUpProgress{
		restart:   true,
		cursors:   map[uuidCursorKey]int{},
		exhausted: map[uuidCursorKey]bool{},
		presence:  map[uuidCursorKey]bool{},
	}
}

// beginCycle starts a new pass when the previous one finished or failed, and reports the
// pass number the caller is now working on.
// Parameters: none.
//
// Return values:
//   - uint64: pass number this cycle belongs to.
func (progress *uuidCatchUpProgress) beginCycle() uint64 {
	if progress == nil {
		return 0
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	if progress.restart {
		progress.restart = false
		progress.pass++
		progress.cursors = map[uuidCursorKey]int{}
		progress.exhausted = map[uuidCursorKey]bool{}
		progress.updated = 0
	}
	return progress.pass
}

// completePass records that every scan reached the end of its candidate set, so the next
// cycle starts a fresh traversal.
// Parameters: none.
//
// Return values: none.
func (progress *uuidCatchUpProgress) completePass() {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.restart = true
}

// failPass abandons the pass in flight and clears the shape memos.
//
// A failed cycle may have advanced a cursor past rows it never wrote, so the pass it belongs
// to can no longer be trusted to have examined everything; the next cycle restarts it. The
// memos go too, because a failure is exactly the case where a missing table, a missing column,
// or an unbuilt index has to be looked at again.
// Parameters: none.
//
// Return values: none.
func (progress *uuidCatchUpProgress) failPass() {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.restart = true
	progress.schemaValidated = false
	progress.indexesEnsured = false
	progress.presence = map[uuidCursorKey]bool{}
}

// scanStart returns where one scan resumes and whether it already finished this pass.
// Parameters:
//   - key: scan identity.
//
// Return values:
//   - int: keyset cursor to resume from; zero for a scan not yet started.
//   - bool: true when the scan is already exhausted for this pass.
func (progress *uuidCatchUpProgress) scanStart(key uuidCursorKey) (int, bool) {
	if progress == nil {
		return 0, false
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	if progress.exhausted[key] {
		return 0, true
	}
	return progress.cursors[key], false
}

// advanceScan records the highest id one scan has examined.
// Parameters:
//   - key: scan identity.
//   - lastID: highest examined id.
//
// Return values: none.
func (progress *uuidCatchUpProgress) advanceScan(key uuidCursorKey, lastID int) {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	if lastID > progress.cursors[key] {
		progress.cursors[key] = lastID
	}
}

// finishScan marks one scan exhausted for the pass in flight.
// Parameters:
//   - key: scan identity.
//
// Return values: none.
func (progress *uuidCatchUpProgress) finishScan(key uuidCursorKey) {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.exhausted[key] = true
}

// addUpdated accumulates rows written into the pass total.
// Parameters:
//   - rows: rows written by one batch.
//
// Return values: none.
func (progress *uuidCatchUpProgress) addUpdated(rows int) {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.updated += rows
}

// passUpdated reports the rows written so far during the pass in flight.
// Parameters: none.
//
// Return values:
//   - int: rows written during this pass.
func (progress *uuidCatchUpProgress) passUpdated() int {
	if progress == nil {
		return 0
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	return progress.updated
}

// schemaIsValidated reports whether topology schema validation already succeeded.
// Parameters: none.
//
// Return values:
//   - bool: true when validation may be skipped.
func (progress *uuidCatchUpProgress) schemaIsValidated() bool {
	if progress == nil {
		return false
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	return progress.schemaValidated
}

// markSchemaValidated memoizes a successful topology schema validation.
// Parameters: none.
//
// Return values: none.
func (progress *uuidCatchUpProgress) markSchemaValidated() {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.schemaValidated = true
}

// indexesAreEnsured reports whether candidate-index assurance already succeeded.
// Parameters: none.
//
// Return values:
//   - bool: true when index assurance may be skipped.
func (progress *uuidCatchUpProgress) indexesAreEnsured() bool {
	if progress == nil {
		return false
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	return progress.indexesEnsured
}

// markIndexesEnsured memoizes a successful candidate-index assurance.
// Parameters: none.
//
// Return values: none.
func (progress *uuidCatchUpProgress) markIndexesEnsured() {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.indexesEnsured = true
}

// targetPresence returns a memoized table-and-column existence answer.
// Parameters:
//   - key: target identity, with an empty predicate.
//
// Return values:
//   - bool: memoized answer.
//   - bool: true when an answer was memoized.
func (progress *uuidCatchUpProgress) targetPresence(key uuidCursorKey) (bool, bool) {
	if progress == nil {
		return false, false
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	present, known := progress.presence[key]
	return present, known
}

// memoizeTargetPresence records a table-and-column existence answer.
// Parameters:
//   - key: target identity, with an empty predicate.
//   - present: whether the table and column exist.
//
// Return values: none.
func (progress *uuidCatchUpProgress) memoizeTargetPresence(key uuidCursorKey, present bool) {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.presence[key] = present
}
