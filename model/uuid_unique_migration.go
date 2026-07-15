package model

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
)

// tokenNameLookupIndexName is the composite index backing bounded token-name resolution.
const tokenNameLookupIndexName = "idx_tokens_user_id_name"

// ensureUUIDCandidateIndexes creates or verifies every non-unique index catch-up depends on.
// It runs as an explicit schema phase before reconciliation so each NULL and empty-string
// pass is served by an index instead of degrading into an unindexed historical scan. Every
// UUID candidate column needs a usable index, and token resolution needs an index beginning
// with (user_id, name). Owned UUID candidate indexes are skipped once the unique index
// exists, because the unique index serves the same lookups. In split mode each authoritative
// database owns its own indexes, so the log database is the only owner of log UUID DDL.
//
// Creation uses the dialect's online policy and runs in the background worker, never on the
// readiness-critical path. A failure stops the cycle and leaves catch-up incomplete; there is
// deliberately no fallback to an unindexed scan.
// Parameters:
//   - ctx: context controlling metadata reads and index creation.
//   - run: coordinator state supplying the topology.
//
// Return values:
//   - error: wrapped database error when an index cannot be created.
func ensureUUIDCandidateIndexes(ctx context.Context, run *uuidMigrationRun) error {
	for _, role := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
		db := run.topology.handle(role)
		for _, target := range ownedTargetsForRole(role) {
			if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, "uuid") {
				continue
			}
			// Look the index up by table name, not by model. GORM's model-based HasIndex
			// calls Schema.LookIndex, which lazily populates the shared schema's parsed
			// indexes without a lock, so two concurrent catch-up workers racing on the same
			// model would race inside GORM itself. The table-name form skips that path.
			if hasIndexNamed(ctx, db, target.table, uuidUniqueIndexName(target.table)) {
				continue
			}
			if err := ensureNonUniqueIndex(ctx, db, target.table, ordinaryUUIDIndexName(target.table), []string{"uuid"}); err != nil {
				return errors.Wrapf(err, "ensure %s uuid candidate index", target.table)
			}
		}
		for _, refRole := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
			for _, target := range fkTargetsForRoles(role, refRole) {
				if err := ensureFKUUIDIndex(ctx, db, target); err != nil {
					return err
				}
			}
		}
	}

	tokenDB := run.topology.handle(uuidRolePrimary)
	if tokenDB.Migrator().HasTable(&Token{}) {
		if err := ensureNonUniqueIndex(ctx, tokenDB, "tokens", tokenNameLookupIndexName, []string{"user_id", "name"}); err != nil {
			return errors.Wrap(err, "ensure token name lookup index")
		}
	}
	return nil
}

// ensureFKUUIDIndex creates one denormalized FK UUID column's non-unique lookup index.
// Denormalized FK UUID indexes always stay non-unique: many rows legitimately share one
// owner's UUID.
// Parameters:
//   - ctx: context controlling metadata reads and index creation.
//   - db: authoritative handle for the target table.
//   - target: registry FK metadata.
//
// Return values:
//   - error: wrapped database error when the index cannot be created.
func ensureFKUUIDIndex(ctx context.Context, db *gorm.DB, target uuidFKTarget) error {
	if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, target.uuidColumn) {
		return nil
	}
	name := fkUUIDIndexName(target.table, target.uuidColumn)
	if err := ensureNonUniqueIndex(ctx, db, target.table, name, []string{target.uuidColumn}); err != nil {
		return errors.Wrapf(err, "ensure %s.%s index", target.table, target.uuidColumn)
	}
	return nil
}

// ensureNonUniqueIndex creates one candidate index when it does not already exist.
// A concurrent worker racing on the same index is tolerated only when the database
// classifies the failure as a duplicate object and a metadata reread finds the index.
// Parameters:
//   - ctx: context controlling metadata reads and index creation.
//   - db: database handle containing the target table.
//   - table: trusted target table name.
//   - name: deterministic index name.
//   - columns: trusted indexed column names in order.
//
// Return values:
//   - error: wrapped database error when the index cannot be created or confirmed.
func ensureNonUniqueIndex(ctx context.Context, db *gorm.DB, table string, name string, columns []string) error {
	// Require a USABLE index, not merely a name match: a PostgreSQL index left invalid by a
	// failed concurrent build still answers a name lookup, and accepting it here would skip
	// the rebuild forever while every candidate query degraded into a sequential scan.
	// createUUIDIndex removes such an index before rebuilding.
	usable, err := hasUsableIndexNamed(ctx, db, table, name)
	if err != nil {
		return err
	}
	if usable {
		return nil
	}
	if err := createUUIDIndex(ctx, db, table, name, columns, uuidIndexCandidate); err != nil {
		if !isDuplicateObjectError(err) {
			return errors.Wrapf(err, "create index %s", name)
		}
		usable, rereadErr := hasUsableIndexNamed(ctx, db, table, name)
		if rereadErr != nil {
			return rereadErr
		}
		if !usable {
			return errors.Wrapf(err, "create index %s", name)
		}
		return nil
	}
	uuidMigrationLogger(ctx).Debug("created uuid candidate index",
		zap.String("table", table),
		zap.String("index", name))
	return nil
}

// promoteUUIDUniqueIndexes performs finalizer-only owned UUID unique-index promotion.
// Ordinary startup never performs this blocking historical DDL. For each owned UUID column
// it confirms the data is complete, canonical, and unique, creates the unique index with
// the dialect's online policy, verifies columns, uniqueness, and validity from dialect
// metadata, and only then drops the redundant ordinary index.
// Parameters:
//   - ctx: context controlling metadata reads and index DDL.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - error: wrapped error when data is unfit for promotion or DDL fails.
func promoteUUIDUniqueIndexes(ctx context.Context, topology *databaseTopology) error {
	for _, role := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
		db := topology.handle(role)
		for _, target := range ownedTargetsForRole(role) {
			if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, "uuid") {
				continue
			}
			if err := promoteUUIDUniqueIndex(ctx, db, target); err != nil {
				return errors.Wrapf(err, "promote %s uuid unique index", target.table)
			}
		}
	}
	return nil
}

// promoteUUIDUniqueIndex promotes one owned UUID column to a verified unique index.
// Parameters:
//   - ctx: context controlling metadata reads and index DDL.
//   - db: authoritative handle for the target table.
//   - target: registry owned UUID metadata.
//
// Return values:
//   - error: wrapped error when the data is unfit or DDL or verification fails.
func promoteUUIDUniqueIndex(ctx context.Context, db *gorm.DB, target uuidOwnedTarget) error {
	uniqueName := uuidUniqueIndexName(target.table)
	if verified, err := verifyUniqueUUIDIndex(ctx, db, target, uniqueName); err != nil {
		return err
	} else if verified {
		return dropRedundantOrdinaryUUIDIndex(ctx, db, target)
	}

	if err := requirePromotableOwnedUUIDs(ctx, db, target); err != nil {
		return err
	}

	if err := createUUIDIndex(ctx, db, target.table, uniqueName, []string{"uuid"}, uuidIndexUnique); err != nil {
		// A duplicate-object race is only success once a metadata reread verifies the
		// exact expected index; the ordinary index stays intact on every other failure.
		if !isDuplicateObjectError(err) {
			return errors.Wrapf(err, "create unique index %s", uniqueName)
		}
		verified, verifyErr := verifyUniqueUUIDIndex(ctx, db, target, uniqueName)
		if verifyErr != nil {
			return verifyErr
		}
		if !verified {
			return errors.Wrapf(err, "create unique index %s", uniqueName)
		}
	}

	verified, err := verifyUniqueUUIDIndex(ctx, db, target, uniqueName)
	if err != nil {
		return err
	}
	if !verified {
		return errors.Errorf("unique index %s failed metadata verification", uniqueName)
	}
	uuidMigrationLogger(ctx).Info("promoted owned uuid unique index",
		zap.String("table", target.table),
		zap.String("index", uniqueName))
	return dropRedundantOrdinaryUUIDIndex(ctx, db, target)
}

// requirePromotableOwnedUUIDs confirms owned UUID data is complete, canonical, and unique.
// Parameters:
//   - ctx: context controlling bounded reads.
//   - db: authoritative handle for the target table.
//   - target: registry owned UUID metadata.
//
// Return values:
//   - error: wrapped error naming the blocking condition.
func requirePromotableOwnedUUIDs(ctx context.Context, db *gorm.DB, target uuidOwnedTarget) error {
	issues, err := validateOwnedUUIDColumn(ctx, db, target)
	if err != nil {
		return errors.Wrapf(err, "inspect %s owned uuids before promotion", target.table)
	}
	if len(issues) == 0 {
		return nil
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.String())
	}
	return errors.Errorf("%s is not promotable: %s", target.table, strings.Join(messages, "; "))
}

// dropRedundantOrdinaryUUIDIndex removes the ordinary UUID index after the unique one is verified.
// Owned UUID model tags never declare an ordinary index, so nothing recreates it on a later
// AutoMigrate.
// Parameters:
//   - ctx: context controlling metadata reads and DDL.
//   - db: authoritative handle for the target table.
//   - target: registry owned UUID metadata.
//
// Return values:
//   - error: wrapped database error when the ordinary index cannot be dropped.
func dropRedundantOrdinaryUUIDIndex(ctx context.Context, db *gorm.DB, target uuidOwnedTarget) error {
	ordinaryName := ordinaryUUIDIndexName(target.table)
	if ordinaryName == uuidUniqueIndexName(target.table) || !hasIndexNamed(ctx, db, target.table, ordinaryName) {
		return nil
	}
	if err := execUUIDDDL(ctx, db, "DROP INDEX "+dropIndexSuffix(db, target.table, ordinaryName)); err != nil {
		return errors.Wrapf(err, "drop ordinary uuid index %s", ordinaryName)
	}
	uuidMigrationLogger(ctx).Debug("dropped redundant ordinary uuid index",
		zap.String("table", target.table),
		zap.String("index", ordinaryName))
	return nil
}

// validateUUIDIndexes verifies every required index exists with the expected shape.
// Owned UUID columns require a valid unique index; denormalized FK UUID columns require a
// non-unique index; token resolution requires an index beginning with (user_id, name).
// Parameters:
//   - ctx: context controlling metadata reads.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - []uuidValidationIssue: blocking index findings.
//   - error: wrapped database error when metadata cannot be read.
func validateUUIDIndexes(ctx context.Context, topology *databaseTopology) ([]uuidValidationIssue, error) {
	issues := []uuidValidationIssue{}
	for _, role := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
		db := topology.handle(role)
		for _, target := range ownedTargetsForRole(role) {
			if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, "uuid") {
				continue
			}
			verified, err := verifyUniqueUUIDIndex(ctx, db, target, uuidUniqueIndexName(target.table))
			if err != nil {
				return nil, err
			}
			if !verified {
				issues = append(issues, uuidValidationIssue{
					table: target.table, column: "uuid", kind: "missing or invalid owned uuid unique index",
				})
			}
		}
		for _, refRole := range []uuidDBRole{uuidRolePrimary, uuidRoleLog} {
			for _, target := range fkTargetsForRoles(role, refRole) {
				if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, target.uuidColumn) {
					continue
				}
				if !hasIndexNamed(ctx, db, target.table, fkUUIDIndexName(target.table, target.uuidColumn)) {
					issues = append(issues, uuidValidationIssue{
						table: target.table, column: target.uuidColumn, kind: "missing denormalized fk uuid index",
					})
				}
			}
		}
	}
	tokenDB := topology.handle(uuidRolePrimary)
	if tokenDB.Migrator().HasTable(&Token{}) && !hasIndexNamed(ctx, tokenDB, "tokens", tokenNameLookupIndexName) {
		issues = append(issues, uuidValidationIssue{
			table: "tokens", column: "user_id,name", kind: "missing token-name lookup index",
		})
	}
	return issues, nil
}

// uuidUniqueIndexName returns the explicit unique UUID index name for a table.
// Parameters:
//   - table: target table name.
//
// Return values:
//   - string: deterministic unique index name.
func uuidUniqueIndexName(table string) string {
	return "idx_" + table + "_uuid_unique"
}

// ordinaryUUIDIndexName returns the non-unique owned UUID candidate index name.
// Parameters:
//   - table: target table name.
//
// Return values:
//   - string: deterministic ordinary UUID index name.
func ordinaryUUIDIndexName(table string) string {
	return "idx_" + table + "_uuid"
}

// fkUUIDIndexName returns the non-unique denormalized FK UUID index name.
// It matches GORM's default naming for the model's index tag so AutoMigrate and this
// migration converge on one index instead of creating two.
// Parameters:
//   - table: target table name.
//   - column: denormalized UUID column name.
//
// Return values:
//   - string: deterministic FK UUID index name.
func fkUUIDIndexName(table string, column string) string {
	return "idx_" + table + "_" + column
}

// quoteIdentifier returns a dialect-specific quoted SQL identifier.
// Parameters:
//   - db: database handle whose dialect controls quoting.
//   - identifier: trusted schema identifier to quote.
//
// Return values:
//   - string: quoted identifier safe for use in migration DDL.
func quoteIdentifier(db *gorm.DB, identifier string) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "mysql" {
		return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
