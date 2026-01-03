package model

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
)

func TestParseLegacySuspendUntil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  []byte
		ok     bool
		expect time.Time
	}{
		{
			name:   "microseconds",
			input:  []byte("1700000000000000"),
			ok:     true,
			expect: time.UnixMicro(1700000000000000),
		},
		{
			name:   "milliseconds",
			input:  []byte("1700000000000"),
			ok:     true,
			expect: time.UnixMilli(1700000000000),
		},
		{
			name:   "seconds",
			input:  []byte("1700000000"),
			ok:     true,
			expect: time.Unix(1700000000, 0),
		},
		{
			name:   "iso",
			input:  []byte("2024-01-02 03:04:05"),
			ok:     true,
			expect: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			name:   "iso-with-zone",
			input:  []byte("2024-01-02T03:04:05+02:00"),
			ok:     true,
			expect: time.Date(2024, 1, 2, 1, 4, 5, 0, time.UTC),
		},
		{
			name:  "blank",
			input: []byte("   \n"),
			ok:    false,
		},
		{
			name:  "json-chunk",
			input: []byte("{\"time\":\"2024-01-01\"}"),
			ok:    false,
		},
		{
			name:  "invalid",
			input: []byte("not-a-time"),
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseLegacySuspendUntil(tt.input)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.WithinDuration(t, tt.expect, got, time.Second)
			}
		})
	}
}

func TestNormalizeAbilitySuspendUntilValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	require.NoError(t, DB.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, suspend_until TEXT)").Error)

	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g1", "m1", 1, "1700000000000").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g2", "m2", 2, "2024-01-02 03:04:05").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g3", "m3", 3, "not-a-time").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g4", "m4", 4, " \t ").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g5", "m5", 5, "2024-01-02T03:04:05+02:00").Error)

	require.NoError(t, normalizeAbilitySuspendUntilValues())

	type row struct {
		Suspend sql.NullString `gorm:"column:suspend_until"`
	}

	var r1 row
	require.NoError(t, DB.Table("abilities").Select("suspend_until").Where("model = ?", "m1").Scan(&r1).Error)
	require.True(t, r1.Suspend.Valid)
	expected := parseExpected("1700000000000")
	require.Equal(t, expected.UTC().Format("2006-01-02 15:04:05"), r1.Suspend.String)

	var r2 row
	require.NoError(t, DB.Table("abilities").Select("suspend_until").Where("model = ?", "m2").Scan(&r2).Error)
	require.True(t, r2.Suspend.Valid)
	require.Equal(t, "2024-01-02 03:04:05", r2.Suspend.String)

	var r3 row
	require.NoError(t, DB.Table("abilities").Select("suspend_until").Where("model = ?", "m3").Scan(&r3).Error)
	require.False(t, r3.Suspend.Valid)

	var r4 row
	require.NoError(t, DB.Table("abilities").Select("suspend_until").Where("model = ?", "m4").Scan(&r4).Error)
	require.False(t, r4.Suspend.Valid)

	var r5 row
	require.NoError(t, DB.Table("abilities").Select("suspend_until").Where("model = ?", "m5").Scan(&r5).Error)
	require.True(t, r5.Suspend.Valid)
	require.Equal(t, "2024-01-02 01:04:05", r5.Suspend.String)
}

// TestMysqlColumnDataType ensures MySQL metadata queries scan into strings without type mismatches.
func TestMysqlColumnDataType(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	mock.ExpectQuery(`SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? AND column_name = \?`).
		WithArgs("abilities", "suspend_until").
		WillReturnRows(sqlmock.NewRows([]string{"DATA_TYPE"}).AddRow([]byte("TIMESTAMP")))

	dataType, err := mysqlColumnDataType("abilities", "suspend_until")
	require.NoError(t, err)
	require.Equal(t, "timestamp", dataType)

	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}

func parseExpected(raw string) time.Time {
	t, _ := parseLegacySuspendUntil([]byte(raw))
	return t
}

func TestMigrateAbilitySuspendUntilColumnSQLiteIntegration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	require.NoError(t, DB.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, suspend_until TEXT)").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g1", "m1", 1, "1700000000000").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g1", "m2", 2, "").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g2", "m3", 3, "2024-01-02T03:04:05Z").Error)
	require.NoError(t, DB.Exec("INSERT INTO abilities (`group`, model, channel_id, suspend_until) VALUES (?,?,?,?)",
		"g2", "m4", 4, "garbage").Error)

	require.NoError(t, MigrateAbilitySuspendUntilColumn())

	type row struct {
		Model  string
		Parsed sql.NullString `gorm:"column:suspend_until"`
	}

	var rows []row
	require.NoError(t, DB.Table("abilities").Select("model, suspend_until").Order("model").Scan(&rows).Error)
	require.Len(t, rows, 4)

	require.Equal(t, "m1", rows[0].Model)
	require.True(t, rows[0].Parsed.Valid)
	require.Equal(t, "2023-11-14 22:13:20", rows[0].Parsed.String)

	require.Equal(t, "m2", rows[1].Model)
	require.True(t, rows[1].Parsed.Valid)
	require.Equal(t, "", rows[1].Parsed.String)

	require.Equal(t, "m3", rows[2].Model)
	require.True(t, rows[2].Parsed.Valid)
	require.Equal(t, "2024-01-02 03:04:05", rows[2].Parsed.String)

	require.Equal(t, "m4", rows[3].Model)
	require.False(t, rows[3].Parsed.Valid)
}

func TestMigrateAbilitySuspendUntilColumnWithoutColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	// Simulate an older schema where suspend_until column does not exist yet.
	require.NoError(t, DB.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER)").Error)

	require.NoError(t, MigrateAbilitySuspendUntilColumn())
	require.True(t, DB.Migrator().HasColumn(&Ability{}, "suspend_until"))
}

func TestAbilitySuspendUntilColumnExistsSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	exists, err := abilitySuspendUntilColumnExists()
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, DB.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER)").Error)
	exists, err = abilitySuspendUntilColumnExists()
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, DB.Exec("ALTER TABLE abilities ADD COLUMN suspend_until TEXT").Error)
	exists, err = abilitySuspendUntilColumnExists()
	require.NoError(t, err)
	require.True(t, exists)
}

func TestNormalizeAbilitySuspendUntilValuesWithoutColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgres := common.UsingPostgreSQL.Load()
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgres)
	})

	// Table intentionally omits suspend_until to mimic very old versions.
	require.NoError(t, DB.Exec("CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER)").Error)

	require.NoError(t, normalizeAbilitySuspendUntilValues())
}
