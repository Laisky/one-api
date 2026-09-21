package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/benchdb"
)

// dashboard395Targets returns the existing CI database services when present,
// with benchmark DSNs as an alternative for local multi-engine verification.
func dashboard395Targets(t *testing.T) []benchdb.Target {
	t.Helper()
	targets := []benchdb.Target{{Engine: benchdb.EngineSQLite}}
	for _, item := range []struct {
		engine   benchdb.Engine
		ciEnv    string
		benchEnv string
	}{
		{benchdb.EnginePostgres, "PG_DSN", benchdb.EnvPostgresDSN},
		{benchdb.EngineMySQL, "MYSQL_DSN", benchdb.EnvMySQLDSN},
	} {
		dsn := strings.TrimSpace(os.Getenv(item.ciEnv))
		if dsn == "" {
			dsn = strings.TrimSpace(os.Getenv(item.benchEnv))
		}
		if dsn != "" {
			targets = append(targets, benchdb.Target{Engine: item.engine, DSN: dsn})
		}
	}
	if os.Getenv("ONEAPI_REQUIRE_DB_BACKENDS") == "1" {
		require.Len(t, targets, 3, "required native database arms must not silently skip")
	}
	return targets
}

// dashboard395Database opens a connection-local temporary logs table. The pool
// is pinned to one connection, so tests cannot read or mutate persistent logs.
// It returns the handle and restores LOG_DB and dialect flags during cleanup.
func dashboard395Database(tb testing.TB, target benchdb.Target, nocase bool) *gorm.DB {
	tb.Helper()
	db, restore := benchdb.Open(tb, target)
	sqlDB, err := db.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	collation := ""
	if nocase {
		require.Equal(tb, "sqlite", db.Dialector.Name())
		collation = " COLLATE NOCASE"
	}
	schema := `CREATE TEMPORARY TABLE logs (
		user_id BIGINT, user_uuid VARCHAR(36)%s, created_at BIGINT, type INTEGER,
		model_name VARCHAR(255)%s, username VARCHAR(255)%s, token_name VARCHAR(255)%s,
		quota BIGINT, prompt_tokens BIGINT, completion_tokens BIGINT, cached_prompt_tokens BIGINT,
		content TEXT, metadata TEXT)`
	require.NoError(tb, db.Exec(fmt.Sprintf(schema, collation, collation, collation, collation)).Error)
	require.NoError(tb, db.Exec("CREATE INDEX idx_created_at_type ON logs (created_at, type)").Error)
	require.NoError(tb, db.Exec("CREATE INDEX idx_logs_user_id ON logs (user_id)").Error)
	previous := LOG_DB
	LOG_DB = db
	tb.Cleanup(func() { LOG_DB = previous; restore() })
	return db
}

// dashboard395Rows returns adversarial records for independent old/new queries:
// UTC edges, nullable keys, renames, duplicate token names, large and negative
// quotas, provisional rows, and tool fields that must never enter consume sums.
func dashboard395Rows() []map[string]any {
	const day = int64(1767225600)
	var rows []map[string]any
	for _, userID := range []int{7, 8} {
		for _, kind := range []int{LogTypeConsume, LogTypeTool, LogTypeProvisional} {
			for i, stamp := range []int64{day - 1, day, day + 1, day + 86399, day + 86400} {
				var token any = "shared"
				var userUUID any = fmt.Sprintf("00000000-0000-4000-8000-%012d", userID)
				var modelName any = "model-or-tool"
				if i == 1 {
					token = nil
					userUUID = nil
					modelName = nil
				}
				if i == 2 {
					token = ""
					userUUID = ""
					modelName = ""
				}
				username := "same-name"
				if i == 3 {
					username = "renamed"
				}
				quota := int64(100 + i)
				cached := int64(0)
				if i%2 == 0 {
					cached = 12
					quota = 1 << 40
				}
				if i == 3 {
					quota = -23
					cached = -1
				}
				prompt := int64(31)
				if kind == LogTypeTool {
					prompt = 1 << 62
				}
				rows = append(rows, map[string]any{
					"user_id": userID, "user_uuid": userUUID, "created_at": stamp, "type": kind,
					"model_name": modelName, "username": username, "token_name": token,
					"quota": quota, "prompt_tokens": prompt, "completion_tokens": 17,
					"cached_prompt_tokens": cached,
				})
			}
		}
	}
	// NULL and empty keys on the same day must not be accidentally collapsed
	// by coalescing them before grouping. Tool-name grouping is the exception.
	for _, kind := range []int{LogTypeConsume, LogTypeTool} {
		for _, key := range []any{nil, ""} {
			rows = append(rows, map[string]any{
				"user_id": 7, "user_uuid": key, "created_at": day + 2, "type": kind,
				"model_name": key, "username": key, "token_name": key,
				"quota": nil, "prompt_tokens": nil, "completion_tokens": nil,
				"cached_prompt_tokens": nil,
			})
		}
	}
	return rows
}

// requireDashboard395Equal compares every metric and identity. SQL does not
// promise an ordering between equal sort keys, so ties are compared as multisets.
func requireDashboard395Equal(tb testing.TB, want, got *DashboardAggregates) {
	tb.Helper()
	require.ElementsMatch(tb, want.Logs, got.Logs)
	require.ElementsMatch(tb, want.UserLogs, got.UserLogs)
	require.ElementsMatch(tb, want.TokenLogs, got.TokenLogs)
	require.ElementsMatch(tb, want.ToolLogs, got.ToolLogs)
	require.ElementsMatch(tb, want.ToolUserLogs, got.ToolUserLogs)
	require.ElementsMatch(tb, want.ToolTokenLogs, got.ToolTokenLogs)
}

// TestDashboardPreaggregationEquivalent verifies all six views against the
// unchanged production query functions on SQLite and available native engines.
func TestDashboardPreaggregationEquivalent(t *testing.T) {
	for _, target := range dashboard395Targets(t) {
		t.Run(string(target.Engine), func(t *testing.T) {
			db := dashboard395Database(t, target, false)
			require.True(t, dashboardSupportsPreaggregation(db), "this arm must exercise the new query")
			if target.Engine == benchdb.EnginePostgres {
				require.NoError(t, db.Exec("SET TIME ZONE 'Asia/Tokyo'").Error)
			}
			if target.Engine == benchdb.EngineMySQL {
				require.NoError(t, db.Exec("SET time_zone = '+09:00'").Error)
			}
			rows := dashboard395Rows()
			require.NoError(t, db.Table("logs").Create(&rows).Error)
			// The LOG_DB dialect must win even in a split-engine deployment.
			pg, my, lite := common.UsingPostgreSQL.Load(), common.UsingMySQL.Load(), common.UsingSQLite.Load()
			common.UsingPostgreSQL.Store(true)
			common.UsingMySQL.Store(true)
			common.UsingSQLite.Store(true)
			t.Cleanup(func() { common.UsingPostgreSQL.Store(pg); common.UsingMySQL.Store(my); common.UsingSQLite.Store(lite) })
			for _, window := range [][2]int{{1767225600, 1767312000}, {1767225599, 1767312001}, {0, 1}, {7, 7}, {9, 7}} {
				for _, user := range []int{0, 7, 8, 404, -1} {
					want, err := searchDashboardAggregatesLegacy(context.Background(), user, window[0], window[1])
					require.NoError(t, err)
					got, err := SearchDashboardAggregatesWithContext(context.Background(), user, window[0], window[1])
					require.NoError(t, err)
					requireDashboard395Equal(t, want, got)
				}
			}
			got, err := SearchDashboardAggregatesWithContext(context.Background(), 7, 1767225600, 1767312000)
			require.NoError(t, err)
			requests := 0
			for _, row := range got.Logs {
				requests += row.RequestCount
				require.Equal(t, "2026-01-01", row.Day)
			}
			require.Equal(t, 5, requests, "three consume rows plus two nullable-key rows; no provisional or other owner")
			for _, row := range got.TokenLogs {
				require.Equal(t, 7, row.UserId)
			}
			for _, row := range got.ToolTokenLogs {
				require.Equal(t, 7, row.UserId)
			}
			// Empty bundles retain the same null/array wire shape, not just totals.
			want, err := searchDashboardAggregatesLegacy(context.Background(), 404, 0, 1)
			require.NoError(t, err)
			empty, err := SearchDashboardAggregatesWithContext(context.Background(), 404, 0, 1)
			require.NoError(t, err)
			before, err := json.Marshal(want)
			require.NoError(t, err)
			after, err := json.Marshal(empty)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// TestDashboardPreaggregationOneScan pins the deterministic performance defect:
// the baseline issues six statements; the new path issues one and its SQLite
// execution plan accesses the physical logs table once, even with no Redis.
func TestDashboardPreaggregationOneScan(t *testing.T) {
	db := dashboard395Database(t, benchdb.Target{Engine: benchdb.EngineSQLite}, false)
	rows := dashboard395Rows()
	require.NoError(t, db.Table("logs").Create(&rows).Error)
	calls := 0
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("dashboard395:count", func(tx *gorm.DB) {
		if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "from logs") {
			calls++
		}
	}))
	_, err := searchDashboardAggregatesLegacy(context.Background(), 0, 0, 2000000000)
	require.NoError(t, err)
	require.Equal(t, 6, calls)
	calls = 0
	_, err = SearchDashboardAggregatesWithContext(context.Background(), 0, 0, 2000000000)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	for _, user := range []int{0, 7} {
		query, args := dashboardAggregateQuery(db, user, 0, 2000000000)
		var plan []struct{ Detail string }
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+query, args...).Scan(&plan).Error)
		scans := 0
		for _, row := range plan {
			if strings.Contains(row.Detail, "SEARCH logs ") || strings.Contains(row.Detail, "SCAN logs") {
				scans++
			}
		}
		require.Equal(t, 1, scans, "%+v", plan)
	}
}

// TestDashboardPreaggregationCancellationAndFailure verifies a cancelled query
// or failed database cannot publish a partial or successful-looking bundle.
func TestDashboardPreaggregationCancellationAndFailure(t *testing.T) {
	db := dashboard395Database(t, benchdb.Target{Engine: benchdb.EngineSQLite}, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := SearchDashboardAggregatesWithContext(ctx, 7, 0, 1)
	require.Nil(t, got)
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("dashboard395:cancel", func(tx *gorm.DB) { cancel() }))
	got, err = SearchDashboardAggregatesWithContext(ctx, 7, 0, 1)
	require.Nil(t, got)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.Callback().Row().Remove("dashboard395:cancel"))
	failure := errors.New("injected database failure")
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("dashboard395:fail", func(tx *gorm.DB) { tx.AddError(failure) }))
	got, err = SearchDashboardAggregatesWithContext(context.Background(), 7, 0, 1)
	require.Nil(t, got)
	require.ErrorIs(t, err, failure)
}

// TestDashboardPreaggregationCollation preserves both raw model grouping and
// SQLite's binary COALESCE(tool_name) grouping under a NOCASE source column.
func TestDashboardPreaggregationCollation(t *testing.T) {
	db := dashboard395Database(t, benchdb.Target{Engine: benchdb.EngineSQLite}, true)
	var rows []map[string]any
	for _, kind := range []int{LogTypeConsume, LogTypeTool} {
		for _, name := range []string{"UPPER", "upper"} {
			rows = append(rows, map[string]any{"type": kind, "created_at": 0, "user_id": 7,
				"user_uuid": "u", "username": "alice", "token_name": "t", "model_name": name,
				"quota": 11, "prompt_tokens": 2, "completion_tokens": 1, "cached_prompt_tokens": 1})
		}
	}
	require.NoError(t, db.Table("logs").Create(&rows).Error)
	want, err := searchDashboardAggregatesLegacy(context.Background(), 7, 0, 1)
	require.NoError(t, err)
	got, err := SearchDashboardAggregatesWithContext(context.Background(), 7, 0, 1)
	require.NoError(t, err)
	require.Len(t, got.Logs, 1)
	require.Equal(t, 2, got.Logs[0].RequestCount)
	// SQL may choose either spelling as the representative of a NOCASE group.
	got.Logs[0].ModelName = strings.ToLower(got.Logs[0].ModelName)
	want.Logs[0].ModelName = strings.ToLower(want.Logs[0].ModelName)
	require.Len(t, got.ToolLogs, 2, "preaggregation must not merge the binary tool-name groups")
	requireDashboard395Equal(t, want, got)
}

// TestDashboardPreaggregationCompatibility preserves older or unrecognized
// MySQL deployments instead of issuing unsupported CTE syntax or retrying errors.
func TestDashboardPreaggregationCompatibility(t *testing.T) {
	for _, tc := range []struct {
		version   string
		supported bool
	}{
		{"8.4.0", true}, {"8.0.36", true}, {"9.0.0", true}, {"5.7.44", false},
		{"10.11.0-MariaDB", false}, {"5.5.5-10.6.0-MariaDB", false}, {"", false}, {"unknown", false},
	} {
		db, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/none", ServerVersion: tc.version, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.Equal(t, tc.supported, dashboardSupportsPreaggregation(db), tc.version)
		require.NoError(t, sqlDB.Close())
	}
}
