package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// primaryUUIDFKProbeTargets returns primary database FK UUID columns that can be checked by joining local tables.
// Parameters: none.
//
// Return values:
//   - []uuidRefProbeTarget: FK UUID target metadata and referenced owner table names.
func primaryUUIDFKProbeTargets() []uuidRefProbeTarget {
	return []uuidRefProbeTarget{
		{table: "users", model: &User{}, fkColumn: "inviter_id", uuidColumn: "inviter_uuid", refTable: "users"},
		{table: "tokens", model: &Token{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "redemptions", model: &Redemption{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "logs", model: &Log{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "logs", model: &Log{}, fkColumn: "channel_id", uuidColumn: "channel_uuid", refTable: "channels"},
		{table: "token_transactions", model: &TokenTransaction{}, fkColumn: "token_id", uuidColumn: "token_uuid", refTable: "tokens"},
		{table: "token_transactions", model: &TokenTransaction{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "user_request_costs", model: &UserRequestCost{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "token_id", uuidColumn: "token_uuid", refTable: "tokens"},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "channel_id", uuidColumn: "channel_uuid", refTable: "channels"},
		{table: "mcp_tools", model: &MCPTool{}, fkColumn: "server_id", uuidColumn: "server_uuid", refTable: "mcp_servers"},
		{table: "passkey_credentials", model: &PasskeyCredential{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
	}
}

// logUUIDFKProbeTargets returns log database FK UUID columns that reference primary database resources.
// Parameters: none.
//
// Return values:
//   - []uuidRefProbeTarget: log FK UUID target metadata and referenced primary table names.
func logUUIDFKProbeTargets() []uuidRefProbeTarget {
	return []uuidRefProbeTarget{
		{table: "logs", model: &Log{}, fkColumn: "user_id", uuidColumn: "user_uuid", refTable: "users"},
		{table: "logs", model: &Log{}, fkColumn: "channel_id", uuidColumn: "channel_uuid", refTable: "channels"},
	}
}

// hasPrimaryExternalUUIDBackfill reports whether the primary database has UUID data that can be backfilled.
// Parameters:
//   - ctx: context controlling fast existence checks.
//
// Return values:
//   - bool: true when at least one primary UUID owner or FK UUID column has fillable missing data.
//   - error: wrapped database error when any check fails.
func hasPrimaryExternalUUIDBackfill(ctx context.Context) (bool, error) {
	for _, target := range primaryUUIDBackfillTargets() {
		if !DB.Migrator().HasTable(target.model) || !DB.Migrator().HasColumn(target.model, "uuid") {
			continue
		}
		hasMissing, err := hasMissingStringColumn(ctx, DB, target.table, "uuid")
		if err != nil {
			return false, errors.Wrapf(err, "check missing uuid rows for %s", target.table)
		}
		if hasMissing {
			return true, nil
		}
	}

	hasFKMissing, err := hasPrimaryFKExternalUUIDBackfill(ctx)
	if err != nil {
		return false, errors.Wrap(err, "check primary fk uuid gaps")
	}
	if hasFKMissing {
		return true, nil
	}
	return hasBackfillableLogTokenUUIDs(ctx, DB)
}

// hasLogExternalUUIDBackfill reports whether the log database has UUID data that can be backfilled.
// Parameters:
//   - ctx: context controlling fast existence checks.
//   - logDB: database handle containing the logs table.
//
// Return values:
//   - bool: true when log UUID owner or FK UUID columns have fillable missing data.
//   - error: wrapped database error when any check fails.
func hasLogExternalUUIDBackfill(ctx context.Context, logDB *gorm.DB) (bool, error) {
	if logDB.Migrator().HasTable(&Log{}) && logDB.Migrator().HasColumn(&Log{}, "uuid") {
		hasMissing, err := hasMissingStringColumn(ctx, logDB, "logs", "uuid")
		if err != nil {
			return false, errors.Wrap(err, "check missing log uuids")
		}
		if hasMissing {
			return true, nil
		}
	}

	for _, target := range logUUIDFKProbeTargets() {
		var (
			hasMissing bool
			err        error
		)
		if logDB == DB {
			hasMissing, err = hasBackfillableFKUUIDs(ctx, logDB, target)
		} else {
			hasMissing, err = hasMissingFKUUIDCandidate(ctx, logDB, target)
		}
		if err != nil {
			return false, errors.Wrapf(err, "check log fk uuid gaps for %s", target.uuidColumn)
		}
		if hasMissing {
			return true, nil
		}
	}

	hasLogTokenMissing, err := hasBackfillableLogTokenUUIDs(ctx, logDB)
	if err != nil {
		return false, errors.Wrap(err, "check log token uuid gaps")
	}
	if hasLogTokenMissing {
		return true, nil
	}
	if logDB == DB {
		return false, nil
	}
	return hasMissingFKUUIDCandidate(ctx, DB, uuidRefProbeTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		nullableFK: true,
	})
}

// hasPrimaryFKExternalUUIDBackfill reports whether any primary FK UUID column has fillable missing values.
// Parameters:
//   - ctx: context controlling fast existence checks.
//
// Return values:
//   - bool: true when any same-database FK UUID value can be backfilled.
//   - error: wrapped database error when a check fails.
func hasPrimaryFKExternalUUIDBackfill(ctx context.Context) (bool, error) {
	for _, target := range primaryUUIDFKProbeTargets() {
		hasMissing, err := hasBackfillableFKUUIDs(ctx, DB, target)
		if err != nil {
			return false, errors.Wrapf(err, "check %s.%s", target.table, target.uuidColumn)
		}
		if hasMissing {
			return true, nil
		}
	}

	hasMissing, err := hasBackfillableFKUUIDs(ctx, DB, uuidRefProbeTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		refTable:   "logs",
		nullableFK: true,
	})
	if err != nil {
		return false, errors.Wrap(err, "check token_transactions.log_uuid")
	}
	return hasMissing, nil
}

// hasBackfillableFKUUIDs reports whether a missing FK UUID can be filled from a referenced row in the same database.
// Parameters:
//   - ctx: context controlling the existence query.
//   - db: database handle containing both the target and referenced tables.
//   - target: FK UUID column metadata and referenced table name.
//
// Return values:
//   - bool: true when at least one matching referenced UUID exists.
//   - error: wrapped database error when the existence query fails.
func hasBackfillableFKUUIDs(ctx context.Context, db *gorm.DB, target uuidRefProbeTarget) (bool, error) {
	if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, target.uuidColumn) {
		return false, nil
	}
	if target.refTable == "" {
		return hasMissingFKUUIDCandidate(ctx, db, target)
	}

	var marker int
	fkPredicate := "t." + quoteIdentifier(db, target.fkColumn) + " > 0"
	sql := "SELECT 1 FROM " + quoteIdentifier(db, target.table) + " AS t" +
		" INNER JOIN " + quoteIdentifier(db, target.refTable) + " AS r ON r." + quoteIdentifier(db, "id") + " = t." + quoteIdentifier(db, target.fkColumn) +
		" WHERE (t." + quoteIdentifier(db, target.uuidColumn) + " IS NULL OR t." + quoteIdentifier(db, target.uuidColumn) + " = '')" +
		" AND " + fkPredicate +
		" AND r." + quoteIdentifier(db, "uuid") + " IS NOT NULL AND r." + quoteIdentifier(db, "uuid") + " != '' LIMIT 1"
	err := db.WithContext(ctx).Raw(sql).Scan(&marker).Error
	if err != nil {
		return false, errors.Wrapf(err, "check backfillable values for %s.%s", target.table, target.uuidColumn)
	}
	return marker == 1, nil
}

// hasMissingFKUUIDCandidate reports whether a target table has local rows that may need FK UUID backfill.
// Parameters:
//   - ctx: context controlling the existence query.
//   - db: database handle containing the target table.
//   - target: FK UUID column metadata.
//
// Return values:
//   - bool: true when a row has a missing FK UUID and a present integer FK.
//   - error: wrapped database error when the existence query fails.
func hasMissingFKUUIDCandidate(ctx context.Context, db *gorm.DB, target uuidRefProbeTarget) (bool, error) {
	if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, target.uuidColumn) {
		return false, nil
	}

	var marker int
	fkPredicate := quoteIdentifier(db, target.fkColumn) + " > 0"
	sql := "SELECT 1 FROM " + quoteIdentifier(db, target.table) +
		" WHERE (" + quoteIdentifier(db, target.uuidColumn) + " IS NULL OR " + quoteIdentifier(db, target.uuidColumn) + " = '')" +
		" AND " + fkPredicate + " LIMIT 1"
	err := db.WithContext(ctx).Raw(sql).Scan(&marker).Error
	if err != nil {
		return false, errors.Wrapf(err, "check missing candidates for %s.%s", target.table, target.uuidColumn)
	}
	return marker == 1, nil
}

// hasBackfillableLogTokenUUIDs reports whether logs have missing token UUIDs that can be filled from token names.
// Parameters:
//   - ctx: context controlling the existence query.
//   - logDB: database handle containing the logs table.
//
// Return values:
//   - bool: true when at least one log token UUID can be backfilled.
//   - error: wrapped database error when the existence query fails.
func hasBackfillableLogTokenUUIDs(ctx context.Context, logDB *gorm.DB) (bool, error) {
	if !logDB.Migrator().HasTable(&Log{}) || !logDB.Migrator().HasColumn(&Log{}, "token_uuid") {
		return false, nil
	}

	var marker int
	if logDB != DB {
		sql := "SELECT 1 FROM " + quoteIdentifier(logDB, "logs") +
			" WHERE (" + quoteIdentifier(logDB, "token_uuid") + " IS NULL OR " + quoteIdentifier(logDB, "token_uuid") + " = '')" +
			" AND " + quoteIdentifier(logDB, "user_id") + " > 0 AND " + quoteIdentifier(logDB, "token_name") + " != '' LIMIT 1"
		err := logDB.WithContext(ctx).Raw(sql).Scan(&marker).Error
		if err != nil {
			return false, errors.Wrap(err, "check split log token uuid candidates")
		}
		return marker == 1, nil
	}

	sql := "SELECT 1 FROM " + quoteIdentifier(logDB, "logs") + " AS l" +
		" INNER JOIN (" +
		"SELECT " + quoteIdentifier(logDB, "user_id") + ", " + quoteIdentifier(logDB, "name") +
		" FROM " + quoteIdentifier(logDB, "tokens") +
		" WHERE " + quoteIdentifier(logDB, "uuid") + " IS NOT NULL AND " + quoteIdentifier(logDB, "uuid") + " != '' AND " + quoteIdentifier(logDB, "name") + " != ''" +
		" GROUP BY " + quoteIdentifier(logDB, "user_id") + ", " + quoteIdentifier(logDB, "name") +
		" HAVING COUNT(*) = 1" +
		") AS t ON t." + quoteIdentifier(logDB, "user_id") + " = l." + quoteIdentifier(logDB, "user_id") +
		" AND t." + quoteIdentifier(logDB, "name") + " = l." + quoteIdentifier(logDB, "token_name") +
		" WHERE (l." + quoteIdentifier(logDB, "token_uuid") + " IS NULL OR l." + quoteIdentifier(logDB, "token_uuid") + " = '')" +
		" AND l." + quoteIdentifier(logDB, "user_id") + " > 0 AND l." + quoteIdentifier(logDB, "token_name") + " != '' LIMIT 1"
	err := logDB.WithContext(ctx).Raw(sql).Scan(&marker).Error
	if err != nil {
		return false, errors.Wrap(err, "check backfillable log token uuids")
	}
	return marker == 1, nil
}
