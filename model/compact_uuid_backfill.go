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
	// cursor is the primary key high-water mark reached.
	cursor int64
	// wrapped reports that the traversal passed the recorded high-water mark and restarted.
	wrapped bool
}

// reconcileCompactTarget repairs one target's shadow up to the supplied budget.
//
// Candidate reads use a durable primary-key cursor in keyset order, which is what keeps the
// traversal O(batch) per query rather than degrading into a growing OFFSET scan, and what lets
// a killed worker resume where it stopped instead of rescanning from zero.
// Parameters:
//   - ctx: context bounding the queries; its deadline is the cycle's row-time budget.
//   - db: authoritative handle for the target table.
//   - target: registry target to reconcile.
//   - cursor: durable cursor state for this target.
//   - budget: remaining rows this cycle may examine.
//
// Return values:
//   - compactTargetProgress: counts and the reached cursor.
//   - error: wrapped error when a query or update fails.
func reconcileCompactTarget(ctx context.Context, db *gorm.DB, target compactTarget,
	cursor compactCursor, budget int) (compactTargetProgress, error) {
	progress := compactTargetProgress{cursor: cursor.position}
	batch := compactBatchRows(target)

	for progress.examined < budget {
		if err := ctx.Err(); err != nil {
			// A cancelled or deadline-exceeded cycle stops cleanly with its durable cursor
			// intact; it is not an error and must not start another side effect.
			progress.exhausted = true
			return progress, nil
		}
		remaining := budget - progress.examined
		if remaining < batch {
			batch = remaining
		}

		candidates, err := readCompactCandidates(ctx, db, target, progress.cursor, batch)
		if err != nil {
			return progress, err
		}
		if len(candidates) == 0 {
			// The traversal reached the end. Rewind for the next cycle and YIELD rather than
			// rescanning from zero inside this one.
			//
			// Rescanning here is what made a clean target consume the entire shared row budget
			// on every cycle — it re-read rows it had already proved correct, and every target
			// after it in the fixed order starved permanently. Measured on a live 100k fixture:
			// 490 cycles, 4.5 million rows examined, and users.uuid never reconciled once.
			// Yielding keeps the rolling audit (the next cycle starts this target from zero
			// again) while bounding what one target can take from one cycle.
			progress.cursor = 0
			progress.wrapped = true
			return progress, nil
		}

		for _, candidate := range candidates {
			progress.examined++
			if candidate.id > progress.cursor {
				progress.cursor = candidate.id
			}
			switch classification, derived := classifyCompactRow(target, candidate); classification {
			case compactRowSet, compactRowClear:
				updated, collided, err := applyCompactRepair(ctx, db, target, candidate, derived)
				if err != nil {
					return progress, err
				}
				progress.updated += updated
				if collided {
					progress.collisions++
				}
			case compactRowBlocker:
				progress.blockers++
			case compactRowValid:
			}
		}
	}
	progress.exhausted = true
	return progress, nil
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

// readCompactCandidates materializes one bounded, keyset-ordered batch of observations.
//
// The predicate deliberately reads every row past the cursor rather than only rows whose shadow
// is NULL. A NULL-only predicate would find gaps but never mismatches, and post-completion
// audit exists precisely to catch a shadow that disagrees with its text.
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
	// position is the exclusive primary-key lower bound for the next read.
	position int64
	// wrapped reports whether the traversal has already restarted from zero.
	wrapped bool
	// updatedAt records when the cursor last advanced, for diagnostics only.
	updatedAt time.Time
}
