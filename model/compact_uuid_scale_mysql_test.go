package model

// MySQL 8.4 half of the AUTO-T25/T26 scale qualification.
//
// Split out of compact_uuid_scale_test.go to keep every file inside the proposal's 600-line
// limit (section 9.3). Everything here is MySQL-specific plumbing — the fixture generator, the
// InnoDB index-size reader, the admin statements whose result sets must be drained, and the
// EXPLAIN plan check. The protocol itself (what is measured, what is asserted) lives next door
// and is dialect-independent.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// compactScaleSeedUsersMySQL bulk-inserts the fixture rows with one recursive-CTE INSERT.
//
// MySQL has no generate_series, so a recursive CTE synthesizes the row numbers. The session's
// cte_max_recursion_depth (default 1,000) must be raised first, and the SET and the INSERT must
// run on the SAME pooled connection — issued through the pool they may land on two different
// sessions and the INSERT would fail its recursion cap — so both run inside db.Connection. The
// depth value is an integer this test computes, not request input, and is inlined because MySQL
// system-variable SET statements are not reliably preparable with placeholders.
//
// LOWER(LPAD(HEX(n), 12, '0')) produces exactly the lowercase zero-padded suffix
// compactUUIDTextFor renders for the same index; the caller verifies that equivalence on rows
// 1, 2, and N.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live MySQL handle.
//   - rows: number of users rows to insert.
//
// Return values: none.
func compactScaleSeedUsersMySQL(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()

	const insert = `
INSERT INTO users (id, uuid, username, password, access_token, aff_code, inviter_id, inviter_uuid,
                   role, status, quota, used_quota, request_count, created_at, updated_at)
WITH RECURSIVE seq (n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
SELECT n,
       CONCAT('018f0000-0000-7000-8000-', LOWER(LPAD(HEX(n), 12, '0'))),
       CONCAT('scale-user-', n),
       'x',
       LOWER(LPAD(HEX(n), 32, '0')),
       CONCAT('aff-', n),
       CASE WHEN n % 2 = 0 THEN n - 1 ELSE 0 END,
       CASE WHEN n % 2 = 0
            THEN CONCAT('018f0000-0000-7000-8000-', LOWER(LPAD(HEX(n - 1), 12, '0')))
            ELSE '' END,
       1, 1, 0, 0, 0, 0, 0
FROM seq`

	require.NoError(t, db.Connection(func(conn *gorm.DB) error {
		depth := "SET SESSION cte_max_recursion_depth = " + strconv.Itoa(rows+10)
		if err := conn.Exec(depth).Error; err != nil {
			return errors.Wrap(err, "raise cte_max_recursion_depth for the fixture build")
		}
		if err := conn.Exec(insert, rows).Error; err != nil {
			return errors.Wrapf(err, "bulk-build the %d-row users fixture", rows)
		}
		return nil
	}), "bulk-build the MySQL users fixture on one pinned connection")
}

// compactScaleAdminStatementMySQL runs one table-maintenance statement and drains its rows.
//
// ANALYZE TABLE and OPTIMIZE TABLE return a result set (Table/Op/Msg_type/Msg_text), not an OK
// packet, so they are issued through Raw().Scan into a throwaway destination rather than Exec.
// The rows are also checked: a per-table "error" status still arrives as a normal result row,
// and silently discarding it would let a failed rebuild masquerade as a steady-state
// measurement.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live MySQL handle.
//   - statement: trusted maintenance statement (no request input).
//
// Return values: none.
func compactScaleAdminStatementMySQL(t *testing.T, db *gorm.DB, statement string) {
	t.Helper()
	results := []map[string]any{}
	require.NoError(t, db.Raw(statement).Scan(&results).Error, "run %q", statement)
	require.NotEmpty(t, results, "%q must report at least one status row", statement)
	for _, row := range results {
		msgType := strings.ToLower(fmt.Sprintf("%s", row["Msg_type"]))
		require.NotEqual(t, "error", msgType, "%q reported an error row: %v", statement, row)
	}
}

// compactScaleIndexBytesMySQL reads one index's size from InnoDB's persistent statistics.
//
// MySQL has no pg_relation_size; the engine's own answer is
// mysql.innodb_index_stats stat_name='size', which is the index's page count, multiplied by
// @@innodb_page_size. HONESTY: this is a page-granular ESTIMATE refreshed by ANALYZE TABLE —
// the callers run ANALYZE first — not an exact byte count, and page granularity (16 KiB
// default) is why the MySQL ceiling is asserted only on the steady-state round, where both
// sides of a pair have just been rebuilt compactly and the estimate is at its most faithful.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live MySQL handle.
//   - name: index name, from the compile-time registry or the comparator DDL.
//
// Return values:
//   - int64: index size in bytes (pages times page size).
func compactScaleIndexBytesMySQL(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var exists, bytes int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics"+
			" WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = ?",
		name).Scan(&exists).Error)
	require.Equal(t, int64(1), exists, "index %s must exist before it can be measured", name)
	require.NoError(t, db.Raw(
		"SELECT stat_value * @@innodb_page_size FROM mysql.innodb_index_stats"+
			" WHERE database_name = DATABASE() AND table_name = 'users'"+
			" AND index_name = ? AND stat_name = 'size'",
		name).Scan(&bytes).Error)
	require.Greater(t, bytes, int64(0), "index %s reported a zero size", name)
	return bytes
}

// compactScaleCompareIndexesMySQL measures both AUTO-T26 size rounds on MySQL and asserts the
// ceiling on the steady-state round only.
//
// The methodology mirrors the PostgreSQL branch (see the DECISION note in
// compactScaleCompareIndexesPostgres): the as-built round is measured and surfaced as evidence,
// the steady-state round is the asserted gate. MySQL adds a second, independent reason the
// as-built round cannot be a gate: innodb_index_stats is a page-granular estimate (see
// compactScaleIndexBytesMySQL), and change-buffered, half-filled B-tree pages during
// incremental growth make that estimate least reliable exactly there.
//
// OPTIMIZE TABLE is the steady-state rebuild: on InnoDB it maps to "recreate + analyze",
// rebuilding the table AND every secondary index compactly — the analogue of the PostgreSQL
// branch's REINDEX. An explicit ANALYZE follows each phase so innodb_index_stats is never
// stale for a measurement.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live MySQL handle over the seeded fixture.
//   - dialect: live engine descriptor (MySQL).
//   - pairs: compact/text index pairs to measure.
//   - rows: fixture row count, for reporting.
//
// Return values: none.
func compactScaleCompareIndexesMySQL(t *testing.T, db *gorm.DB, dialect compactLiveDialect,
	pairs []compactScaleIndexPair, rows int) {
	t.Helper()
	const asBuiltPhase = "as-built (incremental, both indexes fed by one INSERT; InnoDB page estimate)"
	const steadyPhase = "steady-state (table and indexes rebuilt by OPTIMIZE TABLE; InnoDB page estimate)"

	compactScaleAdminStatementMySQL(t, db, "ANALYZE TABLE users")
	asBuilt := compactScaleMeasurePairs(t, db, dialect, pairs, asBuiltPhase, rows)

	compactScaleAdminStatementMySQL(t, db, "OPTIMIZE TABLE users")
	compactScaleAdminStatementMySQL(t, db, "ANALYZE TABLE users")
	steady := compactScaleMeasurePairs(t, db, dialect, pairs, steadyPhase, rows)

	compactScaleReportEvidence(t, pairs, asBuilt, asBuiltPhase)
	compactScaleAssertPairs(t, pairs, steady, steadyPhase)
}

// requireProbePlanUsesIndexMySQL proves one probe side rides its intended index on MySQL.
//
// EXPLAIN FORMAT=TRADITIONAL (the default) returns one row per plan table whose `key` column
// names the chosen index. The probes are single-table, so the plan must be exactly one row and
// its key must be EXACTLY one of the acceptable indexes — substring matching would also accept
// a longer index name that merely contains the intended one.
//
// Parameters:
//   - t: test handle used for assertions.
//   - db: live MySQL handle.
//   - side: probe side to verify.
//
// Return values: none.
func requireProbePlanUsesIndexMySQL(t *testing.T, db *gorm.DB, side compactProbeSide) {
	t.Helper()
	plan := []struct {
		Table *string `gorm:"column:table"`
		Type  *string `gorm:"column:type"`
		Key   *string `gorm:"column:key"`
	}{}
	require.NoError(t, db.Raw("EXPLAIN "+side.query, side.binds(0)).Scan(&plan).Error,
		"EXPLAIN the %s probe", side.label)
	require.Len(t, plan, 1, "the single-table %s probe must produce a one-row plan", side.label)
	require.NotNil(t, plan[0].Key,
		"the %s probe chose no index at all; the comparison would measure a table scan", side.label)
	require.Contains(t, side.indexes, *plan[0].Key,
		"the %s probe must use one of %v or the comparison is meaningless; it chose %s",
		side.label, side.indexes, *plan[0].Key)
}
