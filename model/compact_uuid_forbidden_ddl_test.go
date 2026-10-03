package model

// Static forbidden-DDL assertion for compact UUID storage (AUTO-016; evidence for AUTO-A12,
// alongside AUTO-T30).
//
// Proposal section 10.3 makes the legacy text representation permanent: no command, environment
// value, hidden phase, or automatic branch may drop, rename, or retype a legacy UUID column or
// its index while the compatibility contract is active.
//
// This is the STATIC half of that guarantee and the complement of the runtime guard in
// compact_uuid_forbidden_ddl.go, which TestCompactUUIDForbiddenDDL covers. The runtime guard
// makes a destructive statement fail before it reaches the database; this test proves no
// production source contains one at all. Neither alone is sufficient: a static scan cannot see
// an identifier that is computed at runtime, and a runtime guard cannot prove absence.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// compactDestructiveVerbs returns the DDL verbs proposal section 10.3 prohibits on legacy storage.
// Parameters: none.
//
// Return values:
//   - []string: lowercase verbs that drop, rename, or retype a column or index.
func compactDestructiveVerbs() []string {
	return []string{
		"drop column",
		"drop index",
		"rename column",
		"modify column",
		"alter column",
		"change column",
	}
}

// compactProtectedLegacyIdentifiers returns every legacy identifier the contract makes permanent.
//
// It is derived from compactRegistry() and the v3 index naming functions rather than written out,
// so a new registry column is protected the day it is added.
// Parameters: none.
//
// Return values:
//   - map[string]struct{}: lowercase legacy column and index identifiers.
func compactProtectedLegacyIdentifiers() map[string]struct{} {
	identifiers := map[string]struct{}{}
	for _, target := range compactRegistry() {
		identifiers[strings.ToLower(target.legacyColumn)] = struct{}{}
		if target.kind == compactKindOwned {
			identifiers[strings.ToLower(ordinaryUUIDIndexName(target.table))] = struct{}{}
			identifiers[strings.ToLower(uuidUniqueIndexName(target.table))] = struct{}{}
			continue
		}
		identifiers[strings.ToLower(fkUUIDIndexName(target.table, target.legacyColumn))] = struct{}{}
	}
	return identifiers
}

// compactStatementTokens splits a statement fragment into bare SQL identifier tokens.
//
// Whole-token splitting is the whole trick: `uuid` is a prefix of `uuid_compact`, and
// `idx_users_uuid` is a prefix of `idx_users_uuid_compact`, so a substring test would flag every
// legitimate compact statement. This mirrors mentionsIdentifier in compact_uuid_forbidden_ddl.go.
// Parameters:
//   - fragment: lowercase statement fragment.
//
// Return values:
//   - []string: identifier tokens with quoting and punctuation stripped.
func compactStatementTokens(fragment string) []string {
	return strings.FieldsFunc(fragment, func(char rune) bool {
		switch char {
		case ' ', ',', '(', ')', ';', '`', '"', '\'', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
}

// destroysLegacyUUIDStorage reports whether a statement would destroy legacy UUID storage.
//
// It is shape-based, exactly like the runtime guard, and it deliberately has no "the compact
// shadow is also mentioned, so allow it" escape hatch: whole-token matching already prevents the
// prefix false positive, and an escape hatch would let `RENAME COLUMN uuid TO uuid_compact`
// through.
// Parameters:
//   - statement: candidate SQL statement in any casing or spacing.
//
// Return values:
//   - string: bounded description of the violation, empty when the statement is safe.
//   - bool: true when the statement would drop, rename, or retype legacy UUID storage.
func destroysLegacyUUIDStorage(statement string) (string, bool) {
	normalized := strings.Join(strings.Fields(strings.ToLower(statement)), " ")
	protected := compactProtectedLegacyIdentifiers()
	for _, verb := range compactDestructiveVerbs() {
		index := strings.Index(normalized, verb)
		if index < 0 {
			continue
		}
		for _, token := range compactStatementTokens(normalized[index+len(verb):]) {
			if _, found := protected[token]; found {
				return verb + " " + token, true
			}
		}
	}
	return "", false
}

// compactProductionSQLLiterals returns every string literal in one non-test Go source file.
//
// Only real string literals are examined: parsing without ParseComments keeps a comment that
// merely discusses `DROP COLUMN uuid` — compact_uuid_forbidden_ddl.go is full of them — from
// being mistaken for a statement.
// Parameters:
//   - path: absolute path to a Go source file.
//
// Return values:
//   - []string: every unquoted string literal in the file.
//   - error: wrapped error when the file cannot be parsed.
func compactProductionSQLLiterals(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s", path)
	}
	literals := []string{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		literals = append(literals, value)
		return true
	})
	return literals, nil
}

// compactProductionGoFiles lists every non-test Go source file in the repository.
// Parameters:
//   - t: test handle used for assertions.
//
// Return values:
//   - []string: absolute paths to production Go sources.
func compactProductionGoFiles(t *testing.T) []string {
	t.Helper()
	root, err := filepath.Abs("..")
	require.NoError(t, errors.Wrap(err, "resolve the repository root"))

	paths := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.Wrapf(walkErr, "walk %s", path)
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "vendor", "docs", ".git", "node_modules", "web":
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	require.NoError(t, errors.Wrap(err, "walk the repository"))
	return paths
}

func TestCompactUUIDForbiddenDDLStatic(t *testing.T) {
	// AUTO-016 static half / AUTO-A12: destructive cleanup must be impossible while the
	// compatibility contract is active. TestCompactUUIDForbiddenDDL proves the runtime guard
	// rejects such a statement; this proves no production path contains one.

	t.Run("the detector flags every destructive control", func(t *testing.T) {
		// Positive controls. Without these the scan below could pass vacuously.
		for _, statement := range []string{
			`ALTER TABLE users DROP COLUMN uuid`,
			`ALTER TABLE "users" DROP COLUMN "uuid"`,
			"ALTER TABLE `users` DROP COLUMN `uuid`;",
			`alter table users drop   column    uuid`,
			`ALTER TABLE "tokens" RENAME COLUMN "user_uuid" TO "legacy_user_uuid"`,
			`ALTER TABLE "users" MODIFY COLUMN "uuid" VARBINARY(16)`,
			`ALTER TABLE "logs" ALTER COLUMN "uuid" TYPE bytea`,
			`ALTER TABLE "channels" CHANGE COLUMN "uuid" "uuid2" CHAR(36)`,
			`DROP INDEX "uuid" ON "users"`,
			`DROP INDEX idx_users_uuid ON users`,
			`DROP INDEX "idx_users_uuid_unique"`,
			`DROP INDEX idx_tokens_user_uuid`,
			// The compact shadow being mentioned must not excuse destroying the legacy column.
			`ALTER TABLE users RENAME COLUMN uuid TO uuid_compact`,
			`ALTER TABLE users DROP COLUMN uuid_compact, DROP COLUMN uuid`,
		} {
			reason, destructive := destroysLegacyUUIDStorage(statement)
			require.True(t, destructive, "must be detected as destructive: %s", statement)
			require.NotEmpty(t, reason)
		}
	})

	t.Run("the detector permits the real additive compact DDL", func(t *testing.T) {
		// Negative controls, taken from the real generators rather than written by hand: a
		// detector that flagged the migration's own work would be useless and would be
		// "fixed" by weakening it.
		db, _ := newCompactTestTopology(t)
		for _, table := range compactTablesForRole(uuidRolePrimary) {
			for _, target := range table.targets {
				statement, err := compactAddColumnSQL(db, target)
				require.NoError(t, err)
				requireCompactStatementPermitted(t, statement)

				fallback, err := compactAddColumnFallbackSQL(db, target)
				require.NoError(t, err)
				requireCompactStatementPermitted(t, fallback)

				requireCompactStatementPermitted(t,
					`CREATE UNIQUE INDEX "`+target.indexName()+`" ON "`+target.table+
						`" ("`+target.compactColumn+`")`)
				requireCompactStatementPermitted(t, `DROP INDEX "`+target.indexName()+`"`)
				requireCompactStatementPermitted(t,
					`ALTER TABLE "`+target.table+`" DROP COLUMN "`+target.compactColumn+`"`)
			}
			statements, err := compactTriggerDDL(db, table)
			require.NoError(t, err)
			for _, statement := range statements {
				requireCompactStatementPermitted(t, statement)
			}
		}
	})

	t.Run("no production source can destroy legacy uuid storage", func(t *testing.T) {
		paths := compactProductionGoFiles(t)
		require.Greater(t, len(paths), 100,
			"the scan must actually cover the repository, not silently find nothing")

		literals := 0
		scanned := map[string]bool{}
		for _, path := range paths {
			values, err := compactProductionSQLLiterals(path)
			require.NoError(t, err)
			literals += len(values)
			scanned[filepath.Base(path)] = true
			for _, value := range values {
				reason, destructive := destroysLegacyUUIDStorage(value)
				require.False(t, destructive,
					"%s contains a statement that would %s, which proposal section 10.3 prohibits: %q",
					path, reason, value)
			}
		}
		require.Greater(t, literals, 1000, "the scan must actually read string literals")
		// Spot-check that the files that really do build UUID DDL were covered, so a broken
		// walk cannot make this pass by skipping exactly the interesting sources.
		for _, name := range []string{
			"compact_uuid_schema.go", "compact_uuid_trigger.go",
			"uuid_unique_migration.go", "uuid_index_ddl.go",
		} {
			require.True(t, scanned[name], "the scan must cover %s", name)
		}
	})
}

// requireCompactStatementPermitted asserts one legitimate statement is not flagged.
// Parameters:
//   - t: test handle used for assertions.
//   - statement: additive or compact-owned statement that must be permitted.
//
// Return values: none.
func requireCompactStatementPermitted(t *testing.T, statement string) {
	t.Helper()
	reason, destructive := destroysLegacyUUIDStorage(statement)
	require.False(t, destructive,
		"legitimate statement must not be flagged as %s: %s", reason, statement)
}
