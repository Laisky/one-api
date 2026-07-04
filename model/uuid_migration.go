package model

import (
	"context"
	"fmt"
	"sort"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/random"
)

const uuidBackfillBatchSize = 1000

type uuidBackfillTarget struct {
	table string
	model any
}

type uuidRefTarget struct {
	table      string
	model      any
	fkColumn   string
	uuidColumn string
	refs       map[int]string
}

type uuidIntRow struct {
	Id int `gorm:"column:id"`
}

type uuidRefRow struct {
	Id    int `gorm:"column:id"`
	RefID int `gorm:"column:ref_id"`
}

type uuidNullableRefRow struct {
	Id    int  `gorm:"column:id"`
	RefID *int `gorm:"column:ref_id"`
}

type uuidLogTokenRow struct {
	Id        int    `gorm:"column:id"`
	UserID    int    `gorm:"column:user_id"`
	TokenName string `gorm:"column:token_name"`
}

type uuidTokenNameRow struct {
	UserID int    `gorm:"column:user_id"`
	Name   string `gorm:"column:name"`
	UUID   string `gorm:"column:uuid"`
}

// MigrateExternalUUIDs backfills UUID columns and denormalized FK UUID columns on the primary database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//
// Return values:
//   - error: wrapped migration error when any step fails.
func MigrateExternalUUIDs(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Logger.Info("starting external uuid backfill")
	targets := primaryUUIDBackfillTargets()
	for _, target := range targets {
		if err := backfillOwnUUIDs(ctx, DB, target); err != nil {
			return errors.Wrapf(err, "backfill own uuid for %s", target.table)
		}
	}

	if err := backfillPrimaryFKUUIDs(ctx); err != nil {
		return errors.Wrap(err, "backfill primary fk uuids")
	}
	if err := backfillLogTokenUUIDs(ctx, DB); err != nil {
		return errors.Wrap(err, "backfill log token uuids")
	}
	if err := ensureUUIDUniqueIndexes(ctx, DB, targets); err != nil {
		return errors.Wrap(err, "ensure unique uuid indexes")
	}
	logger.Logger.Info("external uuid backfill completed")
	return nil
}

// MigrateLogExternalUUIDs backfills UUID and FK UUID columns on the configured log database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - logDB: database handle containing the logs table.
//
// Return values:
//   - error: wrapped migration error when any step fails.
func MigrateLogExternalUUIDs(ctx context.Context, logDB *gorm.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if logDB == nil {
		return errors.New("log database is nil")
	}

	if err := backfillOwnUUIDs(ctx, logDB, uuidBackfillTarget{table: "logs", model: &Log{}}); err != nil {
		return errors.Wrap(err, "backfill log own uuids")
	}
	userUUIDs, err := loadIDUUIDMap(ctx, DB, "users")
	if err != nil {
		return errors.Wrap(err, "load user uuid map for log db")
	}
	channelUUIDs, err := loadIDUUIDMap(ctx, DB, "channels")
	if err != nil {
		return errors.Wrap(err, "load channel uuid map for log db")
	}
	if err := backfillFKUUIDs(ctx, logDB, uuidRefTarget{
		table:      "logs",
		model:      &Log{},
		fkColumn:   "user_id",
		uuidColumn: "user_uuid",
		refs:       userUUIDs,
	}); err != nil {
		return errors.Wrap(err, "backfill log user uuids")
	}
	if err := backfillFKUUIDs(ctx, logDB, uuidRefTarget{
		table:      "logs",
		model:      &Log{},
		fkColumn:   "channel_id",
		uuidColumn: "channel_uuid",
		refs:       channelUUIDs,
	}); err != nil {
		return errors.Wrap(err, "backfill log channel uuids")
	}
	if err := backfillLogTokenUUIDs(ctx, logDB); err != nil {
		return errors.Wrap(err, "backfill log token uuids")
	}
	if logDB != DB {
		if err := backfillTokenTransactionLogUUIDs(ctx, logDB); err != nil {
			return errors.Wrap(err, "backfill token transaction log uuids from split log db")
		}
	}
	if err := ensureUUIDUniqueIndexes(ctx, logDB, []uuidBackfillTarget{{table: "logs", model: &Log{}}}); err != nil {
		return errors.Wrap(err, "ensure log unique uuid indexes")
	}
	return nil
}

// backfillPrimaryFKUUIDs fills denormalized FK UUID columns that live on the primary database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//
// Return values:
//   - error: wrapped migration error when any FK backfill fails.
func backfillPrimaryFKUUIDs(ctx context.Context) error {
	userUUIDs, err := loadIDUUIDMap(ctx, DB, "users")
	if err != nil {
		return errors.Wrap(err, "load user uuid map")
	}
	channelUUIDs, err := loadIDUUIDMap(ctx, DB, "channels")
	if err != nil {
		return errors.Wrap(err, "load channel uuid map")
	}
	tokenUUIDs, err := loadIDUUIDMap(ctx, DB, "tokens")
	if err != nil {
		return errors.Wrap(err, "load token uuid map")
	}
	serverUUIDs, err := loadIDUUIDMap(ctx, DB, "mcp_servers")
	if err != nil {
		return errors.Wrap(err, "load mcp server uuid map")
	}

	targets := []uuidRefTarget{
		{table: "users", model: &User{}, fkColumn: "inviter_id", uuidColumn: "inviter_uuid", refs: userUUIDs},
		{table: "tokens", model: &Token{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "redemptions", model: &Redemption{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "logs", model: &Log{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "logs", model: &Log{}, fkColumn: "channel_id", uuidColumn: "channel_uuid", refs: channelUUIDs},
		{table: "token_transactions", model: &TokenTransaction{}, fkColumn: "token_id", uuidColumn: "token_uuid", refs: tokenUUIDs},
		{table: "token_transactions", model: &TokenTransaction{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "user_request_costs", model: &UserRequestCost{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "token_id", uuidColumn: "token_uuid", refs: tokenUUIDs},
		{table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "channel_id", uuidColumn: "channel_uuid", refs: channelUUIDs},
		{table: "mcp_tools", model: &MCPTool{}, fkColumn: "server_id", uuidColumn: "server_uuid", refs: serverUUIDs},
		{table: "passkey_credentials", model: &PasskeyCredential{}, fkColumn: "user_id", uuidColumn: "user_uuid", refs: userUUIDs},
	}
	for _, target := range targets {
		if err := backfillFKUUIDs(ctx, DB, target); err != nil {
			return errors.Wrapf(err, "backfill %s.%s", target.table, target.uuidColumn)
		}
	}
	needsLogUUIDs, err := hasMissingStringColumn(ctx, DB, "token_transactions", "log_uuid")
	if err != nil {
		return errors.Wrap(err, "check token transaction log uuid gaps")
	}
	if needsLogUUIDs {
		logUUIDs, err := loadIDUUIDMap(ctx, DB, "logs")
		if err != nil {
			return errors.Wrap(err, "load log uuid map")
		}
		if err := backfillNullableFKUUIDs(ctx, DB, uuidRefTarget{
			table:      "token_transactions",
			model:      &TokenTransaction{},
			fkColumn:   "log_id",
			uuidColumn: "log_uuid",
			refs:       logUUIDs,
		}); err != nil {
			return errors.Wrap(err, "backfill token_transactions.log_uuid")
		}
	}
	return nil
}

// backfillTokenTransactionLogUUIDs fills token transaction log UUIDs from the provided log database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - logDB: database handle containing authoritative log rows.
//
// Return values:
//   - error: wrapped migration error when the log map or update fails.
func backfillTokenTransactionLogUUIDs(ctx context.Context, logDB *gorm.DB) error {
	logUUIDs, err := loadIDUUIDMap(ctx, logDB, "logs")
	if err != nil {
		return errors.Wrap(err, "load split log uuid map")
	}
	if err := backfillNullableFKUUIDs(ctx, DB, uuidRefTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		refs:       logUUIDs,
	}); err != nil {
		return errors.Wrap(err, "backfill token_transactions.log_uuid")
	}
	return nil
}

// backfillLogTokenUUIDs fills log token UUIDs where token names are unique for the user.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - logDB: database handle containing the logs table.
//
// Return values:
//   - error: wrapped migration error when token map loading or log updates fail.
func backfillLogTokenUUIDs(ctx context.Context, logDB *gorm.DB) error {
	if !logDB.Migrator().HasColumn(&Log{}, "token_uuid") {
		return nil
	}
	hasMissing, err := hasMissingStringColumn(ctx, logDB, "logs", "token_uuid")
	if err != nil {
		return errors.Wrap(err, "check missing log token uuids")
	}
	if !hasMissing {
		return nil
	}
	refs, err := loadUserTokenNameUUIDMap(ctx)
	if err != nil {
		return errors.Wrap(err, "load user token name uuid map")
	}
	if len(refs) == 0 {
		return nil
	}
	lastID := 0
	for {
		rows := []uuidLogTokenRow{}
		err := logDB.WithContext(ctx).
			Table("logs").
			Select("id, user_id, token_name").
			Where("id > ? AND user_id > 0 AND token_name != ''", lastID).
			Order("id ASC").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return errors.Wrap(err, "list missing log token uuid rows")
		}
		if len(rows) == 0 {
			return nil
		}
		values := make(map[int]string, len(rows))
		for _, row := range rows {
			uuid := refs[userTokenNameKey(row.UserID, row.TokenName)]
			if uuid == "" {
				continue
			}
			values[row.Id] = uuid
		}
		lastID = rows[len(rows)-1].Id
		updated, err := applyStringColumnRows(ctx, logDB, "logs", "token_uuid", values)
		if err != nil {
			return errors.Wrap(err, "set logs.token_uuid")
		}
		logger.Logger.Debug("backfilled log token uuids",
			zap.Int("count", updated),
			zap.Int("skipped", len(rows)-updated))
	}
}

// loadUserTokenNameUUIDMap returns unique user/token-name to token UUID mappings.
// Parameters:
//   - ctx: context controlling the database read.
//
// Return values:
//   - map[string]string: internal user id plus token name to external token UUID.
//   - error: wrapped database error when the read fails.
func loadUserTokenNameUUIDMap(ctx context.Context) (map[string]string, error) {
	rows := []uuidTokenNameRow{}
	err := DB.WithContext(ctx).
		Table("tokens").
		Select("user_id, name, MAX(uuid) AS uuid").
		Where("uuid IS NOT NULL AND uuid != '' AND name != ''").
		Group("user_id, name").
		Having("COUNT(*) = 1").
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "load token uuid map by user and token name")
	}
	refs := make(map[string]string, len(rows))
	for _, row := range rows {
		refs[userTokenNameKey(row.UserID, row.Name)] = row.UUID
	}
	return refs, nil
}

// userTokenNameKey builds a stable map key for user-scoped token names.
// Parameters:
//   - userID: internal user id.
//   - tokenName: token display name.
//
// Return values:
//   - string: composite map key.
func userTokenNameKey(userID int, tokenName string) string {
	return fmt.Sprintf("%d\x00%s", userID, tokenName)
}

// backfillOwnUUIDs fills missing own UUID values for a single table.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - db: database handle containing the target table.
//   - target: table and model metadata.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillOwnUUIDs(ctx context.Context, db *gorm.DB, target uuidBackfillTarget) error {
	if !db.Migrator().HasColumn(target.model, "uuid") {
		return nil
	}

	for {
		rows := []uuidIntRow{}
		err := db.WithContext(ctx).
			Table(target.table).
			Select("id").
			Where("uuid IS NULL OR uuid = ''").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return errors.Wrapf(err, "list missing uuid rows for %s", target.table)
		}
		if len(rows) == 0 {
			return nil
		}

		values := make(map[int]string, len(rows))
		for _, row := range rows {
			values[row.Id] = random.GetUUIDWithHyphens()
		}
		updated, err := applyStringColumnRows(ctx, db, target.table, "uuid", values)
		if err != nil {
			return errors.Wrapf(err, "set uuid for %s", target.table)
		}
		logger.Logger.Debug("backfilled external uuids",
			zap.String("table", target.table),
			zap.Int("count", updated))
	}
}

// backfillFKUUIDs fills missing denormalized FK UUID values for non-null integer FK columns.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - db: database handle containing the target table.
//   - target: FK and UUID column metadata plus a referenced id-to-uuid map.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillFKUUIDs(ctx context.Context, db *gorm.DB, target uuidRefTarget) error {
	if !db.Migrator().HasColumn(target.model, target.uuidColumn) {
		return nil
	}
	hasMissing, err := hasMissingStringColumn(ctx, db, target.table, target.uuidColumn)
	if err != nil {
		return errors.Wrapf(err, "check missing fk uuid rows for %s.%s", target.table, target.uuidColumn)
	}
	if !hasMissing {
		return nil
	}
	lastID := 0
	for {
		rows := []uuidRefRow{}
		err := db.WithContext(ctx).
			Table(target.table).
			Select("id, "+target.fkColumn+" AS ref_id").
			Where("id > ? AND "+target.fkColumn+" > 0", lastID).
			Order("id ASC").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return errors.Wrapf(err, "list missing fk uuid rows for %s.%s", target.table, target.uuidColumn)
		}
		if len(rows) == 0 {
			return nil
		}
		lastID = rows[len(rows)-1].Id
		updated, err := applyFKUUIDRows(ctx, db, target, rows)
		if err != nil {
			return err
		}
		_ = updated
	}
}

// backfillNullableFKUUIDs fills missing denormalized FK UUID values for nullable integer FK columns.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - db: database handle containing the target table.
//   - target: FK and UUID column metadata plus a referenced id-to-uuid map.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillNullableFKUUIDs(ctx context.Context, db *gorm.DB, target uuidRefTarget) error {
	if !db.Migrator().HasColumn(target.model, target.uuidColumn) {
		return nil
	}
	hasMissing, err := hasMissingStringColumn(ctx, db, target.table, target.uuidColumn)
	if err != nil {
		return errors.Wrapf(err, "check missing nullable fk uuid rows for %s.%s", target.table, target.uuidColumn)
	}
	if !hasMissing {
		return nil
	}
	lastID := 0
	for {
		rows := []uuidNullableRefRow{}
		err := db.WithContext(ctx).
			Table(target.table).
			Select("id, "+target.fkColumn+" AS ref_id").
			Where("id > ? AND "+target.fkColumn+" IS NOT NULL", lastID).
			Order("id ASC").
			Limit(uuidBackfillBatchSize).
			Find(&rows).Error
		if err != nil {
			return errors.Wrapf(err, "list missing nullable fk uuid rows for %s.%s", target.table, target.uuidColumn)
		}
		if len(rows) == 0 {
			return nil
		}
		lastID = rows[len(rows)-1].Id

		converted := make([]uuidRefRow, 0, len(rows))
		for _, row := range rows {
			if row.RefID == nil {
				continue
			}
			converted = append(converted, uuidRefRow{Id: row.Id, RefID: *row.RefID})
		}
		if len(converted) == 0 {
			return nil
		}
		updated, err := applyFKUUIDRows(ctx, db, target, converted)
		if err != nil {
			return err
		}
		_ = updated
	}
}

// applyFKUUIDRows writes FK UUID values for rows whose referenced row still exists.
// Parameters:
//   - ctx: context controlling migration writes.
//   - db: database handle containing the target table.
//   - target: FK and UUID column metadata plus a referenced id-to-uuid map.
//   - rows: rows requiring FK UUID backfill.
//
// Return values:
//   - int: number of rows updated.
//   - error: wrapped database error when a write fails.
func applyFKUUIDRows(ctx context.Context, db *gorm.DB, target uuidRefTarget, rows []uuidRefRow) (int, error) {
	values := make(map[int]string, len(rows))
	for _, row := range rows {
		uuid, ok := target.refs[row.RefID]
		if !ok || uuid == "" {
			continue
		}
		values[row.Id] = uuid
	}
	updated, err := applyStringColumnRows(ctx, db, target.table, target.uuidColumn, values)
	if err != nil {
		return updated, errors.Wrapf(err, "set %s.%s", target.table, target.uuidColumn)
	}
	logger.Logger.Debug("backfilled external fk uuids",
		zap.String("table", target.table),
		zap.String("column", target.uuidColumn),
		zap.Int("count", updated),
		zap.Int("skipped", len(rows)-updated))
	return updated, nil
}

// applyStringColumnRows updates one string column for many row ids in one bounded statement.
// Parameters:
//   - ctx: context controlling the database write.
//   - db: database handle containing the target table.
//   - table: trusted target table name.
//   - column: trusted target string column name.
//   - values: row id to string value mappings to write when the column is missing.
//
// Return values:
//   - int: number of rows affected by the batch update.
//   - error: wrapped database error when the batch write fails.
func applyStringColumnRows(ctx context.Context, db *gorm.DB, table string, column string, values map[int]string) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}

	ids := make([]int, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	caseSQL := "CASE " + quoteIdentifier(db, "id")
	args := make([]any, 0, len(ids)*2)
	for _, id := range ids {
		caseSQL += " WHEN ? THEN ?"
		args = append(args, id, values[id])
	}
	caseSQL += " ELSE " + quoteIdentifier(db, column) + " END"

	var affected int64
	err := runWithSQLiteBusyRetry(ctx, func() error {
		result := db.WithContext(ctx).
			Table(table).
			Where(quoteIdentifier(db, "id")+" IN ? AND ("+quoteIdentifier(db, column)+" IS NULL OR "+quoteIdentifier(db, column)+" = '')", ids).
			Update(column, gorm.Expr(caseSQL, args...))
		affected = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return int(affected), errors.Wrapf(err, "batch update %s.%s", table, column)
	}
	return int(affected), nil
}

// hasMissingStringColumn reports whether a string column still contains NULL or empty values.
// Parameters:
//   - ctx: context controlling the database read.
//   - db: database handle containing the target table.
//   - table: trusted table name to inspect.
//   - column: trusted string column name to inspect.
//
// Return values:
//   - bool: true when at least one row still needs backfill.
//   - error: wrapped database error when the check fails.
func hasMissingStringColumn(ctx context.Context, db *gorm.DB, table string, column string) (bool, error) {
	var marker int
	sql := "SELECT 1 FROM " + quoteIdentifier(db, table) +
		" WHERE " + quoteIdentifier(db, column) + " IS NULL OR " + quoteIdentifier(db, column) + " = '' LIMIT 1"
	err := db.WithContext(ctx).Raw(sql).Scan(&marker).Error
	if err != nil {
		return false, errors.Wrapf(err, "check missing values for %s.%s", table, column)
	}
	return marker == 1, nil
}

// loadIDUUIDMap returns live row id-to-uuid mappings for a table.
// Parameters:
//   - ctx: context controlling the database read.
//   - db: database handle containing the target table.
//   - table: table name with id and uuid columns.
//
// Return values:
//   - map[int]string: internal id to external uuid.
//   - error: wrapped database error when the read fails.
func loadIDUUIDMap(ctx context.Context, db *gorm.DB, table string) (map[int]string, error) {
	rows := []struct {
		Id   int    `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
	}{}
	err := db.WithContext(ctx).
		Table(table).
		Select("id, uuid").
		Where("uuid IS NOT NULL AND uuid != ''").
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrapf(err, "load uuid map for %s", table)
	}

	refs := make(map[int]string, len(rows))
	for _, row := range rows {
		refs[row.Id] = row.UUID
	}
	return refs, nil
}
