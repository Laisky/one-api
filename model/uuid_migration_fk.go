package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// backfillFKUUIDsForPhase reconciles every registry FK target for one (target, reference)
// role pair. Both the target and the reference handle come from the explicit topology, so
// the same code path serves unified and split deployments and a stale primary logs table
// is never reachable in split mode.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - phase: named FK phase whose role pair may be reconciled.
//
// Return values:
//   - error: wrapped error when any FK backfill in the phase fails.
func backfillFKUUIDsForPhase(ctx context.Context, run *uuidMigrationRun, phase uuidFKPhase) error {
	targetDB := run.topology.handle(phase.role)
	refDB := run.topology.handle(phase.refRole)
	for _, target := range fkTargetsForRoles(phase.role, phase.refRole) {
		if err := backfillFKUUIDTarget(ctx, run, targetDB, refDB, target); err != nil {
			return errors.Wrapf(err, "backfill %s.%s", target.table, target.uuidColumn)
		}
	}
	return nil
}

// backfillFKUUIDTarget dispatches one FK target to its specialized resolver.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//
// Return values:
//   - error: wrapped error when the resolver is unknown or a read or write fails.
func backfillFKUUIDTarget(ctx context.Context, run *uuidMigrationRun, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) error {
	if !targetDB.Migrator().HasTable(target.model) || !targetDB.Migrator().HasColumn(target.model, target.uuidColumn) {
		return nil
	}
	switch target.resolver {
	case uuidResolverIntFK, uuidResolverNullableFK:
		return backfillIntFKUUIDs(ctx, run, targetDB, refDB, target)
	case uuidResolverTokenName:
		return backfillTokenNameUUIDs(ctx, run, targetDB, refDB, target)
	default:
		return errors.Errorf("unknown uuid resolver kind %q for %s.%s", target.resolver, target.table, target.uuidColumn)
	}
}

// backfillIntFKUUIDs fills a denormalized UUID copied from a positive integer reference.
// The nullable resolver differs only in how the candidate row scans the reference column:
// a NULL reference is read into a pointer and skipped rather than failing the scan.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillIntFKUUIDs(ctx context.Context, run *uuidMigrationRun, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) error {
	idColumn := quoteIdentifier(targetDB, "id")
	fkColumn := quoteIdentifier(targetDB, target.fkColumn)

	for _, missingPredicate := range missingStringPredicates(targetDB, target.uuidColumn) {
		lastID := 0
		for {
			if run.budget.spent() {
				return nil
			}
			rows, err := readFKCandidateRows(ctx, targetDB, target, idColumn, fkColumn, missingPredicate, lastID, run.budget.limit(uuidBackfillBatchSize))
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			// Advance across every examined row, including permanent orphans, so an
			// unresolvable row never pins the batch or stalls progress to higher ids.
			lastID = rows[len(rows)-1].Id

			updated, err := applyFKUUIDRows(ctx, targetDB, refDB, target, rows)
			if err != nil {
				return err
			}
			run.updated += updated
			recordUUIDBatch(ctx, run, target.role, uuidPhaseFK, target.table, target.uuidColumn, len(rows), updated, 0)
			run.budget.consume(len(rows))
		}
	}
	return nil
}

// readFKCandidateRows reads one bounded batch of rows whose FK UUID is still missing.
// Parameters:
//   - ctx: context controlling the database read.
//   - targetDB: authoritative handle for the target table.
//   - target: registry FK metadata.
//   - idColumn: quoted id column.
//   - fkColumn: quoted integer reference column.
//   - missingPredicate: indexed NULL or empty-string predicate for this pass.
//   - lastID: in-memory keyset cursor.
//   - limit: rows this read may materialize, already reduced to any remaining cycle budget.
//
// Return values:
//   - []uuidRefRow: candidate rows with observed positive references.
//   - error: wrapped database error when the read fails.
func readFKCandidateRows(ctx context.Context, targetDB *gorm.DB, target uuidFKTarget, idColumn string, fkColumn string, missingPredicate string, lastID int, limit int) ([]uuidRefRow, error) {
	query := targetDB.WithContext(ctx).
		Table(target.table).
		Select(idColumn+", "+fkColumn+" AS ref_id").
		Where(idColumn+" > ? AND "+fkColumn+" > 0 AND "+missingPredicate, lastID).
		Order(idColumn + " ASC").
		Limit(limit)

	if target.resolver == uuidResolverNullableFK {
		nullableRows := []uuidNullableRefRow{}
		if err := query.Find(&nullableRows).Error; err != nil {
			return nil, errors.Wrapf(err, "list missing nullable fk uuid rows for %s.%s", target.table, target.uuidColumn)
		}
		rows := make([]uuidRefRow, 0, len(nullableRows))
		for _, row := range nullableRows {
			if row.RefID == nil {
				continue
			}
			rows = append(rows, uuidRefRow{Id: row.Id, RefID: *row.RefID})
		}
		return rows, nil
	}

	rows := []uuidRefRow{}
	if err := query.Find(&rows).Error; err != nil {
		return nil, errors.Wrapf(err, "list missing fk uuid rows for %s.%s", target.table, target.uuidColumn)
	}
	return rows, nil
}

// applyFKUUIDRows writes FK UUID values for rows whose referenced row has a populated UUID.
// Parameters:
//   - ctx: context controlling migration writes.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//   - rows: candidate rows with observed references.
//
// Return values:
//   - int: number of rows updated.
//   - error: wrapped database error when a read or write fails.
func applyFKUUIDRows(ctx context.Context, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget, rows []uuidRefRow) (int, error) {
	refIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		refIDs = append(refIDs, row.RefID)
	}
	refs, err := loadIDUUIDMapForIDs(ctx, refDB, target.refTable, refIDs)
	if err != nil {
		return 0, errors.Wrapf(err, "resolve %s.%s references", target.table, target.uuidColumn)
	}

	values := make([]uuidConditionalValue, 0, len(rows))
	for _, row := range rows {
		uuid := refs[row.RefID]
		if uuid == "" {
			continue
		}
		values = append(values, uuidConditionalValue{
			id:         row.Id,
			conditions: []uuidColumnValue{{column: target.fkColumn, value: row.RefID}},
			value:      uuid,
		})
	}
	updated, err := applyConditionalStringColumnRows(ctx, targetDB, target.table, target.uuidColumn, values)
	if err != nil {
		return updated, errors.Wrapf(err, "set %s.%s", target.table, target.uuidColumn)
	}
	return updated, nil
}

// backfillTokenNameUUIDs fills logs.token_uuid from unambiguous historical token names.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - targetDB: authoritative handle for the logs table.
//   - refDB: authoritative handle for the tokens table.
//   - target: registry FK metadata.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillTokenNameUUIDs(ctx context.Context, run *uuidMigrationRun, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) error {
	idColumn := quoteIdentifier(targetDB, "id")
	userColumn := quoteIdentifier(targetDB, "user_id")
	nameColumn := quoteIdentifier(targetDB, "token_name")

	for _, missingPredicate := range missingStringPredicates(targetDB, target.uuidColumn) {
		lastID := 0
		for {
			if run.budget.spent() {
				return nil
			}
			rows := []uuidLogTokenRow{}
			err := targetDB.WithContext(ctx).
				Table(target.table).
				Select(idColumn+", "+userColumn+", "+nameColumn).
				Where(idColumn+" > ? AND "+userColumn+" > 0 AND "+nameColumn+" != '' AND "+missingPredicate, lastID).
				Order(idColumn + " ASC").
				Limit(run.budget.limit(uuidBackfillBatchSize)).
				Find(&rows).Error
			if err != nil {
				return errors.Wrapf(err, "list missing token uuid rows for %s", target.table)
			}
			if len(rows) == 0 {
				break
			}
			// Ambiguous historical token names are examined and skipped, never retried
			// in place, so they cannot pin the cursor.
			lastID = rows[len(rows)-1].Id

			refs, ambiguous, err := resolveTokenUUIDsForLogRows(ctx, refDB, rows)
			if err != nil {
				return errors.Wrapf(err, "resolve %s.%s references", target.table, target.uuidColumn)
			}
			values := make([]uuidConditionalValue, 0, len(rows))
			for _, row := range rows {
				uuid := refs[userTokenNameKey(row.UserID, row.TokenName)]
				if uuid == "" {
					continue
				}
				values = append(values, uuidConditionalValue{
					id: row.Id,
					conditions: []uuidColumnValue{
						{column: "user_id", value: row.UserID},
						{column: "token_name", value: row.TokenName},
					},
					value: uuid,
				})
			}
			updated, err := applyConditionalStringColumnRows(ctx, targetDB, target.table, target.uuidColumn, values)
			if err != nil {
				return errors.Wrapf(err, "set %s.%s", target.table, target.uuidColumn)
			}
			run.updated += updated
			recordUUIDBatch(ctx, run, target.role, uuidPhaseTokenName, target.table, target.uuidColumn, len(rows), updated, ambiguous)
			run.budget.consume(len(rows))
		}
	}
	return nil
}
