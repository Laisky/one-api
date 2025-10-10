package model

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
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

	originalMySQL := common.UsingMySQL
	originalSQLite := common.UsingSQLite
	originalPostgres := common.UsingPostgreSQL
	common.UsingMySQL = true
	common.UsingSQLite = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		DB = originalDB
		common.UsingMySQL = originalMySQL
		common.UsingSQLite = originalSQLite
		common.UsingPostgreSQL = originalPostgres
	})

	return mock, func() error {
		return sqlDB.Close()
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

	mock.ExpectQuery(`SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS count FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT request_id, MAX\(updated_at\) as max_marker FROM .*user_request_costs.*`).
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "max_marker"}))

	mock.ExpectExec(`DELETE FROM user_request_costs WHERE CHAR_LENGTH\(request_id\) > 32`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(`SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))

	mock.ExpectExec(`ALTER TABLE user_request_costs MODIFY request_id VARCHAR\(32\) NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))

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

	mock.ExpectQuery(`SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("user_request_costs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("varchar"))

	err := MigrateUserRequestCostEnsureUniqueRequestID()
	require.NoError(t, err)
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}
