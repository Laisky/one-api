package model

// This file implements bounded conditional fill and repair (AUTO-006).
//
// Everything here derives the shadow from its own row's authoritative legacy text. The worker
// never re-resolves a relationship across databases: that is the v3 backfill's job, and by the
// time compact runs, the denormalized text column is itself authoritative.
//
// Three bounds are contractual and enforced here rather than by convention: at most
// compactMaxMaterializedRows rows materialized per query, at most compactMaxBinds binds per
// statement, and a per-cycle row budget. Request input never influences any of them.

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	// compactMaxMaterializedRows caps how many rows one candidate query may materialize.
	compactMaxMaterializedRows = 1000
	// compactMaxBinds caps how many placeholders one statement may carry. Engines have their
	// own much larger limits; this bound keeps a repair statement well inside every supported
	// driver's prepared-statement envelope.
	compactMaxBinds = 900
)

// compactRowClassification is the exactly-once decision made for one observed row.
type compactRowClassification string

const (
	// compactRowSet means a valid legacy UUID whose shadow disagrees; write the derived bytes.
	compactRowSet compactRowClassification = "set"
	// compactRowClear means a NULL/empty nullable FK whose shadow is non-NULL; clear it.
	compactRowClear compactRowClassification = "clear"
	// compactRowValid means the observation is already in its correct terminal state.
	compactRowValid compactRowClassification = "valid"
	// compactRowBlocker means invalid authoritative data that cannot be repaired without
	// changing user data.
	compactRowBlocker compactRowClassification = "blocker"
)

// compactCandidate is one materialized row observation for a single target.
type compactCandidate struct {
	// id is the row's integer primary key.
	id int64
	// legacy is the exact observed authoritative text and its null state.
	legacy nullString
	// compact is the observed shadow value and its null state.
	compact nullCompactUUID
}

// classifyCompactRow decides what one observation requires, exactly once.
//
// The classification is deliberately total, and the blocker branch is why: a missing or
// malformed owned UUID, or a malformed populated FK, cannot be repaired without inventing or
// rewriting user data, which this migration never does. It reports the row and blocks
// completion instead.
// Parameters:
//   - target: registry target whose kind selects the null semantics.
//   - candidate: observed row.
//
// Return values:
//   - compactRowClassification: the single decision for this observation.
//   - nullCompactUUID: the value to write when the decision is set or clear.
func classifyCompactRow(target compactTarget, candidate compactCandidate) (compactRowClassification, nullCompactUUID) {
	derived, blocked := deriveCompactFromLegacy(target, candidate.legacy)
	if blocked {
		// Clear a non-null shadow once so a stale value cannot satisfy a compact predicate,
		// but never repeat a no-op repair: an already-NULL shadow over invalid text is the
		// correct derived state and re-clearing it every cycle would spin forever.
		if candidate.compact.valid {
			return compactRowClear, nullCompactUUID{}
		}
		return compactRowBlocker, nullCompactUUID{}
	}
	if derived.equal(candidate.compact) {
		return compactRowValid, candidate.compact
	}
	if !derived.valid {
		return compactRowClear, nullCompactUUID{}
	}
	return compactRowSet, derived
}

// compactTargetProgress is the outcome of reconciling one target within one cycle.
type compactTargetProgress struct {
	// examined counts rows materialized and classified.
	examined int
	// updated counts rows whose shadow this cycle wrote.
	updated int
	// blockers counts observations that invalid authoritative data blocks.
	blockers int
	// collisions counts rows whose derived value is already occupied by another row, which
	// cannot be corrected without mutating authoritative text.
	collisions int
	// exhausted reports that the row or time budget stopped the traversal early.
	exhausted bool
	// cursor is the gap probe's primary key high-water mark reached.
	cursor int64
	// sweepCursor is the rolling equality sweep's position reached.
	sweepCursor int64
	// wrapped reports that the traversal passed the recorded high-water mark and restarted.
	wrapped bool
}

// reconcileCompactTarget repairs one target's shadow up to the supplied budget.
//
// Reconciliation is two-phase, and the split is what makes a million-row deployment converge:
//
//   - The GAP phase probes `<compact> IS NULL` rows through the compact index (section 8.4's
//     "compact NULL probes"), so a fully-derived target answers "no gaps" in one indexed read
//     instead of re-examining every valid row. The first 1m qualification run failed its
//     four-hour deadline precisely because a clean million-row target ahead of a dirty one in
//     registry order consumed the whole shared budget re-reading rows it had already proved
//     correct, starving the dirty target to ~2k repairs/minute.
//   - The SWEEP phase advances section 8.6's bounded rolling equality scan by at most one
//     read-batch per cycle, from its own durable cursor. It is what still catches drift the
//     gap probe cannot see — a wrong non-NULL shadow, or a stale shadow over blanked text —
//     without ever letting one target's full re-scan monopolize a cycle.
//
// Both phases use durable cursors in keyset order, so a killed worker resumes where it
// stopped instead of rescanning from zero.
// Parameters:
//   - ctx: context bounding the queries; its deadline is the cycle's row-time budget.
//   - db: authoritative handle for the target table.
//   - target: registry target to reconcile.
//   - cursor: durable cursor state for this target.
//   - budget: remaining rows this cycle may examine.
//
// Return values:
//   - compactTargetProgress: counts and the reached cursors.
//   - error: wrapped error when a query or update fails.
func reconcileCompactTarget(ctx context.Context, db *gorm.DB, target compactTarget,
	cursor compactCursor, budget int) (compactTargetProgress, error) {
	progress := compactTargetProgress{cursor: cursor.position, sweepCursor: cursor.sweep}
	batch := compactBatchRows(target)

	// GAP phase: repair NULL shadows found through the compact index.
	for progress.examined < budget {
		if err := ctx.Err(); err != nil {
			// A cancelled or deadline-exceeded cycle stops cleanly with its durable cursors
			// intact; it is not an error and must not start another side effect.
			progress.exhausted = true
			return progress, nil
		}
		remaining := budget - progress.examined
		if remaining < batch {
			batch = remaining
		}

		candidates, err := readCompactGapCandidates(ctx, db, target, progress.cursor, batch)
		if err != nil {
			return progress, err
		}
		if len(candidates) == 0 {
			// No gaps past the cursor. Rewind for the next cycle and fall through to the
			// rolling sweep rather than rescanning from zero inside this one — the rescan is
			// what starved every target later in the fixed order (see the measured incidents
			// in this function's comment).
			progress.cursor = 0
			progress.wrapped = true
			break
		}
		if err := reconcileCompactBatch(ctx, db, target, candidates, &progress, &progress.cursor); err != nil {
			return progress, err
		}
	}
	if progress.examined >= budget {
		progress.exhausted = true
		return progress, nil
	}

	// SWEEP phase: advance the bounded rolling equality scan by at most one batch. This is
	// deliberately a single slice, not a loop: its job is to eventually visit every row, not
	// to visit them all now.
	slice := compactBatchRows(target)
	if remaining := budget - progress.examined; remaining < slice {
		slice = remaining
	}
	if slice <= 0 {
		progress.exhausted = true
		return progress, nil
	}
	candidates, err := readCompactCandidates(ctx, db, target, progress.sweepCursor, slice)
	if err != nil {
		return progress, err
	}
	if len(candidates) == 0 {
		progress.sweepCursor = 0
		return progress, nil
	}
	if err := reconcileCompactBatch(ctx, db, target, candidates, &progress, &progress.sweepCursor); err != nil {
		return progress, err
	}
	return progress, nil
}

// reconcileCompactBatch classifies one batch and applies its repairs in one transaction.
//
// The cursor to raise is the CALLING PHASE'S own: a sweep batch raising the gap cursor would
// make the next cycle's gap probe silently skip every row below the sweep position.
// Parameters:
//   - ctx: context bounding the statements.
//   - db: authoritative handle for the target table.
//   - target: registry target being reconciled.
//   - candidates: one read batch of observations.
//   - progress: running counts to update in place.
//   - phaseCursor: the calling phase's cursor, raised to the batch's highest id.
//
// Return values:
//   - error: wrapped error when a statement fails.
func reconcileCompactBatch(ctx context.Context, db *gorm.DB, target compactTarget,
	candidates []compactCandidate, progress *compactTargetProgress, phaseCursor *int64) error {
	repairs := make([]compactRepair, 0, len(candidates))
	for _, candidate := range candidates {
		progress.examined++
		if candidate.id > *phaseCursor {
			*phaseCursor = candidate.id
		}
		switch classification, derived := classifyCompactRow(target, candidate); classification {
		case compactRowSet, compactRowClear:
			repairs = append(repairs, compactRepair{candidate: candidate, derived: derived})
		case compactRowBlocker:
			progress.blockers++
		case compactRowValid:
		}
	}

	updated, collisions, err := applyCompactRepairBatch(ctx, db, target, repairs)
	if err != nil {
		return err
	}
	progress.updated += updated
	progress.collisions += collisions
	return nil
}

// compactBatchRows returns the row batch for one target under the configured and bind caps.
//
// The proposal fixes the formula: min(200, floor((900 - fixed_binds) / binds_per_row)). A
// repair statement binds the derived value and the three recheck predicates per row, so the
// per-row cost is what keeps a large configured batch from exceeding the bind ceiling.
// Parameters:
//   - target: registry target whose repair shape sets the per-row bind count.
//
// Return values:
//   - int: rows one candidate read and repair batch may carry.
func compactBatchRows(target compactTarget) int {
	const bindsPerRow = 4
	const fixedBinds = 2
	byBinds := (compactMaxBinds - fixedBinds) / bindsPerRow
	batch := 200
	if byBinds < batch {
		batch = byBinds
	}
	if configured := compactBatchSize(); configured < batch {
		batch = configured
	}
	if batch > compactMaxMaterializedRows {
		batch = compactMaxMaterializedRows
	}
	if batch < 1 {
		batch = 1
	}
	return batch
}

// readCompactGapCandidates materializes one keyset batch of rows whose shadow is NULL.
//
// This is section 8.4's compact NULL probe as a repair feed: the predicate is shaped so the
// planner can answer it through the compact index when gaps are sparse (the plan suite proves
// the probe shape rides that index), and blank legacy values are excluded because a blank
// nullable FK's NULL shadow is its correct terminal state — including it would re-read every
// terminal row on every pass forever. The blank test is `<> ”` rather than a Go-side trim:
// PostgreSQL's bpchar equality ignores trailing blanks, so a space-padded empty compares
// equal to ” on the engine exactly as isBlankLegacyValue treats it in Go, and MySQL's
// nonbinary comparison behaves the same way. A malformed non-blank value still matches the
// predicate and classifies as a blocker each pass, which is bounded by the blocker count and
// is precisely the visibility blocked data needs.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target to read.
//   - after: exclusive primary-key lower bound.
//   - limit: maximum rows to materialize, already bounded by the caller.
//
// Return values:
//   - []compactCandidate: gap observations in ascending primary-key order.
//   - error: wrapped error when the query or a scan fails.
func readCompactGapCandidates(ctx context.Context, db *gorm.DB, target compactTarget,
	after int64, limit int) ([]compactCandidate, error) {
	if limit > compactMaxMaterializedRows {
		limit = compactMaxMaterializedRows
	}
	legacy := quoteIdentifier(db, target.legacyColumn)
	sql := "SELECT " + quoteIdentifier(db, "id") + " AS id, " +
		legacy + " AS legacy_value, " +
		quoteIdentifier(db, target.compactColumn) + " AS compact_value" +
		" FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, target.compactColumn) + " IS NULL" +
		" AND " + legacy + " IS NOT NULL AND " + legacy + " <> ''" +
		" AND " + quoteIdentifier(db, "id") + " > ?" +
		" ORDER BY " + quoteIdentifier(db, "id") + " ASC LIMIT ?"
	return scanCompactCandidates(ctx, db, target, sql, after, limit)
}

// readCompactCandidates materializes one bounded, keyset-ordered batch of observations.
//
// Unlike the gap probe, this reads EVERY row past the cursor — valid ones included — which is
// what lets the rolling sweep and the drift tests find a wrong non-NULL shadow. It is exactly
// why it must only ever run as a bounded slice: measured on the first 1m qualification run,
// using it as the primary repair feed made one clean million-row target eat the entire cycle
// budget re-proving rows it had already proved.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target to read.
//   - after: exclusive primary-key lower bound.
//   - limit: maximum rows to materialize, already bounded by the caller.
//
// Return values:
//   - []compactCandidate: observations in ascending primary-key order.
//   - error: wrapped error when the query or a scan fails.
func readCompactCandidates(ctx context.Context, db *gorm.DB, target compactTarget,
	after int64, limit int) ([]compactCandidate, error) {
	if limit > compactMaxMaterializedRows {
		limit = compactMaxMaterializedRows
	}
	// Identifiers come only from the compile-time registry; the two binds are the cursor and
	// the limit, neither of which is request input.
	sql := "SELECT " + quoteIdentifier(db, "id") + " AS id, " +
		quoteIdentifier(db, target.legacyColumn) + " AS legacy_value, " +
		quoteIdentifier(db, target.compactColumn) + " AS compact_value" +
		" FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, "id") + " > ?" +
		" ORDER BY " + quoteIdentifier(db, "id") + " ASC LIMIT ?"
	return scanCompactCandidates(ctx, db, target, sql, after, limit)
}

// scanCompactCandidates runs one candidate query and scans its rows.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target being read, for error attribution.
//   - sql: trusted statement built from registry identifiers with (cursor, limit) binds.
//   - after: exclusive primary-key lower bound.
//   - limit: maximum rows to materialize.
//
// Return values:
//   - []compactCandidate: observations in ascending primary-key order.
//   - error: wrapped error when the query or a scan fails.
func scanCompactCandidates(ctx context.Context, db *gorm.DB, target compactTarget,
	sql string, after int64, limit int) ([]compactCandidate, error) {
	rows, err := db.WithContext(ctx).Raw(sql, after, limit).Rows()
	if err != nil {
		return nil, errors.Wrapf(err, "read compact candidates for %s", target.id())
	}
	defer func() { _ = rows.Close() }()

	candidates := make([]compactCandidate, 0, limit)
	for rows.Next() {
		candidate := compactCandidate{}
		if err := rows.Scan(&candidate.id, &candidate.legacy, &candidate.compact); err != nil {
			return nil, errors.Wrapf(err, "scan compact candidate for %s", target.id())
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrapf(err, "iterate compact candidates for %s", target.id())
	}
	return candidates, nil
}

// compactRepair is one classified row repair awaiting application.
type compactRepair struct {
	// candidate is the exact observation the classification was based on.
	candidate compactCandidate
	// derived is the value to write.
	derived nullCompactUUID
}

// applyCompactRepairBatch applies one read-batch's repairs inside a single transaction.
//
// The transaction is the difference between the 100k tier passing and the 1m tier blowing its
// four-hour deadline. Every repair is an individually conditional single-row UPDATE, and under
// autocommit each one pays its own WAL flush; a million-row backfill is then fsync-bound, and
// the 1m AUTO-T25 run missed the deadline exactly that way. Grouping a batch (bounded by
// compactBatchRows, so at most 200 rows and 800 binds of work) under one commit divides that
// cost by the batch size while changing NOTHING about per-row semantics: each statement still
// rechecks its exact observed row state, and a row that changed underneath simply updates zero
// rows inside the transaction, exactly as it did under autocommit.
//
// A unique-collision needs special handling because PostgreSQL aborts the whole transaction on
// the first error. Collisions only occur on data that is heading to blocked_validation anyway,
// so the batch retries WITHOUT the transaction on that classification, restoring the exact
// per-row collision accounting of the untransacted path.
// Parameters:
//   - ctx: context bounding the statements.
//   - db: authoritative handle for the target table.
//   - target: registry target being repaired.
//   - repairs: classified repairs from one read batch.
//
// Return values:
//   - int: rows written.
//   - int: uncorrectable unique collisions observed.
//   - error: wrapped error when a statement fails.
func applyCompactRepairBatch(ctx context.Context, db *gorm.DB, target compactTarget,
	repairs []compactRepair) (int, int, error) {
	if len(repairs) == 0 {
		return 0, 0, nil
	}

	updated := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, repair := range repairs {
			wrote, collided, err := applyCompactRepair(ctx, tx, target, repair.candidate, repair.derived)
			if err != nil {
				return err
			}
			if collided {
				// Abort to the untransacted path: on PostgreSQL the duplicate-key error has
				// already poisoned this transaction, so nothing after it could commit anyway.
				return errCompactRepairCollision
			}
			updated += wrote
		}
		return nil
	})
	if err == nil {
		return updated, 0, nil
	}
	if !errors.Is(err, errCompactRepairCollision) && !isDuplicateObjectError(err) {
		return 0, 0, err
	}

	// The batch contains at least one uncorrectable collision. Replay it row by row under
	// autocommit so every non-colliding row still gets repaired and every collision is counted.
	updated = 0
	collisions := 0
	for _, repair := range repairs {
		wrote, collided, err := applyCompactRepair(ctx, db, target, repair.candidate, repair.derived)
		if err != nil {
			return updated, collisions, err
		}
		updated += wrote
		if collided {
			collisions++
		}
	}
	return updated, collisions, nil
}

// errCompactRepairCollision aborts a repair transaction that observed a unique collision.
var errCompactRepairCollision = errors.New("compact repair batch observed a unique collision")

// applyCompactRepair writes one derived shadow under a full optimistic recheck.
//
// The recheck is what makes the repair safe without holding a lock: the statement only applies
// when the row's id, its exact observed legacy text, and its observed shadow state are all
// still what the classification saw. A trigger-atomic write that landed in between simply makes
// the update affect zero rows, and the next cycle re-observes the row. Text is never updated.
// Parameters:
//   - ctx: context bounding the statement.
//   - db: authoritative handle for the target table.
//   - target: registry target being repaired.
//   - candidate: the exact observation the classification was based on.
//   - derived: the value to write.
//
// Return values:
//   - int: 1 when the row was written, 0 when the recheck rejected it.
//   - bool: true when a unique compact collision makes this row uncorrectable.
//   - error: wrapped error when the statement fails.
func applyCompactRepair(ctx context.Context, db *gorm.DB, target compactTarget,
	candidate compactCandidate, derived nullCompactUUID) (int, bool, error) {
	dialect := dialectName(db)
	legacyColumn := quoteIdentifier(db, target.legacyColumn)
	compactColumn := quoteIdentifier(db, target.compactColumn)

	sql := "UPDATE " + quoteIdentifier(db, target.table) +
		" SET " + compactColumn + " = ?" +
		" WHERE " + quoteIdentifier(db, "id") + " = ?"
	binds := []any{compactNullBindValue(dialect, derived), candidate.id}

	// Recheck the exact observed legacy text, distinguishing SQL NULL from empty string: the
	// two are different authoritative states and a plain equality would silently match neither.
	if candidate.legacy.valid {
		sql += " AND " + legacyColumn + " = ?"
		binds = append(binds, candidate.legacy.value)
	} else {
		sql += " AND " + legacyColumn + " IS NULL"
	}
	if candidate.compact.valid {
		sql += " AND " + compactColumn + " = ?"
		binds = append(binds, compactBindValue(dialect, candidate.compact.value))
	} else {
		sql += " AND " + compactColumn + " IS NULL"
	}

	result := db.WithContext(ctx).Exec(sql, binds...)
	if result.Error != nil {
		if isDuplicateObjectError(result.Error) {
			// A unique compact collision: another row already holds the value this row's text
			// derives to. Compact is derived data, so this cannot be corrected row-by-row —
			// correcting it would mean mutating somebody's authoritative text, which this
			// migration never does.
			//
			// The collision is reported rather than swallowed. Returning a plain zero here
			// would leave the row actionable forever, and the cycle would report backfilling
			// on every pass instead of the blocked_validation the contract requires for an
			// uncorrectable permutation.
			return 0, true, nil
		}
		return 0, false, errors.Wrapf(result.Error, "repair compact shadow for %s", target.id())
	}
	return int(result.RowsAffected), false, nil
}

// compactCursor is one target's durable traversal position.
type compactCursor struct {
	// position is the exclusive primary-key lower bound for the next gap probe.
	position int64
	// sweep is the exclusive primary-key lower bound for the rolling equality sweep.
	sweep int64
	// wrapped reports whether the traversal has already restarted from zero.
	wrapped bool
	// updatedAt records when the cursor last advanced, for diagnostics only.
	updatedAt time.Time
}
