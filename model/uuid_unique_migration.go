package model

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
)

// primaryUUIDBackfillTargets returns primary database tables that own external UUIDs.
// Parameters: none.
//
// Return values:
//   - []uuidBackfillTarget: table and model metadata for UUID backfill and uniqueness promotion.
func primaryUUIDBackfillTargets() []uuidBackfillTarget {
	return []uuidBackfillTarget{
		{table: "users", model: &User{}},
		{table: "tokens", model: &Token{}},
		{table: "channels", model: &Channel{}},
		{table: "redemptions", model: &Redemption{}},
		{table: "logs", model: &Log{}},
		{table: "token_transactions", model: &TokenTransaction{}},
		{table: "user_request_costs", model: &UserRequestCost{}},
		{table: "traces", model: &Trace{}},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}},
		{table: "mcp_servers", model: &MCPServer{}},
		{table: "mcp_tools", model: &MCPTool{}},
		{table: "passkey_credentials", model: &PasskeyCredential{}},
	}
}

// ensureUUIDUniqueIndexes creates unique indexes for UUID owner columns after backfill is complete.
// Parameters:
//   - ctx: context controlling metadata reads and index creation.
//   - db: database handle containing the target tables.
//   - targets: table and model metadata for UUID owner columns.
//
// Return values:
//   - error: wrapped database error when UUID data is incomplete or an index cannot be created.
func ensureUUIDUniqueIndexes(ctx context.Context, db *gorm.DB, targets []uuidBackfillTarget) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return errors.New("database is nil")
	}

	for _, target := range targets {
		if !db.Migrator().HasTable(target.model) {
			continue
		}
		if !db.Migrator().HasColumn(target.model, "uuid") {
			continue
		}
		if err := ensureUUIDUniqueIndex(ctx, db, target); err != nil {
			return errors.Wrapf(err, "ensure %s uuid unique index", target.table)
		}
	}
	return nil
}

// ensureUUIDUniqueIndex creates one table's UUID unique index after confirming UUIDs are populated.
// Parameters:
//   - ctx: context controlling metadata reads and index creation.
//   - db: database handle containing the target table.
//   - target: table and model metadata for one UUID owner column.
//
// Return values:
//   - error: wrapped database error when UUID data is incomplete or the index cannot be created.
func ensureUUIDUniqueIndex(ctx context.Context, db *gorm.DB, target uuidBackfillTarget) error {
	missing, err := countMissingUUIDs(ctx, db, target.table)
	if err != nil {
		return errors.Wrap(err, "count missing uuids")
	}
	if missing > 0 {
		logger.Logger.Debug("deferred uuid unique index because rows still need backfill",
			zap.String("table", target.table),
			zap.Int64("missing", missing))
		return nil
	}

	indexName := uuidUniqueIndexName(target.table)
	if db.Migrator().HasIndex(target.model, indexName) {
		return nil
	}

	sql := "CREATE UNIQUE INDEX " + quoteIdentifier(db, indexName) +
		" ON " + quoteIdentifier(db, target.table) + " (" + quoteIdentifier(db, "uuid") + ")"
	if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
		return errors.Wrap(err, "create unique uuid index")
	}
	logger.Logger.Debug("created uuid unique index",
		zap.String("table", target.table),
		zap.String("index", indexName))
	return nil
}

// countMissingUUIDs counts rows whose UUID owner column has not been populated.
// Parameters:
//   - ctx: context controlling the database read.
//   - db: database handle containing the target table.
//   - table: target table name.
//
// Return values:
//   - int64: number of rows whose uuid column is NULL or empty.
//   - error: wrapped database error when the count fails.
func countMissingUUIDs(ctx context.Context, db *gorm.DB, table string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Table(table).
		Where("uuid IS NULL OR uuid = ''").
		Count(&count).Error
	if err != nil {
		return 0, errors.Wrapf(err, "count missing uuids for %s", table)
	}
	return count, nil
}

// uuidUniqueIndexName returns the explicit unique UUID index name for a table.
// Parameters:
//   - table: target table name.
//
// Return values:
//   - string: deterministic index name.
func uuidUniqueIndexName(table string) string {
	return "idx_" + table + "_uuid_unique"
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
