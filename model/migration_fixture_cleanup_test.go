package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

// TestMigrationSQLiteFixtureReleasesResources verifies a real SQLite fixture closes
// its pool and restores the caller's dialect flags when the owning test ends.
// Parameters: t owns the outer assertions and fallback cleanup. Returns: no values.
func TestMigrationSQLiteFixtureReleasesResources(t *testing.T) {
	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	var pool *sql.DB
	t.Run("pool lifetime", func(t *testing.T) {
		require.True(t, t.Run("fixture owner", func(t *testing.T) {
			db := setupMigrationTestDB(t)
			var err error
			pool, err = db.DB()
			require.NoError(t, err)
			require.NoError(t, pool.PingContext(context.Background()))
			require.Positive(t, pool.Stats().OpenConnections)
			require.NoError(t, db.Exec("CREATE TABLE fixture_lifetime (id INTEGER PRIMARY KEY)").Error)
		}))
		// Release any leaked fixture after recording RED evidence. Close is idempotent.
		t.Cleanup(func() { require.NoError(t, pool.Close()) })
		require.Zero(t, pool.Stats().OpenConnections, "completed fixture must release its real SQLite connection")
		require.ErrorContains(t, pool.PingContext(context.Background()), "database is closed")
	})

	t.Run("dialect restoration", func(t *testing.T) {
		common.UsingSQLite.Store(false)
		common.UsingMySQL.Store(true)
		common.UsingPostgreSQL.Store(false)
		require.True(t, t.Run("fixture owner", func(t *testing.T) {
			setupMigrationTestDB(t)
			require.True(t, common.UsingSQLite.Load())
			require.False(t, common.UsingMySQL.Load())
			require.False(t, common.UsingPostgreSQL.Load())
		}))
		require.False(t, common.UsingSQLite.Load(), "completed fixture must restore the original SQLite flag")
		require.True(t, common.UsingMySQL.Load(), "completed fixture must restore the original MySQL flag")
		require.False(t, common.UsingPostgreSQL.Load())
	})
}
