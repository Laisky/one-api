package model

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/benchdb"
)

// TestDashboard395PartialReadFailures injects query, scan and iteration failures
// into the first or final read. No partial bundle may escape, and rows must close.
func TestDashboard395PartialReadFailures(t *testing.T) {
	for _, failure := range []string{"model_query", "model_scan", "model_iteration", "token_query", "token_scan", "token_iteration"} {
		t.Run(failure, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: glogger.Discard})
			require.NoError(t, err)
			prev := LOG_DB
			LOG_DB = db
			t.Cleanup(func() {
				LOG_DB = prev
				mock.ExpectClose()
				require.NoError(t, sqlDB.Close())
				require.NoError(t, mock.ExpectationsWereMet())
			})
			models, tools, tokens, _ := dashboardAggregateQueries(1, 0, 86400)
			injected := errors.New("injected dashboard read failure")
			modelColumns := []string{"type", "day_bucket", "name", "request_count", "quota", "prompt_tokens", "completion_tokens", "cached_prompt_tokens", "cache_hit_count", "cache_hit_quota"}
			tokenColumns := []string{"by_token", "type", "day_bucket", "username", "user_id", "user_uuid", "token_name", "request_count", "quota", "prompt_tokens", "completion_tokens", "cached_prompt_tokens", "cache_hit_count", "cache_hit_quota"}
			first := mock.ExpectQuery(regexp.QuoteMeta(models)).WithArgs(0, 86400, 1)
			if failure == "model_query" {
				first.WillReturnError(injected)
			} else {
				var quota any = 11
				if failure == "model_scan" {
					quota = "invalid-integer"
				}
				rows := sqlmock.NewRows(modelColumns).AddRow(2, 0, "m", 1, quota, 2, 3, 0, 0, 0)
				if failure == "model_iteration" {
					rows.AddRow(2, 0, "m2", 1, 13, 2, 3, 0, 0, 0).RowError(1, injected)
				}
				first.WillReturnRows(rows).RowsWillBeClosed()
			}
			if strings.HasPrefix(failure, "token_") {
				mock.ExpectQuery(regexp.QuoteMeta(tools)).WithArgs(0, 86400, 1).
					WillReturnRows(sqlmock.NewRows(modelColumns)).RowsWillBeClosed()
				second := mock.ExpectQuery(regexp.QuoteMeta(tokens)).WithArgs(0, 86400, 1)
				if failure == "token_query" {
					second.WillReturnError(injected)
				} else {
					var quota any = 11
					if failure == "token_scan" {
						quota = "invalid-integer"
					}
					rows := sqlmock.NewRows(tokenColumns).AddRow(0, 2, 0, "u", 1, "uuid", nil, 1, quota, 2, 3, 0, 0, 0)
					if failure == "token_iteration" {
						rows.AddRow(1, 2, 0, "u", 1, "uuid", "t", 1, 11, 2, 3, 0, 0, 0).RowError(1, injected)
					}
					second.WillReturnRows(rows).RowsWillBeClosed()
				}
			}
			got, err := SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 86400)
			require.Error(t, err)
			require.Nil(t, got)
			if !strings.HasSuffix(failure, "_scan") {
				require.ErrorIs(t, err, injected)
			}
		})
	}
}

// TestDashboard395DatabaseCollation proves user regrouping follows the database,
// not Go's case-sensitive map keys. Exact representative letter case in a SQL
// group is unspecified, so assertions use group counts and independent totals.
func TestDashboard395DatabaseCollation(t *testing.T) {
	for _, target := range benchdb.Targets() {
		t.Run(string(target.Engine), func(t *testing.T) {
			db := openDashboard395(t, target)
			if target.Engine == benchdb.EngineSQLite {
				require.NoError(t, db.Exec("DROP TABLE logs").Error)
				require.NoError(t, db.Exec(`CREATE TABLE logs (
				 type INTEGER, created_at INTEGER, user_id INTEGER, user_uuid TEXT,
				 username TEXT COLLATE NOCASE, token_name TEXT COLLATE NOCASE,
				 model_name TEXT COLLATE NOCASE, quota INTEGER, prompt_tokens INTEGER,
				 completion_tokens INTEGER, cached_prompt_tokens INTEGER)`).Error)
			} else if target.Engine == benchdb.EngineMySQL {
				require.NoError(t, db.Exec(`ALTER TABLE logs
				 MODIFY username VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
				 MODIFY token_name VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
				 MODIFY model_name VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`).Error)
			}
			for _, logType := range []int{LogTypeConsume, LogTypeTool} {
				require.NoError(t, db.Exec(`INSERT INTO logs
				 (type,created_at,user_id,user_uuid,username,token_name,model_name,quota,prompt_tokens,completion_tokens,cached_prompt_tokens)
				 VALUES (?,1,1,'uuid','Alice','a','M',11,2,3,1),
				 (?,1,1,'uuid','alice','b','m',13,2,3,0),
				 (?,1,1,'uuid','alice','A','m',16,2,3,1)`, logType, logType, logType).Error)
			}
			got, err := SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 2)
			require.NoError(t, err)
			want, err := legacyDashboard395(context.Background(), 1, 0, 2)
			require.NoError(t, err)
			verifyDashboard395Totals(t, db, 1, 0, 2, got)
			require.Len(t, got.Logs, len(want.Logs))
			require.Len(t, got.ToolLogs, len(want.ToolLogs))
			require.Len(t, got.UserLogs, len(want.UserLogs))
			require.Len(t, got.TokenLogs, len(want.TokenLogs))
			require.Len(t, got.ToolUserLogs, len(want.ToolUserLogs))
			require.Len(t, got.ToolTokenLogs, len(want.ToolTokenLogs))
			if target.Engine != benchdb.EnginePostgres {
				require.Len(t, got.UserLogs, 1)
				require.Equal(t, 40, got.UserLogs[0].Quota)
				require.Equal(t, 2, got.UserLogs[0].CacheHitCount)
				require.Equal(t, 27, got.UserLogs[0].CacheHitQuota)
				require.Len(t, got.TokenLogs, 2)
			}
		})
	}
}

// TestDashboard395TokenCTEReuse checks that SQLite materializes token groups
// once: the two output dimensions must not cause two scans of the raw log table.
func TestDashboard395TokenCTEReuse(t *testing.T) {
	db := openDashboard395(t, benchdb.Target{Engine: benchdb.EngineSQLite})
	_, _, query, args := dashboardAggregateQueries(1, 0, 86400)
	var plan []struct{ Detail string }
	require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+query, args...).Scan(&plan).Error)
	reads := 0
	for _, step := range plan {
		if strings.Contains(step.Detail, "SEARCH logs") || strings.Contains(step.Detail, "SCAN logs") {
			reads++
		}
	}
	require.Equal(t, 1, reads, "the user/token query must read logs once, not once per UNION arm")
}
