package model

// Execution-plan evidence for W2.4 (proposal
// docs/proposals/20260905_observability-data-tiering.md, acceptance gate G2).
//
// The plans here are collected for the statements the product actually emits:
// the builders under test are the ones the request path calls, not hand-written
// approximations of them. A plan for a rewritten query would prove nothing
// about the query that ships.
//
// The collector is opt-in. It provisions a multi-million-row table, which is
// not something an ordinary `go test ./...` should do.
//
// Run with:
//
//	ONEAPI_W24_PLANS=1 ONEAPI_W24_PLAN_OUT=/tmp/w24 \
//	ONEAPI_BENCH_PG_DSN=... ONEAPI_BENCH_MYSQL_DSN=... \
//	go test ./model/ -run TestCollectW24CursorPlans -timeout 120m -v

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
)

// planRowCount is how many usage rows the plan corpus holds.
//
// Deep history is the whole point of the exercise: at a few thousand rows every
// plan is a sequential scan and every difference disappears into noise.
const planRowCount = 2_000_000

// planHeavyUserID owns the majority of the corpus, so the per-user plans are
// collected against a skewed distribution rather than a uniform one that no
// deployment has.
const planHeavyUserID = 1

// planTailUserID owns a few hundred rows out of two million, which is the case
// a per-user access path has to earn rather than the case that is free anyway.
const planTailUserID = 2

// planQuery is one statement whose plan is evidence.
type planQuery struct {
	// name identifies the query family in the report.
	name string
	// note explains what the plan is evidence about.
	note string
	// sql is the statement, with ? placeholders.
	sql string
	// args are the bind values.
	args []any
}

// TestCollectW24CursorPlans writes per-engine execution plans for the cursor,
// count and legacy-offset query families.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCollectW24CursorPlans(t *testing.T) {
	if os.Getenv("ONEAPI_W24_PLANS") != "1" {
		t.Skip("set ONEAPI_W24_PLANS=1 to collect execution plans")
	}
	outDir := os.Getenv("ONEAPI_W24_PLAN_OUT")
	require.NotEmpty(t, outDir, "set ONEAPI_W24_PLAN_OUT to the report directory")
	require.NoError(t, os.MkdirAll(outDir, 0o755))

	for _, target := range benchdb.Targets() {
		t.Run(string(target.Engine), func(t *testing.T) {
			db, restore := benchdb.Open(t, target)
			defer restore()

			// Reuse mode re-collects against a corpus a previous run already
			// seeded, which makes iterating on the query list cheap. It first
			// drops the candidate indexes so the "before" plans really are
			// before.
			// SQLite lives in a per-test temporary directory, so there is
			// never a previous corpus to reuse; only the server engines can
			// skip seeding.
			if os.Getenv("ONEAPI_W24_PLAN_REUSE") == "1" && target.Engine != benchdb.EngineSQLite {
				dropCandidateIndexes(t, db, target.Engine)
			} else {
				seedPlanCorpus(t, db, target.Engine)
			}

			var report strings.Builder
			fmt.Fprintf(&report, "engine: %s\nrows: %d\n\n", target.Engine, planRowCount)

			// Existing index set first: the plans a deployment gets today.
			fmt.Fprintf(&report, "## Index set: as declared by the model (no W2.5 candidates)\n\n")
			writeIndexInventory(t, &report, db, target.Engine)
			for _, q := range planQueries(t, db) {
				writePlan(t, &report, db, target.Engine, q)
			}

			// Then the candidate keyset indexes, so their cost is attributable.
			addCandidateIndexes(t, db, target.Engine)
			fmt.Fprintf(&report, "## Index set: with candidate (created_at, id) and (user_id, created_at, id)\n\n")
			writeIndexInventory(t, &report, db, target.Engine)
			for _, q := range planQueries(t, db) {
				writePlan(t, &report, db, target.Engine, q)
			}

			path := filepath.Join(outDir, fmt.Sprintf("plans-%s.txt", target.Engine))
			require.NoError(t, os.WriteFile(path, []byte(report.String()), 0o644))
			t.Logf("wrote %s (%d bytes)", path, report.Len())
		})
	}
}

// seedPlanCorpus creates and fills the logs table for plan collection.
//
// Parameters:
//   - t: the test handle.
//   - db: the target handle.
//   - engine: the engine being seeded.
//
// Return values: none.
func seedPlanCorpus(t *testing.T, db *gorm.DB, engine benchdb.Engine) {
	t.Helper()

	require.NoError(t, db.Exec("DROP TABLE IF EXISTS logs").Error)
	require.NoError(t, db.AutoMigrate(&Log{}))

	// A deterministic generator keeps the corpus reproducible across runs, so a
	// plan difference is attributable to the change under test.
	rng := rand.New(rand.NewSource(20260906))
	models := []string{"gpt-4.1", "gpt-4.1-mini", "claude-sonnet-5", "gemini-3-pro", "deepseek-v4"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

	// The Log row has ~28 columns, so the insert batch is sized to stay under
	// the tightest placeholder limit of the three engines (SQLite's 32766).
	const batch = 5000
	const insertBatch = 500
	start := time.Now()
	for offset := 0; offset < planRowCount; offset += batch {
		rows := make([]*Log, 0, batch)
		for i := 0; i < batch && offset+i < planRowCount; i++ {
			n := offset + i
			userID := planHeavyUserID
			if rng.Intn(100) >= 70 {
				// A long tail of 5000 other users holds the remaining 30%.
				userID = 2 + rng.Intn(5000)
			}
			logType := LogTypeConsume
			switch r := rng.Intn(100); {
			case r < 2:
				logType = LogTypeTopup
			case r < 4:
				logType = LogTypeProvisional
			case r < 6:
				logType = LogTypeManage
			}
			rows = append(rows, &Log{
				UserId: userID,
				// Roughly 25 rows share each second, so the tie-breaker arm is
				// exercised rather than being trivially empty.
				CreatedAt: base + int64(n/25),
				Type:      logType,
				Username:  fmt.Sprintf("user-%d", userID),
				TokenName: fmt.Sprintf("token-%d", rng.Intn(20)),
				ModelName: models[rng.Intn(len(models))],
				ChannelId: 1 + rng.Intn(50),
				Quota:     rng.Intn(100000),
				Content:   fmt.Sprintf("request %d completed", n),
			})
		}
		require.NoError(t, db.CreateInBatches(rows, insertBatch).Error)
	}
	t.Logf("seeded %d rows in %s", planRowCount, time.Since(start).Round(time.Second))

	// Plans are only meaningful against current statistics.
	switch engine {
	case benchdb.EnginePostgres:
		require.NoError(t, db.Exec("VACUUM ANALYZE logs").Error)
	case benchdb.EngineMySQL:
		require.NoError(t, db.Exec("ANALYZE TABLE logs").Error)
	default:
		require.NoError(t, db.Exec("ANALYZE").Error)
	}
}

// dropCandidateIndexes removes the W2.5 candidate indexes if a previous run
// left them behind.
//
// Parameters:
//   - t: the test handle.
//   - db: the target handle.
//   - engine: the engine being altered.
//
// Return values: none.
func dropCandidateIndexes(t *testing.T, db *gorm.DB, engine benchdb.Engine) {
	t.Helper()

	for _, name := range []string{"idx_logs_created_at_id", "idx_logs_user_created_at_id"} {
		stmt := "DROP INDEX IF EXISTS " + name
		if engine == benchdb.EngineMySQL {
			// MySQL has no IF EXISTS for DROP INDEX, so a missing index is an
			// expected, ignorable error rather than a failure.
			stmt = "DROP INDEX " + name + " ON logs"
			if err := db.Exec(stmt).Error; err != nil {
				t.Logf("%s: %v", stmt, err)
			}
			continue
		}
		require.NoError(t, db.Exec(stmt).Error, stmt)
	}

	switch engine {
	case benchdb.EnginePostgres:
		require.NoError(t, db.Exec("ANALYZE logs").Error)
	case benchdb.EngineMySQL:
		require.NoError(t, db.Exec("ANALYZE TABLE logs").Error)
	default:
		require.NoError(t, db.Exec("ANALYZE").Error)
	}
}

// addCandidateIndexes creates the W2.5 candidate keyset indexes.
//
// Parameters:
//   - t: the test handle.
//   - db: the target handle.
//   - engine: the engine being altered.
//
// Return values: none.
func addCandidateIndexes(t *testing.T, db *gorm.DB, engine benchdb.Engine) {
	t.Helper()

	for _, ddl := range []string{
		"CREATE INDEX idx_logs_created_at_id ON logs (created_at, id)",
		"CREATE INDEX idx_logs_user_created_at_id ON logs (user_id, created_at, id)",
	} {
		start := time.Now()
		require.NoError(t, db.Exec(ddl).Error, ddl)
		t.Logf("%s took %s", ddl, time.Since(start).Round(time.Millisecond))
	}

	switch engine {
	case benchdb.EnginePostgres:
		require.NoError(t, db.Exec("ANALYZE logs").Error)
	case benchdb.EngineMySQL:
		require.NoError(t, db.Exec("ANALYZE TABLE logs").Error)
	default:
		require.NoError(t, db.Exec("ANALYZE").Error)
	}
}

// planQueries returns the statements whose plans are collected.
//
// Every cursor and count statement is produced by the shipping builder, so the
// evidence describes the shipping query.
//
// Parameters:
//   - t: the test handle.
//   - db: the target handle, for dialect-dependent builders.
//
// Return values:
//   - []planQuery: the statements to explain.
func planQueries(t *testing.T, db *gorm.DB) []planQuery {
	t.Helper()

	allScope := LogListScope{Kind: LogListScopeAll, PrincipalUserID: 1, Role: 100}
	selfScope := LogListScope{Kind: LogListScopeSelf, SubjectUserID: planHeavyUserID, PrincipalUserID: planHeavyUserID}

	empty := LogListFilter{}
	typed := LogListFilter{LogType: LogTypeConsume}
	modelFiltered := LogListFilter{ModelName: "claude-sonnet-5"}

	// An anchor deep in the corpus: the page a user reaches after a long walk,
	// which is exactly where offset pagination collapses.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	deepAnchor := &LogCursorAnchor{CreatedAt: base + int64(planRowCount/25) - 4000, ID: planRowCount - 100_000}

	queries := []planQuery{}

	add := func(name, note string, sql string, args []any) {
		queries = append(queries, planQuery{name: name, note: note, sql: sql, args: args})
	}

	sql, args := buildLogCursorStatement(allScope, empty.Normalize(allScope), nil, 20)
	add("cursor/all/first-page", "site-wide keyset entry page", sql, args)

	sql, args = buildLogCursorStatement(allScope, empty.Normalize(allScope), deepAnchor, 20)
	add("cursor/all/deep-anchor", "site-wide keyset page ~100k rows deep", sql, args)

	sql, args = buildLogCursorStatement(allScope, typed.Normalize(allScope), deepAnchor, 20)
	add("cursor/all/deep-anchor+type", "type-filtered deep keyset page", sql, args)

	sql, args = buildLogCursorStatement(allScope, modelFiltered.Normalize(allScope), deepAnchor, 20)
	add("cursor/all/deep-anchor+model", "unindexed-detail-filter deep keyset page", sql, args)

	sql, args = buildLogCursorStatement(selfScope, empty.Normalize(selfScope), nil, 20)
	add("cursor/self/first-page", "authorized-user keyset entry page", sql, args)

	sql, args = buildLogCursorStatement(selfScope, empty.Normalize(selfScope), deepAnchor, 20)
	add("cursor/self/deep-anchor", "authorized-user deep keyset page, heavy user (~70% of rows)", sql, args)

	// The heavy user is the easy case: a user_id filter that rejects almost
	// nothing is nearly free. A tail user owning a few hundred rows out of two
	// million is what a per-user access path actually has to earn, so it gets
	// its own plan.
	tailScope := LogListScope{Kind: LogListScopeSelf, SubjectUserID: planTailUserID, PrincipalUserID: planTailUserID}
	sql, args = buildLogCursorStatement(tailScope, empty.Normalize(tailScope), nil, 20)
	add("cursor/self-tail/first-page", "tail-user keyset entry page (~0.01% of rows)", sql, args)

	sql, args = buildLogCursorStatement(tailScope, empty.Normalize(tailScope), deepAnchor, 20)
	add("cursor/self-tail/deep-anchor", "tail-user deep keyset page (~0.01% of rows)", sql, args)

	where, targs := BuildLogListPredicate(tailScope, empty.Normalize(tailScope))
	add("count/self-tail/probe-10001", "bounded count probe, tail user",
		buildLogCountProbeStatement(db, where, 10000, 0), targs)

	where, pargs := BuildLogListPredicate(allScope, empty.Normalize(allScope))
	add("count/all/probe-10001", "bounded count probe, site-wide",
		buildLogCountProbeStatement(db, where, 10000, 0), pargs)

	where, pargs = BuildLogListPredicate(selfScope, empty.Normalize(selfScope))
	add("count/self/probe-10001", "bounded count probe, authorized user",
		buildLogCountProbeStatement(db, where, 10000, 0), pargs)

	// The legacy baseline the cursor replaces. It is the same row selection
	// reached by offset, so the two plans are directly comparable.
	add("legacy/all/offset-0", "legacy offset page 1 (baseline)",
		"SELECT "+logCursorColumns+" FROM logs WHERE type <> ? ORDER BY id DESC LIMIT 20 OFFSET 0",
		[]any{LogTypeProvisional})
	add("legacy/all/offset-100000", "legacy offset page 5001 (baseline)",
		"SELECT "+logCursorColumns+" FROM logs WHERE type <> ? ORDER BY id DESC LIMIT 20 OFFSET 100000",
		[]any{LogTypeProvisional})
	add("legacy/all/count", "legacy unbounded exact total (baseline)",
		"SELECT count(*) FROM logs WHERE type <> ?", []any{LogTypeProvisional})

	return queries
}

// writePlan explains one statement and appends the result to the report.
//
// Parameters:
//   - t: the test handle.
//   - report: the report being built.
//   - db: the target handle.
//   - engine: the engine being explained.
//   - q: the statement.
//
// Return values: none.
func writePlan(t *testing.T, report *strings.Builder, db *gorm.DB, engine benchdb.Engine, q planQuery) {
	t.Helper()

	prefix := ""
	switch engine {
	case benchdb.EnginePostgres:
		prefix = "EXPLAIN (ANALYZE, BUFFERS, TIMING, SUMMARY) "
	case benchdb.EngineMySQL:
		prefix = "EXPLAIN ANALYZE "
	default:
		prefix = "EXPLAIN QUERY PLAN "
	}

	// Wall-clock is measured separately: an EXPLAIN ANALYZE timing includes
	// instrumentation overhead that a served request does not pay.
	start := time.Now()
	require.NoError(t, db.Exec(q.sql, q.args...).Error, q.name)
	elapsed := time.Since(start)

	rows, err := db.Raw(prefix+q.sql, q.args...).Rows()
	require.NoError(t, err, q.name)
	defer rows.Close()

	fmt.Fprintf(report, "### %s\n%s\nexecuted in: %s\n\n```\n", q.name, q.note, elapsed.Round(time.Microsecond))
	columns, err := rows.Columns()
	require.NoError(t, err)
	for rows.Next() {
		cells := make([]any, len(columns))
		holders := make([]any, len(columns))
		for i := range cells {
			holders[i] = &cells[i]
		}
		require.NoError(t, rows.Scan(holders...))
		parts := make([]string, 0, len(cells))
		for _, cell := range cells {
			parts = append(parts, fmt.Sprintf("%v", normalizePlanCell(cell)))
		}
		fmt.Fprintf(report, "%s\n", strings.Join(parts, " | "))
	}
	require.NoError(t, rows.Err())
	fmt.Fprintf(report, "```\n\n")
}

// normalizePlanCell renders a plan cell as readable text.
//
// Parameters:
//   - cell: the scanned value.
//
// Return values:
//   - any: the value, with byte slices decoded as text.
func normalizePlanCell(cell any) any {
	if b, ok := cell.([]byte); ok {
		return string(b)
	}
	return cell
}

// writeIndexInventory records the indexes present on logs.
//
// The plans are only interpretable against the index set that produced them.
//
// Parameters:
//   - t: the test handle.
//   - report: the report being built.
//   - db: the target handle.
//   - engine: the engine being inspected.
//
// Return values: none.
func writeIndexInventory(t *testing.T, report *strings.Builder, db *gorm.DB, engine benchdb.Engine) {
	t.Helper()

	var query string
	switch engine {
	case benchdb.EnginePostgres:
		query = "SELECT indexname, pg_size_pretty(pg_relation_size(indexname::regclass)) FROM pg_indexes WHERE tablename = 'logs' ORDER BY indexname"
	case benchdb.EngineMySQL:
		query = "SELECT index_name, GROUP_CONCAT(column_name ORDER BY seq_in_index) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'logs' GROUP BY index_name ORDER BY index_name"
	default:
		query = "SELECT name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'index' AND tbl_name = 'logs' ORDER BY name"
	}

	rows, err := db.Raw(query).Rows()
	require.NoError(t, err)
	defer rows.Close()

	fmt.Fprintf(report, "```\n")
	for rows.Next() {
		var name, detail any
		require.NoError(t, rows.Scan(&name, &detail))
		fmt.Fprintf(report, "%v | %v\n", normalizePlanCell(name), normalizePlanCell(detail))
	}
	require.NoError(t, rows.Err())
	fmt.Fprintf(report, "```\n\n")
}
