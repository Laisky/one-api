package model

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
)

// uuidIndexKind selects the online-DDL policy for one index creation.
type uuidIndexKind string

const (
	// uuidIndexCandidate is a non-unique catch-up index. It is created by the background
	// worker, never on the readiness-critical path.
	uuidIndexCandidate uuidIndexKind = "candidate"
	// uuidIndexUnique is the finalizer-only owned UUID unique index.
	uuidIndexUnique uuidIndexKind = "unique"
)

// execUUIDDDL runs one DDL statement on a pinned session with bounded lock and statement
// timeouts. The lock timeout bounds how long the statement waits to acquire a table lock;
// the statement timeout bounds the whole operation. Exceeding either returns a retryable
// failure instead of blocking indefinitely.
// Parameters:
//   - ctx: context controlling the statement.
//   - db: database handle to execute against.
//   - sql: trusted DDL statement built from registry identifiers.
//
// Return values:
//   - error: raw database error so the caller can classify duplicate-object races.
func execUUIDDDL(ctx context.Context, db *gorm.DB, sql string) error {
	return execUUIDDDLWithTimeout(ctx, db, sql, uuidDDLTimeout())
}

// execUUIDDDLWithTimeout runs one DDL statement under an explicit statement timeout.
//
// The compact migration has its own DDL budget (COMPACT_UUID_DDL_TIMEOUT), separate from the
// external UUID backfill's, so the timeout is a parameter rather than a global read. Sharing
// the body matters more than the extra argument: the session-restore ordering below is subtle
// and must not be reimplemented per migration generation.
// Parameters:
//   - ctx: context controlling the statement.
//   - db: database handle to execute against.
//   - sql: trusted DDL statement built from registry identifiers.
//   - timeout: statement timeout for this DDL.
//
// Return values:
//   - error: raw database error so the caller can classify duplicate-object races.
func execUUIDDDLWithTimeout(ctx context.Context, db *gorm.DB, sql string, timeout time.Duration) error {
	ddlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.WithContext(ddlCtx).Connection(func(tx *gorm.DB) error {
		return withUUIDDDLTimeoutsBounded(tx, uuidLockTimeout(), timeout, func() error {
			return runWithSQLiteBusyRetry(ddlCtx, func() error {
				return tx.Exec(sql).Error
			})
		})
	})
}

// withUUIDDDLTimeouts installs the dialect's lock and statement timeouts on one pinned
// session, runs fn, and always restores the session defaults.
//
// The timeouts and the DDL must share a session, which is why every caller pins a connection
// with Connection rather than relying on the pool. Restoring is not optional: these are
// session-level settings, not transaction-local ones (CREATE INDEX CONCURRENTLY forbids a
// transaction), so a connection returned to the pool would otherwise keep the migration's
// aggressive timeouts and make ordinary application statements abort on lock waits that the
// server default would have tolerated.
// Parameters:
//   - tx: pinned session that will run the DDL.
//   - fn: statement to run under the bounded timeouts.
//
// Return values:
//   - error: raw database error from setting a timeout or from fn.
func withUUIDDDLTimeouts(tx *gorm.DB, fn func() error) error {
	return withUUIDDDLTimeoutsBounded(tx, uuidLockTimeout(), uuidDDLTimeout(), fn)
}

// withUUIDDDLTimeoutsBounded installs explicit lock and statement timeouts on one pinned
// session, runs fn, and always restores the session defaults.
//
// The timeouts are parameters because the external UUID backfill and the compact migration
// carry independent budgets, and a compact DDL statement must not silently inherit the other
// generation's timeout. See withUUIDDDLTimeouts for why restoration is registered before the
// first SET and runs on a cancellation-detached context.
// Parameters:
//   - tx: pinned session that will run the DDL.
//   - lockTimeout: maximum time the statement may wait for a lock.
//   - statementTimeout: maximum duration of the statement itself.
//   - fn: statement to run under the bounded timeouts.
//
// Return values:
//   - error: raw database error from setting a timeout or from fn.
func withUUIDDDLTimeoutsBounded(tx *gorm.DB, lockTimeout time.Duration, statementTimeout time.Duration, fn func() error) error {
	// Restoration is registered BEFORE the first SET and runs on a context detached from
	// cancellation. Both details are load-bearing:
	//
	//   - registering after the SETs would skip restoration when the second SET itself
	//     fails, leaving the first one applied; and
	//   - the usual reason fn() fails is that the DDL deadline fired, which means the
	//     session context is already done — a RESET issued on that context would fail too,
	//     and the driver hands the connection back to the pool without discarding session
	//     state (pgx's stdlib ResetSession is a no-op, and go-sql-driver no longer sends
	//     COM_RESET_CONNECTION), so the migration's aggressive timeouts would silently
	//     poison ordinary application statements on that connection.
	//
	// Detaching keeps the restore on the same pinned connection because Statement.ConnPool
	// survives the session copy.
	restore := []string{}
	defer func() {
		if len(restore) == 0 {
			return
		}
		clean := tx.Session(&gorm.Session{Context: context.WithoutCancel(tx.Statement.Context)})
		for _, statement := range restore {
			if err := clean.Exec(statement).Error; err != nil {
				uuidMigrationLogger(tx.Statement.Context).Warn(
					"failed to restore session timeout after migration DDL; a pooled connection may keep it",
					zap.Error(err))
			}
		}
	}()

	switch dialectName(tx) {
	case "postgres":
		lock := strconv.Itoa(int(lockTimeout / time.Millisecond))
		statement := strconv.Itoa(int(statementTimeout / time.Millisecond))
		restore = append(restore, "RESET lock_timeout")
		if err := tx.Exec("SET lock_timeout = " + lock).Error; err != nil {
			return err
		}
		restore = append(restore, "RESET statement_timeout")
		if err := tx.Exec("SET statement_timeout = " + statement).Error; err != nil {
			return err
		}
	case "mysql":
		// lock_wait_timeout is whole seconds and must be at least 1. MySQL applies
		// max_execution_time to reads only, so the context deadline remains the
		// authoritative statement bound for DDL.
		lock := int(lockTimeout / time.Second)
		if lock < 1 {
			lock = 1
		}
		restore = append(restore, "SET SESSION lock_wait_timeout = DEFAULT")
		if err := tx.Exec("SET SESSION lock_wait_timeout = " + strconv.Itoa(lock)).Error; err != nil {
			return err
		}
	default:
		// SQLite has no server-side lock timeout; its bounded busy retry plus the context
		// deadline provide the same guarantee.
	}
	return fn()
}

// createUUIDIndex creates one index using the dialect's online-DDL policy.
// PostgreSQL builds concurrently outside a transaction so reads and writes continue. MySQL
// requires ALGORITHM=INPLACE with LOCK=NONE and never silently falls back to a blocking
// ALTER without EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL. SQLite has no online DDL at all
// and no maintenance-window concept that maps onto a single-process default deployment, so
// its DDL simply runs under the bounded busy retry and context deadline; the migration must
// complete automatically there without any operator flag.
// Parameters:
//   - ctx: context controlling the statement.
//   - db: authoritative handle for the target table.
//   - table: trusted target table name.
//   - name: deterministic index name.
//   - columns: trusted indexed column names in order.
//   - kind: candidate or unique index policy.
//
// Return values:
//   - error: raw database error so the caller can classify duplicate-object races.
func createUUIDIndex(ctx context.Context, db *gorm.DB, table string, name string, columns []string, kind uuidIndexKind) error {
	unique := ""
	if kind == uuidIndexUnique {
		unique = "UNIQUE "
	}
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(db, column))
	}
	columnList := "(" + strings.Join(quoted, ", ") + ")"

	switch dialectName(db) {
	case "postgres":
		return createUUIDIndexPostgres(ctx, db, table, name, columnList, unique)
	case "mysql":
		return createUUIDIndexMySQL(ctx, db, table, name, columnList, unique, kind)
	default:
		return execUUIDDDL(ctx, db, "CREATE "+unique+"INDEX "+quoteIdentifier(db, name)+
			" ON "+quoteIdentifier(db, table)+" "+columnList)
	}
}

// createUUIDIndexPostgres builds an index concurrently on one pinned connection.
// CREATE INDEX CONCURRENTLY cannot run inside a transaction block, so the statements are
// pinned with Connection rather than wrapped in a transaction. A previous failed concurrent
// build can leave an invalid same-name index behind, which must be removed before retrying.
// Parameters:
//   - ctx: context controlling the statements.
//   - db: PostgreSQL handle for the target table.
//   - table: trusted target table name.
//   - name: deterministic index name.
//   - columnList: parenthesized quoted column list.
//   - unique: "UNIQUE " for a unique index, otherwise empty.
//
// Return values:
//   - error: raw database error from the concurrent build.
func createUUIDIndexPostgres(ctx context.Context, db *gorm.DB, table string, name string, columnList string, unique string) error {
	if err := dropInvalidPostgresIndex(ctx, db, name); err != nil {
		return err
	}
	ddlCtx, cancel := context.WithTimeout(ctx, uuidDDLTimeout())
	defer cancel()
	return db.WithContext(ddlCtx).Connection(func(tx *gorm.DB) error {
		return withUUIDDDLTimeouts(tx, func() error {
			return tx.Exec("CREATE " + unique + "INDEX CONCURRENTLY " + quoteIdentifier(db, name) +
				" ON " + quoteIdentifier(db, table) + " " + columnList).Error
		})
	})
}

// dropInvalidPostgresIndex removes a same-name index left invalid by a failed concurrent build.
// PostgreSQL keeps such an index in the catalog but never uses it for reads, and a retry of
// CREATE INDEX CONCURRENTLY would fail on the duplicate name forever. Only an index that
// metadata proves invalid is dropped; a valid index is left untouched for the caller to verify.
// Parameters:
//   - ctx: context controlling the metadata read and drop.
//   - db: PostgreSQL handle owning the index.
//   - name: index name to inspect.
//
// Return values:
//   - error: wrapped database error when the metadata read or drop fails.
func dropInvalidPostgresIndex(ctx context.Context, db *gorm.DB, name string) error {
	valid, found, err := postgresIndexValidity(ctx, db, name)
	if err != nil {
		return err
	}
	if !found || valid {
		return nil
	}
	if err := execUUIDDDL(ctx, db, "DROP INDEX CONCURRENTLY IF EXISTS "+quoteIdentifier(db, name)); err != nil {
		return errors.Wrapf(err, "drop invalid postgres index %s", name)
	}
	uuidMigrationLogger(ctx).Warn("dropped invalid postgres concurrent index before retrying promotion")
	return nil
}

// createUUIDIndexMySQL adds an index with online DDL on one pinned connection.
// Parameters:
//   - ctx: context controlling the statements.
//   - db: MySQL handle for the target table.
//   - table: trusted target table name.
//   - name: deterministic index name.
//   - columnList: parenthesized quoted column list.
//   - unique: "UNIQUE " for a unique index, otherwise empty.
//   - kind: candidate or unique index policy.
//
// Return values:
//   - error: raw database error, including an explicit maintenance-retry error when the
//     server cannot add the index without a blocking lock.
func createUUIDIndexMySQL(ctx context.Context, db *gorm.DB, table string, name string, columnList string, unique string, kind uuidIndexKind) error {
	ddlCtx, cancel := context.WithTimeout(ctx, uuidDDLTimeout())
	defer cancel()
	return db.WithContext(ddlCtx).Connection(func(tx *gorm.DB) error {
		return withUUIDDDLTimeouts(tx, func() error {
			return createUUIDIndexMySQLStatement(ctx, tx, db, table, name, columnList, unique, kind)
		})
	})
}

// createUUIDIndexMySQLStatement runs the online ALTER on a session whose timeouts are set.
// Parameters:
//   - ctx: context carrying the migration logger.
//   - tx: pinned session with bounded timeouts installed.
//   - db: handle whose dialect controls identifier quoting.
//   - table: trusted target table name.
//   - name: deterministic index name.
//   - columnList: parenthesized quoted column list.
//   - unique: "UNIQUE " for a unique index, otherwise empty.
//   - kind: candidate or unique index policy.
//
// Return values:
//   - error: raw database error, or an explicit maintenance-retry error.
func createUUIDIndexMySQLStatement(ctx context.Context, tx *gorm.DB, db *gorm.DB, table string, name string, columnList string, unique string, kind uuidIndexKind) error {
	indexType := "INDEX"
	if unique != "" {
		indexType = "UNIQUE INDEX"
	}
	online := "ALTER TABLE " + quoteIdentifier(db, table) +
		" ADD " + indexType + " " + quoteIdentifier(db, name) + " " + columnList +
		", ALGORITHM=INPLACE, LOCK=NONE"
	err := tx.Exec(online).Error
	if err == nil || isDuplicateObjectError(err) {
		return err
	}
	if !isMySQLOnlineDDLUnsupported(err) {
		return err
	}
	if !uuidBlockingDDLAllowed() {
		// Falling back silently would stall reads and writes on a large table, so
		// finalization fails and the operator retries in a maintenance window.
		return errors.Wrapf(err,
			"mysql cannot add index %s online (LOCK=NONE unsupported); retry with EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL=true in an approved maintenance window", name)
	}
	uuidMigrationLogger(ctx).Warn("falling back to blocking mysql index DDL because the operator approved it",
		zap.String("index_kind", string(kind)))
	return tx.Exec("ALTER TABLE " + quoteIdentifier(db, table) +
		" ADD " + indexType + " " + quoteIdentifier(db, name) + " " + columnList).Error
}

// isMySQLOnlineDDLUnsupported reports whether MySQL refused the requested online algorithm.
// Parameters:
//   - err: error returned by the online ALTER statement.
//
// Return values:
//   - bool: true when the server cannot satisfy ALGORITHM=INPLACE with LOCK=NONE.
func isMySQLOnlineDDLUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	// MySQL 1845/1846: ALGORITHM=INPLACE or LOCK=NONE is not supported for this operation.
	return strings.Contains(message, "algorithm=inplace is not supported") ||
		strings.Contains(message, "lock=none is not supported") ||
		strings.Contains(message, "is not supported. reason:")
}

// verifyUniqueUUIDIndex confirms an index exists and is a valid unique index on (uuid).
// A matching name alone is never accepted: the indexed columns, uniqueness, and dialect
// validity are all read from metadata.
// Parameters:
//   - ctx: context controlling metadata reads.
//   - db: authoritative handle for the target table.
//   - target: registry owned UUID metadata.
//   - name: deterministic unique index name.
//
// Return values:
//   - bool: true when the index exists with the expected shape.
//   - error: wrapped database error when metadata cannot be read.
func verifyUniqueUUIDIndex(ctx context.Context, db *gorm.DB, target uuidOwnedTarget, name string) (bool, error) {
	indexes, err := db.WithContext(ctx).Migrator().GetIndexes(target.model)
	if err != nil {
		return false, errors.Wrapf(err, "read index metadata for %s", target.table)
	}
	found := false
	for _, index := range indexes {
		if index.Name() != name {
			continue
		}
		columns := index.Columns()
		if len(columns) != 1 || !strings.EqualFold(columns[0], "uuid") {
			return false, nil
		}
		if unique, ok := index.Unique(); !ok || !unique {
			return false, nil
		}
		found = true
		break
	}
	if !found {
		return false, nil
	}
	return isIndexValid(ctx, db, name)
}

// isIndexValid reports whether the dialect considers the index usable.
// PostgreSQL can leave an index INVALID when a concurrent build fails; such an index is
// never accepted as promotion evidence. Other dialects have no invalid index state.
// Parameters:
//   - ctx: context controlling the metadata read.
//   - db: database handle owning the index.
//   - name: index name to inspect.
//
// Return values:
//   - bool: true when the index is valid.
//   - error: wrapped database error when the metadata read fails.
func isIndexValid(ctx context.Context, db *gorm.DB, name string) (bool, error) {
	if dialectName(db) != "postgres" {
		return true, nil
	}
	valid, found, err := postgresIndexValidity(ctx, db, name)
	if err != nil {
		return false, err
	}
	return found && valid, nil
}

// postgresIndexValidity reads whether a PostgreSQL index exists and is valid.
// The lookup is restricted to the current schema: an unqualified relname can match an index
// of the same name in another schema, which would let an unrelated object stand in as
// promotion evidence.
// Parameters:
//   - ctx: context controlling the metadata read.
//   - db: PostgreSQL handle owning the index.
//   - name: index name to inspect.
//
// Return values:
//   - bool: true when the index is valid.
//   - bool: true when an index with that name exists in the current schema.
//   - error: wrapped database error when the metadata read fails.
func postgresIndexValidity(ctx context.Context, db *gorm.DB, name string) (bool, bool, error) {
	rows := []struct {
		Valid bool `gorm:"column:indisvalid"`
	}{}
	sql := "SELECT i.indisvalid FROM pg_index i" +
		" JOIN pg_class c ON c.oid = i.indexrelid" +
		" JOIN pg_namespace n ON n.oid = c.relnamespace" +
		" WHERE c.relname = ? AND n.nspname = CURRENT_SCHEMA()"
	if err := db.WithContext(ctx).Raw(sql, name).Scan(&rows).Error; err != nil {
		return false, false, errors.Wrapf(err, "read postgres index validity for %s", name)
	}
	if len(rows) == 0 {
		return false, false, nil
	}
	return rows[0].Valid, true, nil
}

// hasUsableIndexNamed reports whether an index exists AND the dialect considers it usable.
//
// A name match alone is not enough. PostgreSQL keeps an index left behind by a failed
// concurrent build in the catalog and reports it through pg_indexes — which is what gorm's
// HasIndex queries — but marks it invalid and never uses it for reads. Accepting that name
// would skip the rebuild forever while every candidate query silently degraded into a
// sequential scan.
// Parameters:
//   - ctx: context controlling the metadata reads.
//   - db: database handle containing the table.
//   - table: trusted target table name.
//   - name: index name to look for.
//
// Return values:
//   - bool: true when the index exists and is usable.
//   - error: wrapped database error when a metadata read fails.
func hasUsableIndexNamed(ctx context.Context, db *gorm.DB, table string, name string) (bool, error) {
	if !hasIndexNamed(ctx, db, table, name) {
		return false, nil
	}
	return isIndexValid(ctx, db, name)
}

// hasIndexNamed reports whether an index name exists on the target table.
// Parameters:
//   - ctx: context controlling the metadata read.
//   - db: database handle containing the table.
//   - table: trusted target table name.
//   - name: index name to look for.
//
// Return values:
//   - bool: true when the index exists.
func hasIndexNamed(ctx context.Context, db *gorm.DB, table string, name string) bool {
	return db.WithContext(ctx).Migrator().HasIndex(table, name)
}

// dropIndexSuffix returns the dialect-specific tail of a DROP INDEX statement.
// MySQL requires the owning table; SQLite and PostgreSQL drop by index name alone.
// Parameters:
//   - db: database handle whose dialect controls the syntax.
//   - table: trusted target table name.
//   - name: index name to drop.
//
// Return values:
//   - string: statement tail after the DROP INDEX keyword.
func dropIndexSuffix(db *gorm.DB, table string, name string) string {
	if dialectName(db) == "mysql" {
		return quoteIdentifier(db, name) + " ON " + quoteIdentifier(db, table)
	}
	return quoteIdentifier(db, name)
}

// dialectName returns the lowercase dialect name for a handle.
// Parameters:
//   - db: database handle to inspect.
//
// Return values:
//   - string: dialect name, or an empty string when unavailable.
func dialectName(db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return ""
	}
	return strings.ToLower(db.Dialector.Name())
}
