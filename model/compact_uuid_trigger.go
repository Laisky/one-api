package model

// This file generates the versioned synchronization triggers (AUTO-004).
//
// One trigger set per authoritative table derives every compact shadow from the final legacy
// text of the same row, on every supported insert and update. Text is never derived from
// compact, which removes any bidirectional last-writer ambiguity.
//
// Three properties are contractual and are what the golden DDL tests pin:
//
//  1. A trigger never makes a previously accepted legacy write fail. Invalid text stays
//     exactly as written, derives compact NULL, and degrades migration health instead.
//  2. Every dialect implements the same shape check before conversion: exactly 36 characters,
//     hyphens at positions 9/14/19/24, hexadecimal elsewhere, version nibble 7, and an RFC
//     variant nibble. Conversion runs only after that check succeeds.
//  3. SQL is built from compile-time registry identifiers only.

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactTriggerPrefix namespaces every generated synchronization object.
// It embeds the object version, so a body change bumps compactObjectVersion and the new
// objects cannot be confused with the previous generation's.
const compactTriggerPrefix = "cuuid_" + compactObjectVersion + "_"

// compactUUIDShapeRegex is the PostgreSQL/MySQL shape check applied before conversion.
//
// It encodes the full contract in one expression: 36 characters, hyphens at the four fixed
// offsets, hexadecimal everywhere else, a literal version nibble 7, and an RFC variant nibble.
// Both cases are spelled out explicitly rather than relying on a case-insensitive flag,
// because MySQL's REGEXP case sensitivity follows the column collation and the derivation must
// be identical on every deployment.
const compactUUIDShapeRegex = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-7[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`

// compactSyncTriggerName returns the PostgreSQL trigger name for one table.
// Parameters:
//   - table: trusted registry table name.
//
// Return values:
//   - string: deterministic trigger name.
func compactSyncTriggerName(table string) string {
	return compactTriggerPrefix + table + "_sync"
}

// compactSyncFunctionName returns the PostgreSQL trigger function name for one table.
// Parameters:
//   - table: trusted registry table name.
//
// Return values:
//   - string: deterministic function name.
func compactSyncFunctionName(table string) string {
	return compactTriggerPrefix + table + "_sync_fn"
}

// compactInsertTriggerName returns the MySQL/SQLite insert trigger name for one table.
// Parameters:
//   - table: trusted registry table name.
//
// Return values:
//   - string: deterministic trigger name.
func compactInsertTriggerName(table string) string {
	return compactTriggerPrefix + table + "_insert"
}

// compactUpdateTriggerName returns the MySQL/SQLite update trigger name for one table.
// Parameters:
//   - table: trusted registry table name.
//
// Return values:
//   - string: deterministic trigger name.
func compactUpdateTriggerName(table string) string {
	return compactTriggerPrefix + table + "_update"
}

// compactTriggerNames returns every synchronization object name one table owns on a dialect.
// Parameters:
//   - dialect: lowercase dialect name.
//   - table: trusted registry table name.
//
// Return values:
//   - []string: object names, in installation order.
//   - error: wrapped error when the dialect is unsupported.
func compactTriggerNames(dialect string, table string) ([]string, error) {
	switch dialect {
	case "postgres":
		return []string{compactSyncTriggerName(table)}, nil
	case "mysql", "sqlite":
		return []string{compactInsertTriggerName(table), compactUpdateTriggerName(table)}, nil
	default:
		return nil, errors.Errorf("compact uuid storage has no trigger contract for dialect %q", dialect)
	}
}

// compactTriggerDDL returns the complete ordered DDL that installs one table's trigger set.
//
// Every statement is idempotent-by-construction where the dialect allows it, and the caller
// still verifies the installed metadata afterwards: a matching name alone is never accepted
// as evidence that the correct body is installed.
// Parameters:
//   - db: authoritative handle whose dialect selects the contract.
//   - table: registry table whose targets the triggers derive.
//
// Return values:
//   - []string: ordered DDL statements.
//   - error: wrapped error when the dialect is unsupported.
func compactTriggerDDL(db *gorm.DB, table compactTable) ([]string, error) {
	switch dialectName(db) {
	case "postgres":
		return compactPostgresTriggerDDL(db, table), nil
	case "mysql":
		return compactMySQLTriggerDDL(db, table), nil
	case "sqlite":
		return compactSQLiteTriggerDDL(db, table), nil
	default:
		return nil, errors.Errorf("compact uuid storage has no trigger contract for dialect %q", dialectName(db))
	}
}

// =============================================================================
// POSTGRESQL
// =============================================================================

// compactPostgresTriggerDDL builds the PostgreSQL function and trigger for one table.
//
// The function is SECURITY INVOKER with a pinned search_path of pg_catalog, pg_temp. Both
// matter: SECURITY INVOKER keeps the trigger from becoming a privilege-escalation vector, and
// the pinned path means a schema-qualified shadowing of lower() or the uuid type cannot change
// what the trigger derives.
//
// The regex check runs before the cast, so invalid text derives NULL rather than aborting the
// caller's transaction with a bad-input error.
// Parameters:
//   - db: PostgreSQL handle controlling quoting.
//   - table: registry table whose targets the trigger derives.
//
// Return values:
//   - []string: ordered DDL statements.
func compactPostgresTriggerDDL(db *gorm.DB, table compactTable) []string {
	body := &strings.Builder{}
	body.WriteString("CREATE OR REPLACE FUNCTION " + quoteIdentifier(db, compactSyncFunctionName(table.table)) + "()\n")
	body.WriteString("RETURNS trigger\n")
	body.WriteString("LANGUAGE plpgsql\n")
	body.WriteString("SECURITY INVOKER\n")
	body.WriteString("SET search_path = pg_catalog, pg_temp\n")
	body.WriteString("AS $cuuid$\n")
	body.WriteString("BEGIN\n")
	for _, target := range table.targets {
		legacy := "NEW." + quoteIdentifier(db, target.legacyColumn)
		shadow := "NEW." + quoteIdentifier(db, target.compactColumn)
		body.WriteString("  IF " + legacy + " IS NOT NULL AND " + legacy +
			" ~ '" + compactUUIDShapeRegex + "' THEN\n")
		body.WriteString("    " + shadow + " := lower(" + legacy + ")::uuid;\n")
		body.WriteString("  ELSE\n")
		body.WriteString("    " + shadow + " := NULL;\n")
		body.WriteString("  END IF;\n")
	}
	body.WriteString("  RETURN NEW;\n")
	body.WriteString("END;\n")
	body.WriteString("$cuuid$;")

	trigger := "CREATE TRIGGER " + quoteIdentifier(db, compactSyncTriggerName(table.table)) +
		" BEFORE INSERT OR UPDATE ON " + quoteIdentifier(db, table.table) +
		" FOR EACH ROW EXECUTE FUNCTION " + quoteIdentifier(db, compactSyncFunctionName(table.table)) + "()"

	return []string{
		body.String(),
		// The drop keeps installation idempotent across retries and object-version changes.
		// It only ever targets this generation's own trigger, never a legacy object.
		"DROP TRIGGER IF EXISTS " + quoteIdentifier(db, compactSyncTriggerName(table.table)) +
			" ON " + quoteIdentifier(db, table.table),
		trigger,
	}
}

// =============================================================================
// MYSQL
// =============================================================================

// compactMySQLTriggerDDL builds the MySQL BEFORE INSERT and BEFORE UPDATE triggers.
//
// UUID_TO_BIN's second argument is pinned to 0. Swap flag 1 is forbidden by the proposal: it
// reorders the timestamp fields and would make MySQL's bytes disagree with every other
// dialect and with the Go codec.
// Parameters:
//   - db: MySQL handle controlling quoting.
//   - table: registry table whose targets the triggers derive.
//
// Return values:
//   - []string: ordered DDL statements.
func compactMySQLTriggerDDL(db *gorm.DB, table compactTable) []string {
	statements := make([]string, 0, 4)
	for _, spec := range []struct {
		name  string
		event string
	}{
		{name: compactInsertTriggerName(table.table), event: "INSERT"},
		{name: compactUpdateTriggerName(table.table), event: "UPDATE"},
	} {
		statements = append(statements, "DROP TRIGGER IF EXISTS "+quoteIdentifier(db, spec.name))

		body := &strings.Builder{}
		body.WriteString("CREATE TRIGGER " + quoteIdentifier(db, spec.name) +
			" BEFORE " + spec.event + " ON " + quoteIdentifier(db, table.table) + "\n")
		body.WriteString("FOR EACH ROW\n")
		body.WriteString("BEGIN\n")
		for _, target := range table.targets {
			legacy := "NEW." + quoteIdentifier(db, target.legacyColumn)
			shadow := "NEW." + quoteIdentifier(db, target.compactColumn)
			body.WriteString("  IF " + legacy + " IS NOT NULL AND " + legacy +
				" REGEXP '" + compactUUIDShapeRegex + "' THEN\n")
			body.WriteString("    SET " + shadow + " = UUID_TO_BIN(LOWER(" + legacy + "), 0);\n")
			body.WriteString("  ELSE\n")
			body.WriteString("    SET " + shadow + " = NULL;\n")
			body.WriteString("  END IF;\n")
		}
		body.WriteString("END")
		statements = append(statements, body.String())
	}
	return statements
}

// =============================================================================
// SQLITE
// =============================================================================

// compactSQLiteTriggerDDL builds the SQLite AFTER INSERT and AFTER UPDATE triggers.
//
// SQLite has no BEFORE-trigger assignment to NEW, so synchronization is an AFTER trigger that
// updates the row. Two details make that safe:
//
//   - Only core SQL is used. A Go connection-local function would not exist in a pinned old
//     binary's process, and these triggers are persistent main-schema objects that the old
//     binary's writes must fire.
//   - The WHEN clause is a null-safe mismatch predicate, so the inner UPDATE terminates with
//     recursive_triggers both ON and OFF. With recursion ON the inner UPDATE re-fires the
//     update trigger once, whose WHEN is then false because the shadows already match; with
//     recursion OFF it never re-fires at all.
//
// Parameters:
//   - db: SQLite handle controlling quoting.
//   - table: registry table whose targets the triggers derive.
//
// Return values:
//   - []string: ordered DDL statements.
func compactSQLiteTriggerDDL(db *gorm.DB, table compactTable) []string {
	statements := make([]string, 0, 4)
	for _, spec := range []struct {
		name  string
		event string
	}{
		{name: compactInsertTriggerName(table.table), event: "INSERT"},
		{name: compactUpdateTriggerName(table.table), event: "UPDATE"},
	} {
		statements = append(statements, "DROP TRIGGER IF EXISTS "+quoteIdentifier(db, spec.name))

		assignments := make([]string, 0, len(table.targets))
		mismatches := make([]string, 0, len(table.targets))
		for _, target := range table.targets {
			derived := compactSQLiteDeriveExpr(db, target, "NEW.")
			assignments = append(assignments,
				quoteIdentifier(db, target.compactColumn)+" = "+compactSQLiteDeriveExpr(db, target, ""))
			// IS NOT is SQLite's null-safe inequality: it is true when exactly one side is
			// NULL, which a plain <> would report as NULL and silently skip.
			mismatches = append(mismatches,
				"NEW."+quoteIdentifier(db, target.compactColumn)+" IS NOT "+derived)
		}

		body := &strings.Builder{}
		body.WriteString("CREATE TRIGGER " + quoteIdentifier(db, spec.name) +
			" AFTER " + spec.event + " ON " + quoteIdentifier(db, table.table) + "\n")
		body.WriteString("FOR EACH ROW\n")
		body.WriteString("WHEN " + strings.Join(mismatches, "\n  OR ") + "\n")
		body.WriteString("BEGIN\n")
		body.WriteString("  UPDATE " + quoteIdentifier(db, table.table) + "\n")
		body.WriteString("  SET " + strings.Join(assignments, ",\n      ") + "\n")
		body.WriteString("  WHERE " + quoteIdentifier(db, "id") + " = NEW." + quoteIdentifier(db, "id") + ";\n")
		body.WriteString("END")
		statements = append(statements, body.String())
	}
	return statements
}

// compactSQLiteDeriveExpr builds the guarded core-SQL derivation for one column.
//
// The structural guard runs inside a CASE, so unhex is evaluated only after the shape check
// succeeds. unhex itself completes the contract: it is the hexadecimal validator, and it
// returns NULL rather than raising for non-hexadecimal input, which is exactly the required
// "invalid text derives NULL and never aborts the write" behavior.
//
// The length(replace(...)) = 32 guard is not redundant with the length 36 check. Verifying the
// four fixed hyphen offsets does not rule out additional hyphens elsewhere in the string, and
// an even number of extra hyphens would otherwise let unhex succeed on a short value.
// Parameters:
//   - db: SQLite handle controlling quoting.
//   - target: registry target whose legacy column is derived.
//   - prefix: "NEW." inside a WHEN clause, empty inside the UPDATE's SET list.
//
// Return values:
//   - string: complete CASE expression yielding a 16-byte BLOB or NULL.
func compactSQLiteDeriveExpr(db *gorm.DB, target compactTarget, prefix string) string {
	column := prefix + quoteIdentifier(db, target.legacyColumn)
	stripped := "replace(lower(" + column + "), '-', '')"

	guards := []string{
		column + " IS NOT NULL",
		"length(" + column + ") = 36",
		"substr(" + column + ", 9, 1) = '-'",
		"substr(" + column + ", 14, 1) = '-'",
		"substr(" + column + ", 19, 1) = '-'",
		"substr(" + column + ", 24, 1) = '-'",
		"lower(substr(" + column + ", 15, 1)) = '7'",
		"lower(substr(" + column + ", 20, 1)) IN ('8', '9', 'a', 'b')",
		"length(replace(" + column + ", '-', '')) = 32",
	}
	return "CASE WHEN " + strings.Join(guards, "\n        AND ") +
		"\n        THEN unhex(" + stripped + ")\n        ELSE NULL END"
}
