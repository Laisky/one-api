package model

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	uuidScaleDefaultLogRows   = 1_000_000
	uuidScaleDefaultTokenRows = 100_000
	uuidScaleInsertBatchSize  = 500
)

// TestMigrateExternalUUIDsScaleT27 verifies the T27 large backfill acceptance scenario.
// Parameters: none.
//
// Return values: none.
func TestMigrateExternalUUIDsScaleT27(t *testing.T) {
	if os.Getenv("ONEAPI_UUID_SCALE_TEST") != "1" {
		t.Skip("set ONEAPI_UUID_SCALE_TEST=1 to run the T27 UUID scale backfill acceptance test")
	}

	logRows := uuidScaleEnvInt(t, "ONEAPI_UUID_SCALE_LOG_ROWS", uuidScaleDefaultLogRows)
	tokenRows := uuidScaleEnvInt(t, "ONEAPI_UUID_SCALE_TOKEN_ROWS", uuidScaleDefaultTokenRows)
	if logRows < uuidScaleDefaultLogRows || tokenRows < uuidScaleDefaultTokenRows {
		t.Logf("running reduced UUID scale smoke: logs=%d tokens=%d; T27 acceptance requires logs>=%d tokens>=%d",
			logRows, tokenRows, uuidScaleDefaultLogRows, uuidScaleDefaultTokenRows)
	}

	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			originalDB := DB
			originalLOGDB := LOG_DB
			var db *gorm.DB
			switch dialect {
			case "sqlite":
				db = setupMigrationTestDB(t)
			case "mysql", "postgres":
				db = openBackend(t, dialect)
				if db == nil {
					t.Skipf("%s DSN not set, skipping UUID scale acceptance test", dialect)
				}
			default:
				t.Fatalf("unsupported dialect %q", dialect)
			}

			DB = db
			LOG_DB = db
			t.Cleanup(func() {
				DB = originalDB
				LOG_DB = originalLOGDB
				resetBackendFlags()
			})

			dropUUIDMigrationTables(t, db)
			require.NoError(t, migrateDB())
			seedUUIDScaleReferenceRows(t, db)
			seedUUIDScaleTokens(t, db, tokenRows)
			seedUUIDScaleLogs(t, db, logRows, tokenRows)

			start := time.Now()
			require.NoError(t, MigrateExternalUUIDs(context.Background()))
			duration := time.Since(start)

			requireUUIDScaleColumnComplete(t, db, "logs", "uuid")
			requireUUIDScaleColumnComplete(t, db, "logs", "user_uuid")
			requireUUIDScaleColumnComplete(t, db, "logs", "channel_uuid")
			requireUUIDScaleColumnComplete(t, db, "logs", "token_uuid")
			requireUUIDScaleColumnComplete(t, db, "tokens", "uuid")
			requireUUIDScaleColumnComplete(t, db, "tokens", "user_uuid")
			requireUUIDUniqueIndex(t, db, uuidBackfillTarget{table: "logs", model: &Log{}})
			requireUUIDUniqueIndex(t, db, uuidBackfillTarget{table: "tokens", model: &Token{}})

			t.Logf("T27 UUID scale backfill finished for %s: logs=%d tokens=%d duration=%s",
				dialect, logRows, tokenRows, duration)
		})
	}
}

// uuidScaleEnvInt parses an optional positive integer test setting.
// Parameters:
//   - t: test handle used for assertions.
//   - name: environment variable name to parse.
//   - fallback: default value used when the environment variable is absent.
//
// Return values:
//   - int: parsed or fallback positive integer setting.
func uuidScaleEnvInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	require.NoError(t, err, "%s must be an integer", name)
	require.Positive(t, value, "%s must be positive", name)
	return value
}

// seedUUIDScaleReferenceRows inserts the user and channel referenced by scale rows.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving the reference rows.
//
// Return values: none.
func seedUUIDScaleReferenceRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Table("users").Create(map[string]any{
		"id": 1, "username": "scale-root", "password": "password-hash",
	}).Error)
	require.NoError(t, db.Table("channels").Create(map[string]any{
		"id": 1, "type": 1, "name": "scale-channel", "models": "gpt-4o", "config": "{}",
	}).Error)
}

// seedUUIDScaleTokens inserts legacy token rows without UUID values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving token rows.
//   - tokenRows: number of legacy token rows to insert.
//
// Return values: none.
func seedUUIDScaleTokens(t *testing.T, db *gorm.DB, tokenRows int) {
	t.Helper()
	seedUUIDScaleRows(t, db, "tokens", []string{"id", "user_id", "key", "name", "status"}, tokenRows, func(id int) []any {
		name := fmt.Sprintf("scale-token-%d", id)
		return []any{id, 1, name, name, TokenStatusEnabled}
	})
}

// seedUUIDScaleLogs inserts legacy log rows without own or FK UUID values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving log rows.
//   - logRows: number of legacy log rows to insert.
//   - tokenRows: number of token names to cycle through for token UUID backfill.
//
// Return values: none.
func seedUUIDScaleLogs(t *testing.T, db *gorm.DB, logRows int, tokenRows int) {
	t.Helper()
	seedUUIDScaleRows(t, db, "logs", []string{"id", "user_id", "channel_id", "type", "token_name", "content"}, logRows, func(id int) []any {
		tokenID := ((id - 1) % tokenRows) + 1
		return []any{id, 1, 1, LogTypeConsume, fmt.Sprintf("scale-token-%d", tokenID), "scale log"}
	})
}

// seedUUIDScaleRows bulk inserts deterministic legacy rows for a scale test table.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle receiving rows.
//   - table: trusted table name to insert into.
//   - columns: trusted column names included in each inserted row.
//   - total: number of rows to insert.
//   - build: callback that returns one row's values for a given id.
//
// Return values: none.
func seedUUIDScaleRows(t *testing.T, db *gorm.DB, table string, columns []string, total int, build func(id int) []any) {
	t.Helper()
	for start := 1; start <= total; start += uuidScaleInsertBatchSize {
		end := start + uuidScaleInsertBatchSize - 1
		if end > total {
			end = total
		}
		placeholders := make([]string, 0, end-start+1)
		args := make([]any, 0, (end-start+1)*len(columns))
		for id := start; id <= end; id++ {
			values := build(id)
			require.Len(t, values, len(columns))
			placeholders = append(placeholders, "("+strings.TrimRight(strings.Repeat("?,", len(columns)), ",")+")")
			args = append(args, values...)
		}
		sql := "INSERT INTO " + quoteIdentifier(db, table) + " (" + quotedUUIDScaleColumns(db, columns) + ") VALUES " + strings.Join(placeholders, ",")
		require.NoError(t, db.Exec(sql, args...).Error, "insert %s rows %d-%d", table, start, end)
	}
}

// quotedUUIDScaleColumns returns a comma-separated list of quoted column names.
// Parameters:
//   - db: database handle whose dialect controls identifier quoting.
//   - columns: trusted column names to quote.
//
// Return values:
//   - string: comma-separated quoted column names.
func quotedUUIDScaleColumns(db *gorm.DB, columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(db, column))
	}
	return strings.Join(quoted, ",")
}

// requireUUIDScaleColumnComplete asserts that a backfilled column has no missing values.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle containing the target table.
//   - table: trusted table name to inspect.
//   - column: trusted UUID column name to inspect.
//
// Return values: none.
func requireUUIDScaleColumnComplete(t *testing.T, db *gorm.DB, table string, column string) {
	t.Helper()
	var missing int64
	err := db.Table(table).
		Where(quoteIdentifier(db, column) + " IS NULL OR " + quoteIdentifier(db, column) + " = ''").
		Count(&missing).Error
	require.NoError(t, err, "count missing %s.%s", table, column)
	require.Zero(t, missing, "%s.%s still has missing UUID values", table, column)
}
