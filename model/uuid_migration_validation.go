package model

import (
	"context"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// uuidValidationExampleLimit bounds how many example row ids one issue reports.
const uuidValidationExampleLimit = 10

// uuidValidationIssue is one blocking finding. It carries aggregate counts and bounded
// example row ids only; it never carries row content, UUID values, or credentials.
type uuidValidationIssue struct {
	table    string
	column   string
	kind     string
	count    int
	examples []int
}

// String renders one issue for a wrapped terminal error.
// Parameters: none.
//
// Return values:
//   - string: table, column, finding kind, aggregate count, and bounded example ids.
func (issue uuidValidationIssue) String() string {
	examples := make([]string, 0, len(issue.examples))
	for _, id := range issue.examples {
		examples = append(examples, strconv.Itoa(id))
	}
	return issue.table + "." + issue.column + ": " + issue.kind +
		" (count=" + strconv.Itoa(issue.count) + ", example_ids=[" + strings.Join(examples, ",") + "])"
}

// validateExternalUUIDs runs topology-wide validation after the UUID-aware-writer barrier.
// It never repairs data: malformed, duplicate, or mismatched values are reported so an
// operator can correct them, and completion markers stay absent until they do.
// Parameters:
//   - ctx: context controlling every bounded validation read.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - error: wrapped error listing every blocking finding, or nil when the topology passes.
func validateExternalUUIDs(ctx context.Context, topology *databaseTopology) error {
	issues := []uuidValidationIssue{}

	for _, role := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
		db := topology.handle(role)
		for _, target := range ownedTargetsForRole(role) {
			found, err := validateOwnedUUIDColumn(ctx, db, target)
			if err != nil {
				return errors.Wrapf(err, "validate owned uuid for %s", target.table)
			}
			issues = append(issues, found...)
		}
	}

	for _, phase := range uuidFKPhaseOrder() {
		targetDB := topology.handle(phase.role)
		refDB := topology.handle(phase.refRole)
		for _, target := range fkTargetsForRoles(phase.role, phase.refRole) {
			found, err := validateFKUUIDColumn(ctx, targetDB, refDB, target)
			if err != nil {
				return errors.Wrapf(err, "validate %s.%s", target.table, target.uuidColumn)
			}
			issues = append(issues, found...)
		}
	}

	indexIssues, err := validateUUIDIndexes(ctx, topology)
	if err != nil {
		return errors.Wrap(err, "validate uuid indexes")
	}
	issues = append(issues, indexIssues...)

	if len(issues) == 0 {
		uuidMigrationLogger(ctx).Info("external uuid global validation passed",
			zap.String("topology", string(topology.mode)))
		return nil
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.String())
	}
	return errors.Errorf("external uuid validation found %d blocking issue(s): %s",
		len(issues), strings.Join(messages, "; "))
}

// validateOwnedUUIDColumn verifies one owned UUID column is complete, canonical, and unique.
// Missing owned UUIDs are always fillable, so any remaining missing value blocks completion.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - db: authoritative handle for the target table.
//   - target: registry owned UUID metadata.
//
// Return values:
//   - []uuidValidationIssue: blocking findings for this column.
//   - error: wrapped database error when a read fails.
func validateOwnedUUIDColumn(ctx context.Context, db *gorm.DB, target uuidOwnedTarget) ([]uuidValidationIssue, error) {
	if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, "uuid") {
		return nil, nil
	}
	issues := []uuidValidationIssue{}

	missing, examples, err := countMissingStringColumn(ctx, db, target.table, "uuid")
	if err != nil {
		return nil, err
	}
	if missing > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: "uuid", kind: "missing owned uuid",
			count: missing, examples: examples,
		})
	}

	malformed, malformedExamples, err := findMalformedOwnedUUIDs(ctx, db, target.table)
	if err != nil {
		return nil, err
	}
	if malformed > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: "uuid", kind: "malformed owned uuid requires operator remediation",
			count: malformed, examples: malformedExamples,
		})
	}

	duplicates, err := countDuplicateOwnedUUIDs(ctx, db, target.table)
	if err != nil {
		return nil, err
	}
	if duplicates > 0 {
		issues = append(issues, uuidValidationIssue{
			table: target.table, column: "uuid", kind: "duplicate owned uuid requires operator remediation",
			count: duplicates,
		})
	}
	return issues, nil
}

// findMalformedOwnedUUIDs scans populated owned UUIDs in bounded batches and parses each one.
// The UUID parser is the authority: a value that is non-empty but not a canonical hyphenated
// UUID is preserved by catch-up and blocks finalization.
// Parameters:
//   - ctx: context controlling bounded validation reads.
//   - db: authoritative handle for the target table.
//   - table: trusted target table name.
//
// Return values:
//   - int: number of malformed populated owned UUID values.
//   - []int: bounded example row ids.
//   - error: wrapped database error when a read fails.
func findMalformedOwnedUUIDs(ctx context.Context, db *gorm.DB, table string) (int, []int, error) {
	idColumn := quoteIdentifier(db, "id")
	uuidColumn := quoteIdentifier(db, "uuid")

	malformed := 0
	examples := []int{}
	lastID := 0
	for {
		rows := []struct {
			Id   int    `gorm:"column:id"`
			UUID string `gorm:"column:uuid"`
		}{}
		err := db.WithContext(ctx).
			Table(table).
			Select(idColumn+", "+uuidColumn).
			Where(idColumn+" > ? AND "+uuidColumn+" IS NOT NULL AND "+uuidColumn+" != ''", lastID).
			Order(idColumn + " ASC").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return 0, nil, errors.Wrapf(err, "scan owned uuid values for %s", table)
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].Id
		for _, row := range rows {
			if isCanonicalHyphenatedUUID(row.UUID) {
				continue
			}
			malformed++
			if len(examples) < uuidValidationExampleLimit {
				examples = append(examples, row.Id)
			}
		}
	}
	return malformed, examples, nil
}

// isCanonicalHyphenatedUUID reports whether value is a canonical hyphenated UUIDv7 that
// round-trips through the UUID parser.
//
// uuid.Parse also accepts urn, braced, and unhyphenated forms, so the canonical length is
// checked first and the parsed value must render back to the same string. Version 7 is
// required because these identifiers are contractually time-ordered UUIDv7 values; a legacy
// v4 value is well-formed but is not the identifier this project promises, so it blocks
// finalization for explicit operator remediation rather than being silently accepted.
// Parameters:
//   - value: populated owned UUID value.
//
// Return values:
//   - bool: true when the value is a canonical hyphenated UUIDv7.
func isCanonicalHyphenatedUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Version() != 7 {
		return false
	}
	return parsed.String() == strings.ToLower(value)
}

// countDuplicateOwnedUUIDs counts populated owned UUID values shared by more than one row.
// Parameters:
//   - ctx: context controlling the validation read.
//   - db: authoritative handle for the target table.
//   - table: trusted target table name.
//
// Return values:
//   - int: number of duplicated owned UUID values.
//   - error: wrapped database error when the read fails.
func countDuplicateOwnedUUIDs(ctx context.Context, db *gorm.DB, table string) (int, error) {
	uuidColumn := quoteIdentifier(db, "uuid")
	rows := []struct {
		Total int `gorm:"column:total"`
	}{}
	sql := "SELECT COUNT(*) AS " + quoteIdentifier(db, "total") +
		" FROM " + quoteIdentifier(db, table) +
		" WHERE " + uuidColumn + " IS NOT NULL AND " + uuidColumn + " != ''" +
		" GROUP BY " + uuidColumn +
		" HAVING COUNT(*) > 1"
	if err := db.WithContext(ctx).Raw(sql).Scan(&rows).Error; err != nil {
		return 0, errors.Wrapf(err, "count duplicate owned uuids for %s", table)
	}
	return len(rows), nil
}

// countMissingStringColumn counts NULL and empty values and collects bounded example ids.
// Parameters:
//   - ctx: context controlling the validation reads.
//   - db: database handle containing the target table.
//   - table: trusted target table name.
//   - column: trusted target string column name.
//
// Return values:
//   - int: number of rows whose column is NULL or empty.
//   - []int: bounded example row ids.
//   - error: wrapped database error when a read fails.
func countMissingStringColumn(ctx context.Context, db *gorm.DB, table string, column string) (int, []int, error) {
	total := 0
	examples := []int{}
	for _, missingPredicate := range missingStringPredicates(db, column) {
		var count int64
		if err := db.WithContext(ctx).Table(table).Where(missingPredicate).Count(&count).Error; err != nil {
			return 0, nil, errors.Wrapf(err, "count missing values for %s.%s", table, column)
		}
		total += int(count)
		if count == 0 || len(examples) >= uuidValidationExampleLimit {
			continue
		}
		rows := []uuidIntRow{}
		err := db.WithContext(ctx).
			Table(table).
			Select(quoteIdentifier(db, "id")).
			Where(missingPredicate).
			Order(quoteIdentifier(db, "id") + " ASC").
			Limit(uuidValidationExampleLimit - len(examples)).
			Find(&rows).Error
		if err != nil {
			return 0, nil, errors.Wrapf(err, "list missing example ids for %s.%s", table, column)
		}
		for _, row := range rows {
			examples = append(examples, row.Id)
		}
	}
	return total, examples, nil
}
