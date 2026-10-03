package model

// This file implements global validation and the clean-pass epoch (AUTO-006).
//
// Validation is what stands between an expanded schema and a completion marker. It is
// deliberately expensive and deliberately not truncated by the row-cycle budget: a pass that
// silently stopped early would report "clean" for rows it never examined, and a marker written
// on that evidence would authorize compact predicates over unverified shadows.

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactValidationReport is the outcome of one full validation pass.
type compactValidationReport struct {
	// examined counts rows traversed across every authoritative target.
	examined int
	// actionable counts rows whose shadow disagreed with its authoritative text.
	actionable int
	// blockers counts observations blocked by invalid authoritative data.
	blockers int
	// blockerReason is a bounded, value-free description of the first blocker class found.
	blockerReason string
	// objectsVerified reports that schema, trigger, and index metadata all matched.
	objectsVerified bool
	// objectReason is a bounded, value-free description of the first object mismatch.
	objectReason string
}

// clean reports whether this pass found nothing actionable and every object verified.
// Parameters: none.
//
// Return values:
//   - bool: true when the pass is a clean pass.
func (report compactValidationReport) clean() bool {
	return report.objectsVerified && report.actionable == 0 && report.blockers == 0
}

// compactPassEpoch identifies the conditions under which a clean pass was observed.
//
// Two clean passes may only be combined when their epochs match. The epoch resets on restart,
// owner change, topology/marker/object-version change, retry, repair, or validation error —
// each of those means the evidence behind an earlier pass no longer describes the current
// system, and combining across them would let two halves of two different worlds count as
// proof of completion.
type compactPassEpoch struct {
	// generation is the process's init generation, so a restart cannot inherit passes.
	generation uint64
	// owner is the ownership token, so an owner change invalidates prior passes.
	owner uint64
	// topology is the topology mode the passes observed.
	topology uuidTopologyMode
	// objectVersion is the compact object version the passes verified against.
	objectVersion string
}

// equal reports whether two epochs describe the same world.
// Parameters:
//   - other: epoch to compare against.
//
// Return values:
//   - bool: true when every component matches.
func (epoch compactPassEpoch) equal(other compactPassEpoch) bool {
	return epoch.generation == other.generation &&
		epoch.owner == other.owner &&
		epoch.topology == other.topology &&
		epoch.objectVersion == other.objectVersion
}

// validateCompactObjects verifies schema, trigger, and index metadata for every target.
//
// It runs before and after the data traversal in a full pass. Running it twice is not
// redundancy: a trigger dropped midway through a long scan would otherwise let rows examined
// after the drop count as clean while nothing was maintaining their shadows.
// Parameters:
//   - ctx: context bounding the metadata reads.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when every object matches its contract.
//   - string: bounded, value-free reason for the first mismatch.
//   - error: wrapped error when metadata cannot be read.
func validateCompactObjects(ctx context.Context, topology *databaseTopology) (bool, string, error) {
	for _, table := range compactTablesForTopology(topology) {
		db := topology.handle(table.role)

		expanded, err := compactTableExpanded(ctx, db, table)
		if err != nil {
			return false, "", err
		}
		if !expanded {
			return false, "table " + table.table + " is not fully expanded with typed compact columns", nil
		}

		state, err := verifyCompactTriggers(ctx, db, table)
		if err != nil {
			return false, "", err
		}
		if !state.installed {
			return false, "table " + table.table + " sync trigger: " + state.reason, nil
		}

		for _, target := range table.targets {
			valid, err := verifyCompactIndex(ctx, db, target)
			if err != nil {
				return false, "", err
			}
			if !valid {
				return false, "compact index for " + target.id() + " is absent, misshapen, or invalid", nil
			}
		}
	}

	// The legacy-index manifest is re-verified here as well as before DDL: global validation
	// is the evidence a marker rests on, and that evidence must include the proof that no
	// legacy UUID index changed shape.
	for _, role := range topology.targetRoles() {
		ok, reason, err := ensureLegacyIndexManifest(ctx, topology.handle(role), role)
		if err != nil {
			return false, "", err
		}
		if !ok {
			return false, reason, nil
		}
	}
	return true, "", nil
}

// runCompactValidationPass performs one complete traversal of every authoritative target.
//
// A pass runs under its own validation timeout, independent of the row-cycle budget, and
// traverses to per-target primary-key high-water marks captured at pass start. Bounding by a
// captured high-water mark rather than by "the end of the table" is what makes the pass
// terminate under live traffic: trigger-atomic writes above the mark are correct by
// construction and do not need to be examined for this pass to be meaningful.
// Parameters:
//   - ctx: context bounding the pass.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - compactValidationReport: exact counts and the first bounded blocker/object reason.
//   - error: wrapped error when a query fails.
func runCompactValidationPass(ctx context.Context, topology *databaseTopology) (compactValidationReport, error) {
	report := compactValidationReport{}

	verified, reason, err := validateCompactObjects(ctx, topology)
	if err != nil {
		return report, err
	}
	if !verified {
		report.objectReason = reason
		return report, nil
	}

	for _, target := range compactTargetsForTopology(topology) {
		db := topology.handle(target.role)
		mark, err := compactHighWaterMark(ctx, db, target)
		if err != nil {
			return report, err
		}
		counts, err := validateCompactTarget(ctx, db, target, mark)
		if err != nil {
			return report, err
		}
		report.examined += counts.examined
		report.actionable += counts.actionable
		report.blockers += counts.blockers
		if report.blockerReason == "" && counts.blockerReason != "" {
			report.blockerReason = counts.blockerReason
		}
	}

	// Verify objects again after the traversal so a mid-pass drop cannot be reported clean.
	verified, reason, err = validateCompactObjects(ctx, topology)
	if err != nil {
		return report, err
	}
	if !verified {
		report.objectReason = reason
		return report, nil
	}
	report.objectsVerified = true
	return report, nil
}

// compactTargetCounts are one target's validation counts within a pass.
type compactTargetCounts struct {
	// examined counts rows traversed for this target.
	examined int
	// actionable counts rows whose shadow disagreed with its text.
	actionable int
	// blockers counts observations blocked by invalid authoritative data.
	blockers int
	// blockerReason is a bounded, value-free class description for the first blocker.
	blockerReason string
}

// validateCompactTarget traverses one target up to a captured high-water mark.
// Parameters:
//   - ctx: context bounding the queries.
//   - db: authoritative handle for the target table.
//   - target: registry target to validate.
//   - mark: primary-key high-water mark captured at pass start.
//
// Return values:
//   - compactTargetCounts: exact counts for this target.
//   - error: wrapped error when a query fails.
func validateCompactTarget(ctx context.Context, db *gorm.DB, target compactTarget,
	mark int64) (compactTargetCounts, error) {
	counts := compactTargetCounts{}
	cursor := int64(0)
	batch := compactBatchRows(target)

	for cursor < mark {
		if err := ctx.Err(); err != nil {
			return counts, errors.Wrapf(err, "validate compact target %s", target.id())
		}
		candidates, err := readCompactCandidatesBounded(ctx, db, target, cursor, mark, batch)
		if err != nil {
			return counts, err
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			counts.examined++
			if candidate.id > cursor {
				cursor = candidate.id
			}
			switch classification, _ := classifyCompactRow(target, candidate); classification {
			case compactRowSet, compactRowClear:
				counts.actionable++
			case compactRowBlocker:
				counts.blockers++
				if counts.blockerReason == "" {
					counts.blockerReason = compactBlockerReason(target, candidate)
				}
			case compactRowValid:
			}
		}
	}
	return counts, nil
}

// compactBlockerReason classifies one blocking observation without exposing its value.
//
// The class is what an operator needs to act; the UUID itself is exactly what the proposal
// forbids logging. The caller pairs this with a bounded example row ID when it reports.
// Parameters:
//   - target: registry target the observation belongs to.
//   - candidate: the blocking observation.
//
// Return values:
//   - string: bounded, value-free classification.
func compactBlockerReason(target compactTarget, candidate compactCandidate) string {
	switch {
	case !candidate.legacy.valid:
		return target.id() + " has a NULL owned uuid"
	case isBlankLegacyValue(candidate.legacy.value):
		// Blank rather than empty: PostgreSQL's CHAR(36) returns an empty value space-padded
		// to 36 characters, and reporting that as "malformed" would send an operator hunting
		// for corrupt data that is merely absent.
		return target.id() + " has an empty owned uuid"
	default:
		return target.id() + " has a malformed or non-v7 uuid"
	}
}

// readCompactCandidatesBounded materializes one batch bounded above by a high-water mark.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target to read.
//   - after: exclusive primary-key lower bound.
//   - mark: inclusive primary-key upper bound captured at pass start.
//   - limit: maximum rows to materialize.
//
// Return values:
//   - []compactCandidate: observations in ascending primary-key order.
//   - error: wrapped error when the query or a scan fails.
func readCompactCandidatesBounded(ctx context.Context, db *gorm.DB, target compactTarget,
	after int64, mark int64, limit int) ([]compactCandidate, error) {
	if limit > compactMaxMaterializedRows {
		limit = compactMaxMaterializedRows
	}
	sql := "SELECT " + quoteIdentifier(db, "id") + " AS id, " +
		quoteIdentifier(db, target.legacyColumn) + " AS legacy_value, " +
		quoteIdentifier(db, target.compactColumn) + " AS compact_value" +
		" FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, "id") + " > ? AND " + quoteIdentifier(db, "id") + " <= ?" +
		" ORDER BY " + quoteIdentifier(db, "id") + " ASC LIMIT ?"

	rows, err := db.WithContext(ctx).Raw(sql, after, mark, limit).Rows()
	if err != nil {
		return nil, errors.Wrapf(err, "read bounded compact candidates for %s", target.id())
	}
	defer func() { _ = rows.Close() }()

	candidates := make([]compactCandidate, 0, limit)
	for rows.Next() {
		candidate := compactCandidate{}
		if err := rows.Scan(&candidate.id, &candidate.legacy, &candidate.compact); err != nil {
			return nil, errors.Wrapf(err, "scan bounded compact candidate for %s", target.id())
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrapf(err, "iterate bounded compact candidates for %s", target.id())
	}
	return candidates, nil
}

// compactHighWaterMark reads one target's maximum primary key.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target to measure.
//
// Return values:
//   - int64: highest primary key, or zero for an empty table.
//   - error: wrapped error when the query fails.
func compactHighWaterMark(ctx context.Context, db *gorm.DB, target compactTarget) (int64, error) {
	var mark *int64
	sql := "SELECT MAX(" + quoteIdentifier(db, "id") + ") FROM " + quoteIdentifier(db, target.table)
	if err := db.WithContext(ctx).Raw(sql).Scan(&mark).Error; err != nil {
		return 0, errors.Wrapf(err, "read high-water mark for %s", target.id())
	}
	if mark == nil {
		return 0, nil
	}
	return *mark, nil
}

// compactNullBacklog counts one target's rows whose shadow is NULL, up to a bound.
//
// This is the cheap post-completion probe: it rides the compact index directly and answers
// "is there gap backlog" without traversing the table. It deliberately reports a bounded
// observation rather than a claimed global total, which is what the backlog metric documents.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target to probe.
//
// Return values:
//   - int: bounded count of rows with a NULL shadow.
//   - error: wrapped error when the query fails.
func compactNullBacklog(ctx context.Context, db *gorm.DB, target compactTarget) (int, error) {
	var count int64
	// The subquery's LIMIT keeps this bounded on a table with a large genuine backlog: the
	// exact total is not needed, only whether work remains and roughly how much.
	sql := "SELECT COUNT(*) FROM (SELECT 1 FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, target.compactColumn) + " IS NULL LIMIT ?) AS probe"
	if err := db.WithContext(ctx).Raw(sql, compactMaxMaterializedRows).Scan(&count).Error; err != nil {
		return 0, errors.Wrapf(err, "probe compact null backlog for %s", target.id())
	}
	return int(count), nil
}

// compactTargetsForTopology returns every compact target the topology authoritatively owns.
// Parameters:
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - []compactTarget: authoritative targets across every marker-carrying role.
func compactTargetsForTopology(topology *databaseTopology) []compactTarget {
	targets := make([]compactTarget, 0, len(compactRegistry()))
	for _, role := range topology.targetRoles() {
		targets = append(targets, compactTargetsForRole(role)...)
	}
	return targets
}

// newCompactCursor returns a fresh cursor positioned at the start of a target.
// Parameters: none.
//
// Return values:
//   - compactCursor: zeroed cursor.
func newCompactCursor() compactCursor {
	return compactCursor{updatedAt: time.Now().UTC()}
}
