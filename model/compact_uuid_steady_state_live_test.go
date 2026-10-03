package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// compactGapProbeFixtureRows is the size of the logs fixture for the live gap-probe test. It is
// large enough that walking the whole table is unmistakable next to an indexed read, and small
// enough to seed in seconds.
const compactGapProbeFixtureRows = 20000

// compactGapProbeNullRows is how many fixture rows carry no channel reference at all. Their
// legacy text is NULL, so their shadow is legitimately NULL: they are not gaps, but they are
// exactly the rows the compact index returns for `IS NULL`, and so the floor for an indexed probe.
const compactGapProbeNullRows = 2000

// compactGapProbeRowPadding pads each fixture row to production's width. b1's logs rows average
// 789 bytes, which is part of what makes random heap fetches through the compact index look
// expensive to the planner.
const compactGapProbeRowPadding = 760

// compactGapProbeBatchRows is the probe LIMIT used for the measured cycle.
//
// It is what puts a 20,000-row fixture in production's planner regime. PostgreSQL walks the
// primary key when, assuming NULL shadows and non-NULL text are independent, it expects to find
// LIMIT matches after a short prefix of id order. That happens roughly when
// nullShadowRows^2 >> LIMIT * tableRows. b1 sits about 9x past that line (22,800^2 against
// 200 * 293,556); this fixture at the default LIMIT of 200 sits exactly on it (2,000^2 against
// 200 * 20,000), and the first two versions of this test passed on the defective code because
// both planners chose the index. At LIMIT 20 the fixture is 10x past the line, like production,
// without seeding 180,000 rows. COMPACT_UUID_BATCH_SIZE is an operator setting, so the regime is
// reachable in production at any size.
const compactGapProbeBatchRows = 20

// TestCompactGapProbeCostLive reproduces the planner half of the post-completion cost on the
// engines that have planners worth the name, and pins the steady state that replaced it.
//
// Pre-fix behavior: a ready cycle probed every target, foreign keys included, with
// `WHERE <compact> IS NULL ... ORDER BY id LIMIT n`. The planner assumes a NULL shadow and non-NULL
// text are independent, expects plentiful matches, and walks the primary key through the whole
// table to find none — 273,892 rows per column on production's logs table, every idle interval.
//
// Required behavior: a ready cycle never probes a foreign-key shadow, whose legitimately NULL
// references would make even an indexed probe cost as many rows as there are NULL references;
// and each owned probe is a seek into an empty index range.
func TestCompactGapProbeCostLive(t *testing.T) {
	for _, dialect := range compactLiveDialects() {
		dialect := dialect
		t.Run(dialect.name, func(t *testing.T) {
			db, topology, ok := newLiveCompactTopology(t, dialect, false)
			if !ok {
				compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
			}
			seedCompactGapProbeFixture(t, db)

			coordinator := newCompactCoordinator(topology)
			driveCompactToReady(t, coordinator)
			requireCompactMarkersPresent(t, topology)
			analyzeCompactLiveTable(t, db, dialect, "logs")

			withCompactBatchSize(t, compactGapProbeBatchRows)
			capture := installCompactStatementCapture(t, db)
			require.Equal(t, compactStateReady, runCompactCycleForTest(t, coordinator).state)
			statements := capture.drain()

			if probe, found := findCompactGapProbe(statements, "logs", "channel_uuid_compact"); found {
				examined := measureCompactRowsExamined(t, db, dialect, probe)
				t.Fatalf("%s: a ready cycle probed the foreign-key shadow logs.channel_uuid, examining %d rows "+
					"(table %d, NULL references %d, limit %d)", dialect.name, examined,
					compactGapProbeFixtureRows, compactGapProbeNullRows, compactGapProbeBatchRows)
			}
			for _, statement := range statements {
				if isCompactGapProbe(statement.sql) && !compactProbeTargetsOwnedColumn(statement.sql) {
					t.Fatalf("%s: a ready cycle probed a foreign-key shadow: %s", dialect.name, statement.sql)
				}
			}

			probe, found := findCompactGapProbe(statements, "logs", "uuid_compact")
			require.True(t, found, "a ready cycle must probe the owned logs.uuid shadow")
			examined := measureCompactRowsExamined(t, db, dialect, probe)
			t.Logf("%s owned probe on logs.uuid examined %d rows (table %d)",
				dialect.name, examined, compactGapProbeFixtureRows)
			// A clean owned shadow has no NULLs, so the index range is empty; the bound only
			// leaves room for how each engine counts a seek.
			require.LessOrEqual(t, examined, int64(compactBatchRows(compactTarget{})),
				"the owned probe must be an index seek, not a walk of the table")
		})
	}
}

// seedCompactGapProbeFixture writes one channel and a logs table whose shadows are fully derived.
// Most rows reference the channel; a minority carry no channel reference at all.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live primary handle.
//
// Return values: none.
func seedCompactGapProbeFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	channelUUID := compactUUIDTextFor(900001)
	require.NoError(t, db.Exec(
		"INSERT INTO channels (id, type, name, models, config, uuid) VALUES (1, 1, 'probe', 'gpt-4o', '{}', ?)",
		channelUUID).Error)

	padding := strings.Repeat("x", compactGapProbeRowPadding)
	const chunk = 500
	for start := 1; start <= compactGapProbeFixtureRows; start += chunk {
		end := start + chunk - 1
		if end > compactGapProbeFixtureRows {
			end = compactGapProbeFixtureRows
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			row := map[string]any{
				"id": id, "user_id": 0, "type": 2, "content": strconv.Itoa(id) + padding,
				"uuid": compactUUIDTextFor(id), "created_at": int64(id),
			}
			// Spread the reference-less rows through the id range, as real history has them,
			// rather than clustering them where an id-ordered walk would find them first.
			if id%(compactGapProbeFixtureRows/compactGapProbeNullRows) == 0 {
				row["channel_id"] = 0
			} else {
				row["channel_id"] = 1
				row["channel_uuid"] = channelUUID
			}
			rows = append(rows, row)
		}
		require.NoError(t, db.Table("logs").Create(rows).Error)
	}
}

// withCompactBatchSize sets the compact per-statement row batch for the rest of one test.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - rows: batch size.
//
// Return values: none.
func withCompactBatchSize(t *testing.T, rows int) {
	t.Helper()
	original := config.CompactUUIDBatchSize
	config.CompactUUIDBatchSize = rows
	t.Cleanup(func() { config.CompactUUIDBatchSize = original })
}

// analyzeCompactLiveTable refreshes planner statistics so the plan under test is the plan a
// production table with current statistics gets.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - dialect: engine descriptor.
//   - table: trusted table name.
//
// Return values: none.
func analyzeCompactLiveTable(t *testing.T, db *gorm.DB, dialect compactLiveDialect, table string) {
	t.Helper()
	statement := "ANALYZE " + quoteIdentifier(db, table)
	if dialect.name == "mysql" {
		statement = "ANALYZE TABLE " + quoteIdentifier(db, table)
	}
	require.NoError(t, db.Exec(statement).Error)
}

// findCompactGapProbe returns the captured NULL-backlog probe for one table and shadow column.
// Parameters:
//   - statements: captured statements.
//   - table: trusted table name.
//   - compactColumn: shadow column the probe must target.
//
// Return values:
//   - compactStatement: the probe.
//   - bool: true when one was captured.
func findCompactGapProbe(statements []compactStatement, table string, compactColumn string) (compactStatement, bool) {
	for _, statement := range statements {
		if !isCompactGapProbe(statement.sql) {
			continue
		}
		// Match the quoted column exactly, so "uuid_compact" never matches "channel_uuid_compact".
		quotedTable := strings.Contains(statement.sql, `"`+table+`"`) || strings.Contains(statement.sql, "`"+table+"`")
		quotedColumn := strings.Contains(statement.sql, `"`+compactColumn+`" IS NULL`) ||
			strings.Contains(statement.sql, "`"+compactColumn+"` IS NULL")
		if quotedTable && quotedColumn {
			return statement, true
		}
	}
	return compactStatement{}, false
}

// measureCompactRowsExamined executes one statement and reports how many rows the engine read
// to answer it, which is the cost the probe's contract bounds.
//
// PostgreSQL reports it through EXPLAIN ANALYZE: per scan node, rows returned plus rows removed
// by filter, times loops. MySQL reports it through the session's handler counters, which count
// every row the storage engine handed to the executor.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - dialect: engine descriptor.
//   - statement: captured statement with its arguments.
//
// Return values:
//   - int64: rows the engine examined.
func measureCompactRowsExamined(t *testing.T, db *gorm.DB, dialect compactLiveDialect, statement compactStatement) int64 {
	t.Helper()
	if dialect.name == "postgres" {
		var plan string
		require.NoError(t, db.Raw("EXPLAIN (ANALYZE, FORMAT JSON) "+statement.sql, statement.vars...).Row().Scan(&plan))
		var document []map[string]any
		require.NoError(t, json.Unmarshal([]byte(plan), &document))
		require.NotEmpty(t, document)
		planNode, ok := document[0]["Plan"].(map[string]any)
		require.True(t, ok, "PostgreSQL EXPLAIN JSON did not contain an object Plan node")
		return postgresPlanRowsExamined(planNode)
	}

	var examined int64
	require.NoError(t, db.Connection(func(conn *gorm.DB) error {
		// Reading the counters itself touches rows on some versions, so the cost of a
		// statement that reads no table is measured first and subtracted.
		overhead, err := mysqlHandlerReads(conn, "SELECT 1")
		if err != nil {
			return err
		}
		total, err := mysqlHandlerReads(conn, statement.sql, statement.vars...)
		if err != nil {
			return err
		}
		examined = total - overhead
		return nil
	}))
	return examined
}

// mysqlHandlerReads runs one statement on a pinned connection and returns the session's
// storage-engine row reads for it.
// Parameters:
//   - conn: pinned connection.
//   - sql: statement to run.
//   - vars: bound arguments.
//
// Return values:
//   - int64: sum of the session's Handler_read_* counters after the statement.
//   - error: wrapped error when a statement fails.
func mysqlHandlerReads(conn *gorm.DB, sql string, vars ...any) (int64, error) {
	if err := conn.Exec("FLUSH STATUS").Error; err != nil {
		return 0, errors.Wrap(err, "flush mysql session status")
	}
	rows, err := conn.Raw(sql, vars...).Rows()
	if err != nil {
		return 0, errors.Wrap(err, "run measured statement")
	}
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Wrap(err, "iterate measured statement")
	}
	if err := rows.Close(); err != nil {
		return 0, errors.Wrap(err, "close measured statement")
	}
	counters := []struct {
		Name  string `gorm:"column:Variable_name"`
		Value string `gorm:"column:Value"`
	}{}
	if err := conn.Raw("SHOW SESSION STATUS LIKE 'Handler_read%'").Scan(&counters).Error; err != nil {
		return 0, errors.Wrap(err, "read mysql handler counters")
	}
	total := int64(0)
	for _, counter := range counters {
		value, parseErr := strconv.ParseInt(counter.Value, 10, 64)
		if parseErr != nil {
			return 0, errors.Wrapf(parseErr, "parse mysql handler counter %s", counter.Name)
		}
		total += value
	}
	return total, nil
}

// postgresPlanRowsExamined sums rows read by every scan node in a PostgreSQL JSON plan tree.
// Parameters:
//   - node: one plan node.
//
// Return values:
//   - int64: rows returned plus rows removed by filter, times loops, over every scan node.
func postgresPlanRowsExamined(node map[string]any) int64 {
	total := int64(0)
	nodeType, _ := node["Node Type"].(string)
	// A Bitmap Index Scan only builds the bitmap; its parent Bitmap Heap Scan reports every row
	// actually fetched and filtered, so counting both would charge each row twice.
	if strings.Contains(nodeType, "Scan") && nodeType != "Bitmap Index Scan" {
		loops := jsonNumber(node["Actual Loops"])
		if loops == 0 {
			loops = 1
		}
		total += (jsonNumber(node["Actual Rows"]) + jsonNumber(node["Rows Removed by Filter"]) +
			jsonNumber(node["Rows Removed by Index Recheck"])) * loops
	}
	if children, ok := node["Plans"].([]any); ok {
		for _, child := range children {
			if childNode, ok := child.(map[string]any); ok {
				total += postgresPlanRowsExamined(childNode)
			}
		}
	}
	return total
}

// jsonNumber converts a decoded JSON number to int64, treating anything else as zero.
// Parameters:
//   - value: decoded JSON value.
//
// Return values:
//   - int64: the number, or zero.
func jsonNumber(value any) int64 {
	if number, ok := value.(float64); ok {
		return int64(number)
	}
	return 0
}
