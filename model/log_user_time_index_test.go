package model

import (
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/one-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const userTimeIndex = "idx_logs_user_created_at_id"

// TestLogUserTimeIndexDefinition guards the ordered access path without changing legacy indexes or query defaults.
func TestLogUserTimeIndexDefinition(t *testing.T) {
	parsed, err := schema.Parse(&Log{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	indexes := parsed.ParseIndexes()
	found := false
	for _, index := range indexes {
		if index.Name == userTimeIndex {
			found = true
			fields := []string{}
			for _, field := range index.Fields {
				fields = append(fields, field.DBName)
			}
			require.Equal(t, []string{"user_id", "created_at", "id"}, fields)
			require.Empty(t, index.Class, "this must not become a uniqueness constraint")
		}
	}
	require.True(t, found)
	for _, name := range []string{"idx_created_at_type", "idx_logs_user_id", "idx_user_token"} {
		require.NotNil(t, parsed.LookIndex(name), "existing access paths remain available")
	}
}

// TestLogUserTimeIndexMigrationAndQueries checks actual creation and re-creation on supported CI engines.
func TestLogUserTimeIndexMigrationAndQueries(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres", "mysql"} {
		t.Run(engine, func(t *testing.T) {
			oldSQLite, oldPG, oldMySQL := common.UsingSQLite.Load(), common.UsingPostgreSQL.Load(), common.UsingMySQL.Load()
			t.Cleanup(func() {
				common.UsingSQLite.Store(oldSQLite)
				common.UsingPostgreSQL.Store(oldPG)
				common.UsingMySQL.Store(oldMySQL)
			})
			var db *gorm.DB
			if engine == "sqlite" {
				var err error
				db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{Logger: logger.Discard})
				require.NoError(t, err)
			} else {
				db = openBackend(t, engine)
				if db == nil {
					t.Skip("native backend DSN not provided")
				}
			}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			var suffix [8]byte
			_, err = rand.Read(suffix[:])
			require.NoError(t, err)
			name := fmt.Sprintf("web_index_%x", suffix)
			// Isolate PostgreSQL index names; never drop or alter shared production/CI tables.
			if engine == "postgres" {
				require.NoError(t, db.Exec("CREATE SCHEMA "+name).Error)
				require.NoError(t, db.Exec("SET search_path TO "+name).Error)
				t.Cleanup(func() {
					require.NoError(t, db.Exec("SET search_path TO public").Error)
					require.NoError(t, db.Exec("DROP SCHEMA "+name+" CASCADE").Error)
				})
			}
			table := db.Table(name)
			require.NoError(t, table.AutoMigrate(&Log{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(name)) })
			require.True(t, table.Migrator().HasIndex(&Log{}, userTimeIndex))
			// Include a tie, another user, provisional records and inclusive timestamp endpoints.
			for _, row := range []struct {
				id, user int
				at       int64
				kind     int
			}{{1, 7, 100, 2}, {2, 7, 100, 2}, {3, 8, 101, 2}, {4, 7, 101, 6}, {5, 7, 102, 7}, {6, 7, 103, 2}} {
				require.NoError(t, db.Exec("INSERT INTO "+name+" (id,user_id,created_at,type,content) VALUES (?,?,?,?,?)", row.id, row.user, row.at, row.kind, "fixture").Error)
			}
			require.NoError(t, table.Migrator().DropIndex(&Log{}, userTimeIndex))
			require.False(t, table.Migrator().HasIndex(&Log{}, userTimeIndex))
			scope := LogListScope{Kind: LogListScopeSelf, SubjectUserID: 7}
			filter := LogListFilter{StartTimestamp: 100, EndTimestamp: 102}
			query, args := buildLogCursorStatement(scope, filter.Normalize(scope), nil, 20)
			query = strings.ReplaceAll(query, "FROM logs", "FROM "+name)
			var before, after []Log
			require.NoError(t, db.WithContext(context.Background()).Raw(query, args...).Scan(&before).Error)
			require.NoError(t, table.AutoMigrate(&Log{}))
			require.True(t, table.Migrator().HasIndex(&Log{}, userTimeIndex))
			require.NoError(t, table.AutoMigrate(&Log{}), "repeated migration must be safe")
			require.NoError(t, db.Raw(query, args...).Scan(&after).Error)
			require.Equal(t, before, after)
			ids := []int{}
			for _, row := range after {
				ids = append(ids, row.Id)
			}
			require.Equal(t, []int{5, 2, 1}, ids)
			var count int64
			require.NoError(t, db.Table(name).Count(&count).Error)
			require.EqualValues(t, 6, count)
			if engine == "sqlite" {
				var plan []struct{ Detail string }
				require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+query, args...).Scan(&plan).Error)
				text := fmt.Sprint(plan)
				require.Contains(t, text, userTimeIndex)
				require.NotContains(t, text, "TEMP B-TREE", "user/date ordering should not sort all history")
			}
		})
	}
}
