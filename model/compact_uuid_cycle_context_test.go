package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Laisky/one-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCompactCycleFixtureReleasesContext verifies that a completed real migration
// cycle releases its database context before the surrounding test ends.
// Parameters:
//   - t: test handle used for assertions and fixture cleanup.
//
// Return values: none.
func TestCompactCycleFixtureReleasesContext(t *testing.T) {
	db, topology := newCompactTestTopology(t)
	var observed context.Context
	const callback = "task17:capture_cycle_context"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if observed == nil && tx.Statement.Table == "data_migrations" {
			observed = tx.Statement.Context
			require.NoError(t, observed.Err(), "the real marker query must receive a live context")
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	result := runCompactCycleForTest(t, newCompactCoordinator(topology))
	require.NotEmpty(t, result.state, "the real coordinator must have completed a cycle")
	require.NotNil(t, observed, "the real marker query must have run")
	require.ErrorIs(t, observed.Err(), context.Canceled,
		"completed cycles must release their timer and context without waiting for test cleanup")
	require.NoError(t, db.Exec("SELECT 1").Error, "cycle cleanup must preserve the fixture database")
}

// TestCompactFileFixtureRestoresDialect verifies file-backed SQLite fixtures restore
// the caller's dialect flags and release their pool when their test ends.
// Parameters: t owns the fixture and cleanup assertions. Return values: none.
func TestCompactFileFixtureRestoresDialect(t *testing.T) {
	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})
	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(true)
	common.UsingPostgreSQL.Store(false)
	var pool *sql.DB
	require.True(t, t.Run("fixture owner", func(t *testing.T) {
		db, _ := newCompactFileTestTopology(t)
		var err error
		pool, err = db.DB()
		require.NoError(t, err)
		require.True(t, common.UsingSQLite.Load())
		require.NoError(t, pool.PingContext(t.Context()))
	}))
	require.Zero(t, pool.Stats().OpenConnections)
	require.ErrorContains(t, pool.PingContext(context.Background()), "database is closed")
	require.False(t, common.UsingSQLite.Load(), "file fixture must restore the caller's dialect")
	require.True(t, common.UsingMySQL.Load())
	require.False(t, common.UsingPostgreSQL.Load())
}
