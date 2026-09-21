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

// dashboard395QueryCounter counts actual log reads while retaining GORM's
// standard logger interface; fixture writes and migrations are excluded.
type dashboard395QueryCounter struct {
	glogger.Interface
	reads atomic.Int64
}

// Trace records completed SQL reads; context, timing and errors are supplied by GORM.
func (l *dashboard395QueryCounter) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	query, _ := fc()
	query = strings.ToLower(query)
	if strings.Contains(query, "from logs") {
		l.reads.Add(1)
	}
}

// TestDashboard395ColdQueryBudget exercises the production uncached collector.
// Before the fix this fails with six reads; afterwards all six chart series must
// be present after exactly three reads, including a fresh repeated cold load.
func TestDashboard395ColdQueryBudget(t *testing.T) {
	counter := &dashboard395QueryCounter{Interface: glogger.Discard}
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dashboard.db"), &gorm.Config{Logger: counter})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Exec(`INSERT INTO logs (type,user_id,created_at,username,token_name,model_name,quota)
	 VALUES (2,1,1,'u','t','m',10),(7,1,1,'u','t','tool',3)`).Error)
	prev := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = prev })
	for range 2 {
		counter.reads.Store(0)
		got, err := collectDashboardAggregatesWithContext(context.Background(), 1, 0, 2)
		require.NoError(t, err)
		require.Len(t, got.Logs, 1)
		require.Len(t, got.UserLogs, 1)
		require.Len(t, got.TokenLogs, 1)
		require.Len(t, got.ToolLogs, 1)
		require.Len(t, got.ToolUserLogs, 1)
		require.Len(t, got.ToolTokenLogs, 1)
		require.EqualValues(t, 3, counter.reads.Load(), "cold dashboard must not independently scan logs for every chart")
	}
}
