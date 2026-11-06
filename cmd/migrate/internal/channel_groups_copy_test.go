package internal

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/groupname"
	"github.com/Laisky/one-api/model"
)

// copiedChannel describes one source channel of the copy test and the group list a
// target must serve for it.
type copiedChannel struct {
	name   string
	column string
	flag   *bool
	// relation is the source channel_groups list; nil writes no rows.
	relation []string
	want     []string
}

// boolPointer returns a pointer to value. Parameters: value is the flag. Returns: the pointer.
func boolPointer(value bool) *bool {
	return &value
}

// copyTestTarget opens the target backend for the copy test and clears the tables it
// fills. Parameters: t is the test handle and kind the backend. Returns: the connection, or
// nil after skipping when the backend's DSN is not set.
func copyTestTarget(t *testing.T, kind string) *DatabaseConnection {
	t.Helper()
	var dsn string
	switch kind {
	case "sqlite":
		dsn = filepath.Join(t.TempDir(), "target.db")
	case "mysql":
		dsn = os.Getenv("MYSQL_DSN")
	case "postgres":
		dsn = os.Getenv("PG_DSN")
	}
	if dsn == "" {
		if os.Getenv("ONEAPI_REQUIRE_DB_BACKENDS") == "1" {
			t.Fatalf("%s DSN is required", kind)
		}
		t.Skipf("%s DSN not set", kind)
		return nil
	}
	conn, err := ConnectDatabase(kind, dsn)
	require.NoError(t, err)
	drop := func() {
		require.NoError(t, conn.DB.Migrator().DropTable("abilities", "channel_groups", "channels"))
	}
	drop()
	t.Cleanup(func() {
		drop()
		require.NoError(t, conn.Close())
	})
	return conn
}

// TestMigrateDataCopiesCompleteChannelGroupLists verifies cmd/migrate carries every
// channel's complete group list to the target, whatever the source wrote: a SQLite list
// wider than the target's varchar(32) column, a row written before the relation existed,
// and a row already in the 32-character form with its list in channel_groups.
func TestMigrateDataCopiesCompleteChannelGroupLists(t *testing.T) {
	wide := []string{"default", "enterprise", "partners", "internal"}
	channels := []copiedChannel{
		{name: "sqlite-wide", column: groupname.JoinList(wide), flag: boolPointer(false), want: wide},
		{name: "pre-relation", column: "default,vip", want: []string{"default", "vip"}},
		{name: "already-truncated", column: "default,enterprise,partners", flag: boolPointer(true), relation: wide, want: wide},
	}

	for _, kind := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			source, err := ConnectDatabase("sqlite", filepath.Join(t.TempDir(), "source.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, source.Close()) })
			require.NoError(t, source.DB.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.Ability{}))
			for _, seed := range channels {
				row := model.Channel{Name: seed.name, Type: 1, Models: "gpt-4o", Group: seed.column,
					GroupTruncated: seed.flag, Status: model.ChannelStatusEnabled}
				insert := source.DB
				if seed.flag == nil {
					insert = insert.Omit("group_truncated")
				}
				require.NoError(t, insert.Create(&row).Error)
				for position, name := range seed.relation {
					require.NoError(t, source.DB.Create(&model.ChannelGroup{ChannelId: row.Id, GroupName: name, Position: position}).Error)
				}
			}

			target := copyTestTarget(t, kind)
			migrator := &Migrator{Workers: 1, BatchSize: 2, sourceConn: source, targetConn: target}
			require.NoError(t, migrator.runAutoMigration())
			stats := &MigrationStats{}
			require.NoError(t, migrator.migrateData(context.Background(), stats))
			require.Empty(t, stats.Errors)

			var raw []model.Channel
			require.NoError(t, target.DB.Session(&gorm.Session{SkipHooks: true}).Order("id").Find(&raw).Error)
			require.Len(t, raw, len(channels))
			for _, row := range raw {
				require.NotNil(t, row.GroupTruncated, "%s: the target row records whether its column is complete", row.Name)
				if kind != "sqlite" {
					require.LessOrEqual(t, utf8.RuneCountInString(row.Group), 32, "%s fits varchar(32)", row.Name)
				}
			}

			require.NoError(t, model.RegisterChannelGroupHydration(target.DB))
			var served []model.Channel
			require.NoError(t, target.DB.Order("id").Find(&served).Error)
			for i, row := range served {
				require.Equal(t, channels[i].want, row.GetGroupNames(), "%s serves its complete list", row.Name)
				var relation []model.ChannelGroup
				require.NoError(t, target.DB.Where("channel_id = ?", row.Id).Order("position").Find(&relation).Error)
				names := make([]string, 0, len(relation))
				for _, member := range relation {
					names = append(names, member.GroupName)
				}
				require.True(t, slices.Equal(channels[i].want, names), "%s: channel_groups holds %v", row.Name, names)
			}
		})
	}
}
