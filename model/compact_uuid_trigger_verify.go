package model

// This file installs and verifies the synchronization trigger sets (AUTO-004).
//
// Verification is deliberately more than a name lookup. The proposal is explicit that "a
// matching name alone is insufficient": an operator, a restore, or a partially applied upgrade
// can leave an object with the right name and a wrong or stale body, and accepting it by name
// would let the coordinator mark a migration complete over shadows nothing is maintaining.
// So every check compares timing, event, table, the canonicalized body hash, security
// properties, enabled state, and every source/target pair.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactTriggerState is one table's observed synchronization-object health.
type compactTriggerState struct {
	// installed reports whether every expected object exists with a matching body.
	installed bool
	// reason is a bounded, value-free description of the first mismatch found.
	reason string
}

// installCompactTriggers creates or replaces one table's complete trigger set.
//
// The whole set is installed as a unit and verified afterwards, because a table is only
// eligible for historical fill once its shadows and its complete trigger set are both
// verified. A partially installed set would let the fill run while some columns had no
// synchronization at all.
// Parameters:
//   - ctx: context bounding the DDL.
//   - db: authoritative handle for the table.
//   - table: registry table whose triggers are installed.
//
// Return values:
//   - error: wrapped error when a statement fails or the set does not verify afterwards.
func installCompactTriggers(ctx context.Context, db *gorm.DB, table compactTable) error {
	statements, err := compactTriggerDDL(db, table)
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if err := execCompactDDL(ctx, db, statement); err != nil {
			if isDuplicateObjectError(err) {
				continue
			}
			return errors.Wrapf(err, "install compact sync trigger set for %s", table.table)
		}
	}

	state, err := verifyCompactTriggers(ctx, db, table)
	if err != nil {
		return err
	}
	if !state.installed {
		return errors.Errorf("compact sync trigger set for %s did not verify after installation: %s",
			table.table, state.reason)
	}
	return nil
}

// verifyCompactTriggers checks one table's trigger set against its versioned body contract.
// Parameters:
//   - ctx: context bounding the metadata reads.
//   - db: authoritative handle for the table.
//   - table: registry table to verify.
//
// Return values:
//   - compactTriggerState: observed health with a bounded, value-free reason on mismatch.
//   - error: wrapped error when catalog metadata cannot be read.
func verifyCompactTriggers(ctx context.Context, db *gorm.DB, table compactTable) (compactTriggerState, error) {
	switch dialectName(db) {
	case "postgres":
		return verifyPostgresCompactTriggers(ctx, db, table)
	case "mysql":
		return verifyMySQLCompactTriggers(ctx, db, table)
	case "sqlite":
		return verifySQLiteCompactTriggers(ctx, db, table)
	default:
		return compactTriggerState{}, errors.Errorf(
			"compact uuid storage has no trigger contract for dialect %q", dialectName(db))
	}
}

// =============================================================================
// BODY CANONICALIZATION
// =============================================================================

// canonicalizeTriggerBody normalizes an engine-reported trigger body for hashing.
//
// Engines do not return the body byte-for-byte as submitted: they add or strip outer
// delimiters, reformat whitespace, and re-case keywords. Hashing the raw text would make
// verification fail on a correctly installed trigger. The normalization is exactly the one the
// proposal specifies:
//
//   - line endings become LF and the edges are trimmed;
//   - comments are removed;
//   - whitespace outside quoted tokens collapses to one byte; and
//   - unquoted keywords and function names lowercase, while quoted identifiers and literals
//     are preserved byte-for-byte.
//
// Preserving quoted regions byte-for-byte is what keeps the shape regex and the literals
// meaningful: lowercasing inside them would silently change the accept/reject boundary.
// Parameters:
//   - body: engine-reported trigger or function body.
//
// Return values:
//   - string: canonical body suitable for hashing.
func canonicalizeTriggerBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = stripSQLComments(body)

	out := &strings.Builder{}
	out.Grow(len(body))
	pendingSpace := false
	index := 0
	for index < len(body) {
		char := body[index]
		if char == ' ' || char == '\t' || char == '\n' {
			// Collapse any run of whitespace to a single byte, but only emit it once a
			// non-whitespace byte follows, which also trims both edges.
			if out.Len() > 0 {
				pendingSpace = true
			}
			index++
			continue
		}
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
		if char == '\'' || char == '"' || char == '`' {
			end := closingQuote(body, index)
			// A quoted region is copied verbatim: literals and quoted identifiers carry
			// meaning that folding case or whitespace would destroy.
			out.WriteString(body[index:end])
			index = end
			continue
		}
		out.WriteByte(lowerASCII(char))
		index++
	}
	return out.String()
}

// closingQuote returns the offset just past the quoted token starting at open.
//
// SQL escapes a quote by doubling it, so a doubled quote continues the same token rather than
// ending it. An unterminated token consumes the rest of the input, which canonicalizes
// deterministically instead of panicking.
// Parameters:
//   - body: full text being scanned.
//   - open: offset of the opening quote byte.
//
// Return values:
//   - int: offset immediately after the token's closing quote.
func closingQuote(body string, open int) int {
	quote := body[open]
	index := open + 1
	for index < len(body) {
		if body[index] != quote {
			index++
			continue
		}
		if index+1 < len(body) && body[index+1] == quote {
			index += 2
			continue
		}
		return index + 1
	}
	return len(body)
}

// stripSQLComments removes line and block comments outside quoted tokens.
// Parameters:
//   - body: text to strip.
//
// Return values:
//   - string: text with comments replaced by a single space.
func stripSQLComments(body string) string {
	out := &strings.Builder{}
	out.Grow(len(body))
	index := 0
	for index < len(body) {
		char := body[index]
		if char == '\'' || char == '"' || char == '`' {
			end := closingQuote(body, index)
			out.WriteString(body[index:end])
			index = end
			continue
		}
		if char == '-' && index+1 < len(body) && body[index+1] == '-' {
			for index < len(body) && body[index] != '\n' {
				index++
			}
			out.WriteByte(' ')
			continue
		}
		if char == '/' && index+1 < len(body) && body[index+1] == '*' {
			index += 2
			for index+1 < len(body) && !(body[index] == '*' && body[index+1] == '/') {
				index++
			}
			index += 2
			if index > len(body) {
				index = len(body)
			}
			out.WriteByte(' ')
			continue
		}
		out.WriteByte(char)
		index++
	}
	return out.String()
}

// triggerBodyHash returns the SHA-256 of a canonicalized body.
//
// The digest is evidence, not a secret, but it is never logged: the proposal forbids exposing
// fingerprint bytes, and a body hash is only ever compared in memory.
// Parameters:
//   - body: engine-reported body.
//
// Return values:
//   - string: lowercase hex SHA-256 of the canonical body.
func triggerBodyHash(body string) string {
	digest := sha256.Sum256([]byte(canonicalizeTriggerBody(body)))
	return hex.EncodeToString(digest[:])
}

// expectedCompactTriggerHash returns the compile-time expected body hash for one table.
//
// The expectation is derived from the same generator that installs the trigger, so the hash is
// versioned by dialect implicitly: a change to the generated body changes this value, and
// every already-installed trigger then fails verification and is reinstalled.
// Parameters:
//   - db: authoritative handle whose dialect selects the generator.
//   - table: registry table.
//   - object: which generated object's body to hash.
//
// Return values:
//   - string: expected canonical body hash.
//   - error: wrapped error when the dialect is unsupported.
func expectedCompactTriggerHash(db *gorm.DB, table compactTable, object compactTriggerObject) (string, error) {
	body, err := expectedCompactTriggerBody(db, table, object)
	if err != nil {
		return "", err
	}
	return triggerBodyHash(body), nil
}

// compactTriggerObject names one generated synchronization object within a table's set.
type compactTriggerObject string

const (
	// compactObjectPostgresFunction is the PostgreSQL trigger function body.
	compactObjectPostgresFunction compactTriggerObject = "postgres_function"
	// compactObjectInsertTrigger is the MySQL/SQLite insert trigger body.
	compactObjectInsertTrigger compactTriggerObject = "insert_trigger"
	// compactObjectUpdateTrigger is the MySQL/SQLite update trigger body.
	compactObjectUpdateTrigger compactTriggerObject = "update_trigger"
)

// expectedCompactTriggerBody returns the generated body an engine is expected to report.
//
// The engine stores only part of what the installer submits: PostgreSQL keeps the function's
// inner body without the CREATE FUNCTION envelope, and MySQL keeps the trigger's action
// statement without the CREATE TRIGGER header. Each branch therefore extracts the same region
// the catalog will report, so the two hashes are comparable.
// Parameters:
//   - db: authoritative handle whose dialect selects the generator.
//   - table: registry table.
//   - object: which generated object's body to return.
//
// Return values:
//   - string: generated body region matching what the catalog reports.
//   - error: wrapped error when the dialect or object is unsupported.
func expectedCompactTriggerBody(db *gorm.DB, table compactTable, object compactTriggerObject) (string, error) {
	statements, err := compactTriggerDDL(db, table)
	if err != nil {
		return "", err
	}
	switch dialectName(db) {
	case "postgres":
		if object != compactObjectPostgresFunction {
			return "", errors.Errorf("postgres compact trigger has no object %q", object)
		}
		// statements[0] is the CREATE OR REPLACE FUNCTION; pg_proc.prosrc reports only the
		// text between the dollar-quote delimiters.
		return extractDollarQuoted(statements[0]), nil
	case "mysql":
		switch object {
		case compactObjectInsertTrigger:
			// statements: [drop, create-insert, drop, create-update].
			return extractAfterEachRow(statements[1]), nil
		case compactObjectUpdateTrigger:
			return extractAfterEachRow(statements[3]), nil
		default:
			return "", errors.Errorf("mysql compact trigger has no object %q", object)
		}
	case "sqlite":
		switch object {
		case compactObjectInsertTrigger:
			// SQLite reports the entire original CREATE TRIGGER text in sqlite_master.sql.
			return statements[1], nil
		case compactObjectUpdateTrigger:
			return statements[3], nil
		default:
			return "", errors.Errorf("sqlite compact trigger has no object %q", object)
		}
	default:
		return "", errors.Errorf("compact uuid storage has no trigger contract for dialect %q", dialectName(db))
	}
}

// extractDollarQuoted returns the text inside the generator's $cuuid$ delimiters.
// Parameters:
//   - statement: generated CREATE OR REPLACE FUNCTION statement.
//
// Return values:
//   - string: the function's inner body, or the whole statement when the delimiters are absent.
func extractDollarQuoted(statement string) string {
	const delimiter = "$cuuid$"
	start := strings.Index(statement, delimiter)
	if start < 0 {
		return statement
	}
	start += len(delimiter)
	end := strings.LastIndex(statement, delimiter)
	if end <= start {
		return statement
	}
	return statement[start:end]
}

// extractAfterEachRow returns the trigger's action statement, dropping the CREATE header.
//
// MySQL's information_schema.triggers reports ACTION_STATEMENT, which is only the BEGIN...END
// block. Comparing that against the full CREATE TRIGGER text would never match.
// Parameters:
//   - statement: generated CREATE TRIGGER statement.
//
// Return values:
//   - string: the action statement, or the whole statement when the marker is absent.
func extractAfterEachRow(statement string) string {
	const marker = "FOR EACH ROW"
	index := strings.Index(statement, marker)
	if index < 0 {
		return statement
	}
	return strings.TrimSpace(statement[index+len(marker):])
}
