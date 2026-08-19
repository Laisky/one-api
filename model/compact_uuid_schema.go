package model

// This file generates and verifies the additive compact shadow columns (AUTO-004).
//
// Every statement here is built exclusively from compile-time registry identifiers. Request
// input never reaches DDL, an identifier, a batch size, or an allocation.
//
// The columns are additive and nullable on every dialect, and they are excluded from ordinary
// AutoMigrate: the projection structs carry `-:migration`, so GORM never owns compact DDL and
// a pinned old binary's AutoMigrate leaves the shadows untouched.

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactColumnType returns the dialect's physical type for a compact shadow column.
//
// The types are locked by the proposal: a PostgreSQL native uuid, a MySQL BINARY(16), and a
// plain SQLite BLOB. SQLite deliberately gets no CHECK constraint, because a checked
// ADD COLUMN can scan the whole table; its type and length are enforced by the trigger output
// and by the audit instead.
// Parameters:
//   - dialect: lowercase dialect name from dialectName.
//
// Return values:
//   - string: column type for the dialect.
//   - error: wrapped error when the dialect has no approved compact representation.
func compactColumnType(dialect string) (string, error) {
	switch dialect {
	case "postgres":
		return "uuid", nil
	case "mysql":
		return "BINARY(16)", nil
	case "sqlite":
		return "BLOB", nil
	default:
		return "", errors.Errorf("compact uuid storage has no approved column type for dialect %q", dialect)
	}
}

// compactAddColumnSQL builds the additive ALTER statement for one shadow column.
//
// The column is always nullable and never has a default. A default would rewrite the table on
// some engines and would also give an unbackfilled row a non-NULL shadow, which is exactly the
// state the NULL-backlog probe uses to find work.
// Parameters:
//   - db: handle whose dialect controls the type and quoting.
//   - target: registry target to expand.
//
// Return values:
//   - string: complete ALTER TABLE statement.
//   - error: wrapped error when the dialect is unsupported.
func compactAddColumnSQL(db *gorm.DB, target compactTarget) (string, error) {
	dialect := dialectName(db)
	columnType, err := compactColumnType(dialect)
	if err != nil {
		return "", err
	}

	statement := "ALTER TABLE " + quoteIdentifier(db, target.table) +
		" ADD COLUMN " + quoteIdentifier(db, target.compactColumn) + " " + columnType + " NULL"
	if dialect == "mysql" {
		// ALGORITHM=INSTANT keeps the add metadata-only on MySQL 8. When the server cannot
		// satisfy it, addCompactColumn retries with the verified INPLACE/LOCK=NONE form
		// rather than silently falling back to a blocking COPY.
		statement += ", ALGORITHM=INSTANT"
	}
	return statement, nil
}

// compactAddColumnFallbackSQL builds the MySQL INPLACE/LOCK=NONE form of the additive ALTER.
// Parameters:
//   - db: MySQL handle controlling quoting.
//   - target: registry target to expand.
//
// Return values:
//   - string: complete ALTER TABLE statement.
//   - error: wrapped error when the dialect is unsupported.
func compactAddColumnFallbackSQL(db *gorm.DB, target compactTarget) (string, error) {
	columnType, err := compactColumnType(dialectName(db))
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + quoteIdentifier(db, target.table) +
		" ADD COLUMN " + quoteIdentifier(db, target.compactColumn) + " " + columnType + " NULL" +
		", ALGORITHM=INPLACE, LOCK=NONE", nil
}

// compactMetadataTimeout bounds one catalog read the coordinator performs before a side effect.
//
// It is deliberately derived from the lock timeout rather than from the DDL budget. A catalog
// read has to take a relation lock, so an unrelated ACCESS EXCLUSIVE holder blocks it — and
// section 11 caps lock acquisition at five seconds. Slack is added because the deadline must
// bound a lock wait, not race a healthy read on a busy server.
// Parameters: none.
//
// Return values:
//   - time.Duration: ceiling for one metadata read.
func compactMetadataTimeout() time.Duration {
	return compactLockTimeout() + 2*time.Second
}

// withCompactMetadataDeadline bounds a context for catalog reads taken before a side effect.
//
// Without it the coordinator's verify-before reads wait on a conflicting relation lock for as
// long as the caller's context allows. Measured against a live PostgreSQL 17 server holding an
// ACCESS EXCLUSIVE lock on one table: a cycle blocked for 40 seconds inside "read column metadata"
// and only returned when the caller's own deadline fired. The DDL itself was already bounded —
// execUUIDDDLWithTimeout sets lock_timeout on its pinned session — but the reads that decide
// whether to run that DDL were not, so the cap section 11 sets was not actually enforced across
// the cycle. A bounded context makes the driver cancel the wait instead.
// Parameters:
//   - ctx: caller context.
//
// Return values:
//   - context.Context: context bounded by compactMetadataTimeout.
//   - context.CancelFunc: cancel that the caller must always invoke.
func withCompactMetadataDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, compactMetadataTimeout())
}

// execCompactDDL screens one compact DDL statement and then executes it.
//
// Every compact DDL goes through this rather than execUUIDDDLWithTimeout directly, so the
// forbidden-DDL guard sits on the single path to the database. A statement that would destroy
// legacy storage fails here, before it reaches the executor.
// Parameters:
//   - ctx: context controlling the statement.
//   - db: database handle to execute against.
//   - statement: trusted DDL statement built from registry identifiers.
//
// Return values:
//   - error: the guard's rejection, or the raw database error so callers can classify races.
func execCompactDDL(ctx context.Context, db *gorm.DB, statement string) error {
	if err := compactGuardedDDL(statement); err != nil {
		return err
	}
	return execUUIDDDLWithTimeout(ctx, db, statement, compactDDLTimeout())
}

// addCompactColumn adds one shadow column with verify-before, create, verify-after semantics.
//
// The verification bracket, not the election lock, is what makes concurrent workers safe: a
// duplicate-object race is converted to success only after metadata confirms the exact
// expected column exists.
// Parameters:
//   - ctx: context bounding the metadata reads and the DDL.
//   - db: authoritative handle for the target table.
//   - target: registry target to expand.
//
// Return values:
//   - bool: true when this call created the column.
//   - error: wrapped error when the column cannot be created or verified.
func addCompactColumn(ctx context.Context, db *gorm.DB, target compactTarget) (bool, error) {
	present, err := hasCompactColumn(ctx, db, target)
	if err != nil {
		return false, err
	}
	if present {
		return false, nil
	}

	statement, err := compactAddColumnSQL(db, target)
	if err != nil {
		return false, err
	}
	err = execCompactDDL(ctx, db, statement)
	if err != nil && dialectName(db) == "mysql" && isMySQLOnlineDDLUnsupported(err) {
		fallback, buildErr := compactAddColumnFallbackSQL(db, target)
		if buildErr != nil {
			return false, buildErr
		}
		err = execCompactDDL(ctx, db, fallback)
	}
	if err != nil && !isDuplicateObjectError(err) {
		return false, errors.Wrapf(err, "add compact column %s.%s", target.table, target.compactColumn)
	}

	// Verify after: a duplicate-object classification is only ever accepted once metadata
	// proves the expected column is really there.
	present, verifyErr := hasCompactColumn(ctx, db, target)
	if verifyErr != nil {
		return false, verifyErr
	}
	if !present {
		return false, errors.Errorf("compact column %s.%s is absent after expansion",
			target.table, target.compactColumn)
	}
	return err == nil, nil
}

// hasCompactColumn reports whether one shadow column exists on the authoritative table.
// Parameters:
//   - ctx: context bounding the metadata read.
//   - db: authoritative handle for the target table.
//   - target: registry target to inspect.
//
// Return values:
//   - bool: true when the shadow column exists.
//   - error: wrapped error when the table is missing or metadata cannot be read.
func hasCompactColumn(ctx context.Context, db *gorm.DB, target compactTarget) (bool, error) {
	ctx, cancel := withCompactMetadataDeadline(ctx)
	defer cancel()
	migrator := db.WithContext(ctx).Migrator()
	if migrator == nil {
		return false, errors.Errorf("migrator is unavailable for %s", target.table)
	}
	// The table-name form is used deliberately: passing a model makes GORM parse the
	// struct's indexes under a lock it does not hold, which races other migrator callers.
	if !migrator.HasTable(target.table) {
		return false, nil
	}
	return migrator.HasColumn(target.model, target.compactColumn), nil
}

// verifyCompactColumnType confirms a shadow column has the dialect's exact physical type.
//
// A name match is never sufficient. An operator or a restore could leave a column of the
// right name and the wrong type, which would silently break the compact predicate or store a
// value the codec cannot decode.
// Parameters:
//   - ctx: context bounding the metadata read.
//   - db: authoritative handle for the target table.
//   - target: registry target to inspect.
//
// Return values:
//   - bool: true when the column exists with the expected physical type.
//   - error: wrapped error when metadata cannot be read.
func verifyCompactColumnType(ctx context.Context, db *gorm.DB, target compactTarget) (bool, error) {
	ctx, cancel := withCompactMetadataDeadline(ctx)
	defer cancel()
	types, err := db.WithContext(ctx).Migrator().ColumnTypes(target.model)
	if err != nil {
		return false, errors.Wrapf(err, "read column metadata for %s", target.table)
	}
	for _, columnType := range types {
		if !equalFoldASCII(columnType.Name(), target.compactColumn) {
			continue
		}
		if nullable, ok := columnType.Nullable(); ok && !nullable {
			// A NOT NULL shadow would make an unbackfilled row impossible to represent and
			// would break a legacy insert that omits the column.
			return false, nil
		}
		return compactPhysicalTypeMatches(dialectName(db), columnType.DatabaseTypeName()), nil
	}
	return false, nil
}

// compactPhysicalTypeMatches reports whether an observed database type is the approved
// compact representation for a dialect.
// Parameters:
//   - dialect: lowercase dialect name.
//   - observed: database type name reported by the driver.
//
// Return values:
//   - bool: true when the observed type is the approved representation.
func compactPhysicalTypeMatches(dialect string, observed string) bool {
	switch dialect {
	case "postgres":
		return equalFoldASCII(observed, "uuid")
	case "mysql":
		// Drivers report either the bare type or the fully qualified form.
		return equalFoldASCII(observed, "binary") || equalFoldASCII(observed, "binary(16)")
	case "sqlite":
		return equalFoldASCII(observed, "blob")
	default:
		return false
	}
}

// expandCompactTable adds every shadow column one authoritative table owns and verifies them.
//
// Expansion is per-table and completes before the table's trigger set is installed, and the
// table only becomes eligible for fill once both are verified. That order is what keeps the
// MySQL column/trigger window safe: MySQL auto-commits DDL, so a gap between the two is
// unavoidable, and the historical fill is what covers rows written inside it.
// Parameters:
//   - ctx: context bounding the metadata reads and DDL.
//   - db: authoritative handle for the table.
//   - table: registry table to expand.
//
// Return values:
//   - int: number of shadow columns this call created.
//   - error: wrapped error when a column cannot be created or verified.
func expandCompactTable(ctx context.Context, db *gorm.DB, table compactTable) (int, error) {
	created := 0
	for _, target := range table.targets {
		added, err := addCompactColumn(ctx, db, target)
		if err != nil {
			return created, err
		}
		if added {
			created++
		}
	}
	return created, nil
}

// compactTableExpanded reports whether every shadow column of one table exists with the
// approved physical type.
// Parameters:
//   - ctx: context bounding the metadata reads.
//   - db: authoritative handle for the table.
//   - table: registry table to inspect.
//
// Return values:
//   - bool: true when the table is fully and correctly expanded.
//   - error: wrapped error when metadata cannot be read.
func compactTableExpanded(ctx context.Context, db *gorm.DB, table compactTable) (bool, error) {
	if !db.WithContext(ctx).Migrator().HasTable(table.table) {
		return false, nil
	}
	for _, target := range table.targets {
		ok, err := verifyCompactColumnType(ctx, db, target)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// equalFoldASCII compares two ASCII identifiers case-insensitively.
//
// Identifier comparison deliberately avoids strings.EqualFold's Unicode folding: database
// identifiers here are ASCII by construction, and Unicode folding would make unrelated
// identifiers compare equal.
// Parameters:
//   - left: first identifier.
//   - right: second identifier.
//
// Return values:
//   - bool: true when the identifiers match ignoring ASCII case.
func equalFoldASCII(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := 0; index < len(left); index++ {
		if lowerASCII(left[index]) != lowerASCII(right[index]) {
			return false
		}
	}
	return true
}
