package model

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

func setupMySQLMockDB(t *testing.T) (sqlmock.Sqlmock, func() error) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	mock.MatchExpectationsInOrder(false)

	dialector := mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	})

	gdb, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	originalDB := DB
	DB = gdb

	originalMySQL := common.UsingMySQL.Load()
	originalSQLite := common.UsingSQLite.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingMySQL.Store(true)
	common.UsingSQLite.Store(false)
	common.UsingPostgreSQL.Store(false)

	t.Cleanup(func() {
		DB = originalDB
		common.UsingMySQL.Store(originalMySQL)
		common.UsingSQLite.Store(originalSQLite)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	return mock, func() error {
		return sqlDB.Close()
	}
}

// setupSQLiteCostDB replaces the global DB with an in-memory SQLite instance for migration tests.
// It returns a cleanup function that restores the original DB and driver flags.
func setupSQLiteCostDB(t *testing.T) func() {
	t.Helper()

	dialector := sqlite.Open("file::memory:?cache=shared")
	gdb, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gdb.DB()
	require.NoError(t, err)

	originalDB := DB
	DB = gdb

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)

	return func() {
		DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
		_ = sqlDB.Close()
	}
}

func TestMigrateUserRequestCostEnsureUniqueRequestIDMySQLTextColumn(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.tables WHERE table_schema = DATABASE\(\) AND table_name = \?`).
		WithArgs("user_request_costs").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.statistics WHERE table_schema = DATABASE\(\) AND table_name = \? AND index_name = \?`).
		WithArgs("user_request_costs", "idx_user_request_costs_request_id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT .*FROM user_request_costs AS stale JOIN \(SELECT request_id, MAX\(updated_at\) as max_marker FROM .*user_request_costs.* GROUP BY .*request_id.*\) AS keep ON keep\.request_id = stale\.request_id WHERE stale\.updated_at < keep\.max_marker`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectExec(`DELETE FROM user_request_costs WHERE CHAR_LENGTH\(request_id\) > 32`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(`SELECT DATA_TYPE AS data_type, CHARACTER_MAXIMUM_LENGTH AS character_maximum_length FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type", "character_maximum_length"}).AddRow("text", nil))

	mock.ExpectExec(`ALTER TABLE user_request_costs MODIFY request_id VARCHAR\(32\) NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.statistics WHERE table_schema = DATABASE\(\) AND table_name = \? AND index_name = \?`).
		WithArgs("user_request_costs", "idx_user_request_costs_request_id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectExec(`ALTER TABLE user_request_costs ADD UNIQUE INDEX idx_user_request_costs_request_id \(request_id\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := MigrateUserRequestCostEnsureUniqueRequestID()
	require.NoError(t, err)
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrateUserRequestCostEnsureUniqueRequestIDMySQLNoChanges(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.tables WHERE table_schema = DATABASE\(\) AND table_name = \?`).
		WithArgs("user_request_costs").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.statistics WHERE table_schema = DATABASE\(\) AND table_name = \? AND index_name = \?`).
		WithArgs("user_request_costs", "idx_user_request_costs_request_id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := MigrateUserRequestCostEnsureUniqueRequestID()
	require.NoError(t, err)
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrateUserRequestCostEnsureUniqueRequestIDLargeDataset(t *testing.T) {
	cleanup := setupSQLiteCostDB(t)
	defer cleanup()

	require.NoError(t, DB.Exec("CREATE TABLE user_request_costs (id INTEGER PRIMARY KEY AUTOINCREMENT, request_id TEXT NOT NULL, updated_at INTEGER NOT NULL)").Error)

	const totalRows = 1500
	for i := range totalRows {
		requestID := fmt.Sprintf("req-%d", i/3)
		require.NoError(t, DB.Exec("INSERT INTO user_request_costs (request_id, updated_at) VALUES (?, ?)", requestID, i).Error)
	}

	require.NoError(t, MigrateUserRequestCostEnsureUniqueRequestID())

	var count int64
	require.NoError(t, DB.Table("user_request_costs").Count(&count).Error)
	require.Equal(t, int64(totalRows/3), count)

	type row struct {
		RequestID string
		Updated   int64 `gorm:"column:updated_at"`
	}

	var rows []row
	require.NoError(t, DB.Table("user_request_costs").Select("request_id, updated_at").Order("request_id").Scan(&rows).Error)
	require.Len(t, rows, totalRows/3)

	seen := make(map[int]struct{}, totalRows/3)
	for _, r := range rows {
		idxStr := strings.TrimPrefix(r.RequestID, "req-")
		idx, err := strconv.Atoi(idxStr)
		require.NoError(t, err)
		require.Less(t, idx, totalRows/3)
		require.Equal(t, int64(idx*3+2), r.Updated)
		_, exists := seen[idx]
		require.False(t, exists)
		seen[idx] = struct{}{}
	}
	require.Len(t, seen, totalRows/3)

	require.True(t, DB.Migrator().HasIndex(&UserRequestCost{}, "idx_user_request_costs_request_id"))
}

func TestMigrateUserRequestCostEnsureUniqueRequestIDWithoutIDColumn(t *testing.T) {
	cleanup := setupSQLiteCostDB(t)
	defer cleanup()

	require.NoError(t, DB.Exec("CREATE TABLE user_request_costs (request_id TEXT NOT NULL, updated_at INTEGER NOT NULL)").Error)
	hasID, err := userRequestCostHasIDColumn()
	require.NoError(t, err)
	require.False(t, hasID)

	insert := func(requestID string, updatedAt int) {
		require.NoError(t, DB.Exec("INSERT INTO user_request_costs (request_id, updated_at) VALUES (?, ?)", requestID, updatedAt).Error)
	}

	for i := range 4 {
		insert("req-a", i)
	}

	for i := range 3 {
		insert("req-b", 100+i)
	}

	require.NoError(t, MigrateUserRequestCostEnsureUniqueRequestID())

	var count int64
	require.NoError(t, DB.Table("user_request_costs").Count(&count).Error)
	require.Equal(t, int64(2), count)

	type row struct {
		RequestID string
		Updated   int64 `gorm:"column:updated_at"`
	}

	var rows []row
	require.NoError(t, DB.Table("user_request_costs").Select("request_id, updated_at").Order("request_id").Scan(&rows).Error)
	require.Len(t, rows, 2)

	require.Equal(t, "req-a", rows[0].RequestID)
	require.Equal(t, int64(3), rows[0].Updated)

	require.Equal(t, "req-b", rows[1].RequestID)
	require.Equal(t, int64(102), rows[1].Updated)

	require.True(t, DB.Migrator().HasIndex(&UserRequestCost{}, "idx_user_request_costs_request_id"))
}

func TestMigrateUserRequestCostEnsureUniqueRequestIDMySQLAlterPermissionError(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.tables WHERE table_schema = DATABASE\(\) AND table_name = \?`).
		WithArgs("user_request_costs").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.statistics WHERE table_schema = DATABASE\(\) AND table_name = \? AND index_name = \?`).
		WithArgs("user_request_costs", "idx_user_request_costs_request_id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT .*FROM user_request_costs AS stale JOIN \(SELECT request_id, MAX\(updated_at\) as max_marker FROM .*user_request_costs.* GROUP BY .*request_id.*\) AS keep ON keep\.request_id = stale\.request_id WHERE stale\.updated_at < keep\.max_marker`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectExec(`DELETE FROM user_request_costs WHERE CHAR_LENGTH\(request_id\) > 32`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(`SELECT DATA_TYPE AS data_type, CHARACTER_MAXIMUM_LENGTH AS character_maximum_length FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type", "character_maximum_length"}).AddRow("text", nil))

	mock.ExpectExec(`ALTER TABLE user_request_costs MODIFY request_id VARCHAR\(32\) NOT NULL`).
		WillReturnError(fmt.Errorf("permission denied"))

	err := MigrateUserRequestCostEnsureUniqueRequestID()
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}
