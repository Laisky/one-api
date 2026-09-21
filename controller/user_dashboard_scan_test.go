package controller

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/model"
)

// TestDashboardCacheMissUsesOneLogScan exercises the actual resolver and
// collector with Redis disabled. It fails if the optimized model function is
// benchmarked but not wired into the dashboard served to users.
func TestDashboardCacheMissUsesOneLogScan(t *testing.T) {
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)
	t.Cleanup(SetDashboardAggregateLifecycleContext(context.Background()))
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dashboard.db"), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	require.NoError(t, db.Exec(`CREATE TABLE logs (
		user_id INTEGER, user_uuid TEXT, created_at BIGINT, type INTEGER,
		model_name TEXT, username TEXT, token_name TEXT, quota BIGINT,
		prompt_tokens INTEGER, completion_tokens INTEGER, cached_prompt_tokens INTEGER)`).Error)
	for _, user := range []int{395, 396} {
		for _, kind := range []int{model.LogTypeConsume, model.LogTypeTool} {
			row := map[string]any{"user_id": user, "user_uuid": "public-user-uuid", "created_at": 1767225600,
				"type": kind, "model_name": "test-model-or-tool", "username": "owner", "token_name": "shared",
				"quota": 97, "prompt_tokens": 31, "completion_tokens": 17, "cached_prompt_tokens": 12}
			require.NoError(t, db.Table("logs").Create(&row).Error)
		}
	}
	var calls atomic.Int32
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("dashboard395:requests", func(tx *gorm.DB) {
		if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "from logs") {
			calls.Add(1)
		}
	}))
	got, err := resolveDashboardAggregates(context.Background(), 395, 1767225600, 1767312000)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load(), "one cold dashboard must not execute six raw-log queries")
	require.Len(t, got.Logs, 1)
	require.Len(t, got.UserLogs, 1)
	require.Len(t, got.TokenLogs, 1)
	require.Len(t, got.ToolLogs, 1)
	require.Len(t, got.ToolUserLogs, 1)
	require.Len(t, got.ToolTokenLogs, 1)
	require.Equal(t, 1, got.Logs[0].RequestCount)
	require.Equal(t, 395, got.TokenLogs[0].UserId)
	require.Equal(t, 395, got.ToolTokenLogs[0].UserId)
	require.Equal(t, 97, got.Logs[0].CacheHitQuota)
	require.EqualValues(t, 97, got.ToolLogs[0].Quota)
}
