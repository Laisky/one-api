package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
)

// validateFKUUIDColumn verifies one denormalized UUID column against its live owner.
// Two conditions block completion: a missing FK UUID whose current owner already has a
// UUID (a fillable gap), and a populated FK UUID that disagrees with its live owner.
// Permanent orphans, absent nullable references, and ambiguous historical token names are
// reported but never block, because their FK UUID is legitimately unresolvable.
// Every read is bounded: target rows in batches, references in deduplicated chunks.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//
// Return values:
//   - []uuidValidationIssue: blocking findings for this column.
//   - error: wrapped database error when a read fails.
func validateFKUUIDColumn(ctx context.Context, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) ([]uuidValidationIssue, error) {
	if !targetDB.Migrator().HasTable(target.model) || !targetDB.Migrator().HasColumn(target.model, target.uuidColumn) {
		return nil, nil
	}
	if target.resolver == uuidResolverTokenName {
		return validateTokenNameUUIDColumn(ctx, targetDB, refDB, target)
	}

	issues := []uuidValidationIssue{}
	fillable, fillableExamples, orphans, err := scanFillableFKGaps(ctx, targetDB, refDB, target)
	if err != nil {
		return nil, err
	}
	if fillable > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: target.uuidColumn, kind: "fillable missing fk uuid",
			count: fillable, examples: fillableExamples,
		})
	}
	mismatched, mismatchExamples, err := scanMismatchedFKUUIDs(ctx, targetDB, refDB, target)
	if err != nil {
		return nil, err
	}
	if mismatched > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: target.uuidColumn, kind: "populated fk uuid disagrees with live owner",
			count: mismatched, examples: mismatchExamples,
		})
	}
	if orphans > 0 {
		uuidMigrationLogger(ctx).Info("external uuid validation tolerated permanent orphans",
			zap.String("table", target.table),
			zap.String("column", target.uuidColumn),
			zap.Int("orphan_rows", orphans))
	}
	return issues, nil
}

// scanFillableFKGaps counts missing FK UUIDs whose current owner already has a UUID.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//
// Return values:
//   - int: number of fillable missing FK UUID rows.
//   - []int: bounded example row ids.
//   - int: number of tolerated permanent orphan rows.
//   - error: wrapped database error when a read fails.
func scanFillableFKGaps(ctx context.Context, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) (int, []int, int, error) {
	idColumn := quoteIdentifier(targetDB, "id")
	fkColumn := quoteIdentifier(targetDB, target.fkColumn)

	fillable := 0
	orphans := 0
	examples := []int{}
	for _, missingPredicate := range missingStringPredicates(targetDB, target.uuidColumn) {
		lastID := 0
		for {
			rows, err := readFKCandidateRows(ctx, targetDB, target, idColumn, fkColumn, missingPredicate, lastID, uuidBackfillBatchSize)
			if err != nil {
				return 0, nil, 0, err
			}
			if len(rows) == 0 {
				break
			}
			lastID = rows[len(rows)-1].Id

			refIDs := make([]int, 0, len(rows))
			for _, row := range rows {
				refIDs = append(refIDs, row.RefID)
			}
			refs, err := loadIDUUIDMapForIDs(ctx, refDB, target.refTable, refIDs)
			if err != nil {
				return 0, nil, 0, errors.Wrapf(err, "resolve %s.%s validation references", target.table, target.uuidColumn)
			}
			for _, row := range rows {
				if refs[row.RefID] == "" {
					orphans++
					continue
				}
				fillable++
				if len(examples) < uuidValidationExampleLimit {
					examples = append(examples, row.Id)
				}
			}
		}
	}
	return fillable, examples, orphans, nil
}

// scanMismatchedFKUUIDs counts populated FK UUIDs that disagree with their live owner.
// A reference whose owner no longer exists is left alone: catch-up never invents a UUID
// for a deleted owner and a historical value is not evidence of corruption.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - targetDB: authoritative handle for the target table.
//   - refDB: authoritative handle for the referenced owner table.
//   - target: registry FK metadata.
//
// Return values:
//   - int: number of mismatched populated FK UUID rows.
//   - []int: bounded example row ids.
//   - error: wrapped database error when a read fails.
func scanMismatchedFKUUIDs(ctx context.Context, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) (int, []int, error) {
	idColumn := quoteIdentifier(targetDB, "id")
	fkColumn := quoteIdentifier(targetDB, target.fkColumn)
	uuidColumn := quoteIdentifier(targetDB, target.uuidColumn)

	mismatched := 0
	examples := []int{}
	lastID := 0
	for {
		rows := []struct {
			Id    int    `gorm:"column:id"`
			RefID int    `gorm:"column:ref_id"`
			UUID  string `gorm:"column:fk_uuid"`
		}{}
		err := targetDB.WithContext(ctx).
			Table(target.table).
			Select(idColumn+", "+fkColumn+" AS ref_id, "+uuidColumn+" AS fk_uuid").
			Where(idColumn+" > ? AND "+fkColumn+" > 0 AND "+uuidColumn+" IS NOT NULL AND "+uuidColumn+" != ''", lastID).
			Order(idColumn + " ASC").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return 0, nil, errors.Wrapf(err, "scan populated fk uuids for %s.%s", target.table, target.uuidColumn)
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].Id

		refIDs := make([]int, 0, len(rows))
		for _, row := range rows {
			refIDs = append(refIDs, row.RefID)
		}
		refs, err := loadIDUUIDMapForIDs(ctx, refDB, target.refTable, refIDs)
		if err != nil {
			return 0, nil, errors.Wrapf(err, "resolve %s.%s owner uuids", target.table, target.uuidColumn)
		}
		for _, row := range rows {
			owner := refs[row.RefID]
			if owner == "" || owner == row.UUID {
				continue
			}
			mismatched++
			if len(examples) < uuidValidationExampleLimit {
				examples = append(examples, row.Id)
			}
		}
	}
	return mismatched, examples, nil
}

// validateTokenNameUUIDColumn verifies logs.token_uuid against unambiguous token names.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - targetDB: authoritative handle for the logs table.
//   - refDB: authoritative handle for the tokens table.
//   - target: registry FK metadata.
//
// Return values:
//   - []uuidValidationIssue: blocking findings for this column.
//   - error: wrapped database error when a read fails.
func validateTokenNameUUIDColumn(ctx context.Context, targetDB *gorm.DB, refDB *gorm.DB, target uuidFKTarget) ([]uuidValidationIssue, error) {
	idColumn := quoteIdentifier(targetDB, "id")
	userColumn := quoteIdentifier(targetDB, "user_id")
	nameColumn := quoteIdentifier(targetDB, "token_name")
	uuidColumn := quoteIdentifier(targetDB, target.uuidColumn)

	fillable := 0
	mismatched := 0
	ambiguous := 0
	fillableExamples := []int{}
	mismatchExamples := []int{}

	for _, pass := range []struct {
		predicate string
		populated bool
	}{
		{predicate: uuidColumn + " IS NULL"},
		{predicate: uuidColumn + " = ''"},
		{predicate: uuidColumn + " IS NOT NULL AND " + uuidColumn + " != ''", populated: true},
	} {
		lastID := 0
		for {
			rows := []struct {
				Id        int    `gorm:"column:id"`
				UserID    int    `gorm:"column:user_id"`
				TokenName string `gorm:"column:token_name"`
				UUID      string `gorm:"column:fk_uuid"`
			}{}
			err := targetDB.WithContext(ctx).
				Table(target.table).
				Select(idColumn+", "+userColumn+", "+nameColumn+", "+uuidColumn+" AS fk_uuid").
				Where(idColumn+" > ? AND "+userColumn+" > 0 AND "+nameColumn+" != '' AND ("+pass.predicate+")", lastID).
				Order(idColumn + " ASC").
				Limit(uuidBackfillBatchSize).
				Find(&rows).Error
			if err != nil {
				return nil, errors.Wrapf(err, "scan %s.%s candidates", target.table, target.uuidColumn)
			}
			if len(rows) == 0 {
				break
			}
			lastID = rows[len(rows)-1].Id

			logRows := make([]uuidLogTokenRow, 0, len(rows))
			for _, row := range rows {
				logRows = append(logRows, uuidLogTokenRow{Id: row.Id, UserID: row.UserID, TokenName: row.TokenName})
			}
			refs, batchAmbiguous, err := resolveTokenUUIDsForLogRows(ctx, refDB, logRows)
			if err != nil {
				return nil, errors.Wrapf(err, "resolve %s.%s validation references", target.table, target.uuidColumn)
			}
			ambiguous += batchAmbiguous

			for _, row := range rows {
				resolved := refs[userTokenNameKey(row.UserID, row.TokenName)]
				if resolved == "" {
					continue
				}
				if !pass.populated {
					fillable++
					if len(fillableExamples) < uuidValidationExampleLimit {
						fillableExamples = append(fillableExamples, row.Id)
					}
					continue
				}
				if resolved != row.UUID {
					mismatched++
					if len(mismatchExamples) < uuidValidationExampleLimit {
						mismatchExamples = append(mismatchExamples, row.Id)
					}
				}
			}
		}
	}

	if ambiguous > 0 {
		uuidMigrationLogger(ctx).Info("external uuid validation tolerated ambiguous token names",
			zap.String("table", target.table),
			zap.String("column", target.uuidColumn),
			zap.Int("ambiguous_keys", ambiguous))
	}

	issues := []uuidValidationIssue{}
	if fillable > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: target.uuidColumn, kind: "fillable missing token uuid",
			count: fillable, examples: fillableExamples,
		})
	}
	if mismatched > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: target.uuidColumn, kind: "populated token uuid disagrees with live token",
			count: mismatched, examples: mismatchExamples,
		})
	}
	return issues, nil
}
