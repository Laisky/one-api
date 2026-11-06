package model

import (
	"context"
	"database/sql"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/groupname"
)

// wideChannelGroups is a list of individually valid group names whose comma-separated
// form (36 characters) does not fit the varchar(32) channels.group column.
var wideChannelGroups = []string{"default", "enterprise", "partners", "internal"}

// wideChannelGroupColumn is the leading run of wideChannelGroups that fits the column.
const wideChannelGroupColumn = "default,enterprise,partners"

// channelGroupDialects lists the backends the relation is verified on. SQLite always runs;
// MySQL and PostgreSQL run when MYSQL_DSN and PG_DSN are set.
var channelGroupDialects = []string{"sqlite", "mysql", "postgres"}

// openChannelGroupBackend installs a clean primary database for dialect with the channel,
// ability and channel_groups tables and the hydration callback, swapping the package
// globals for the test. SQLite gets the 32-character column limit MySQL and PostgreSQL
// enforce. Parameters: t is the test handle and dialect the backend name. Returns: the
// handle, or nil after skipping when the backend's DSN is not set.
func openChannelGroupBackend(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	originalDB := DB
	originalFlags := []bool{common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()}
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite.Store(originalFlags[0])
		common.UsingMySQL.Store(originalFlags[1])
		common.UsingPostgreSQL.Store(originalFlags[2])
	})

	var db *gorm.DB
	if dialect == "sqlite" {
		conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := conn.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		db = conn
		common.UsingSQLite.Store(true)
		common.UsingMySQL.Store(false)
		common.UsingPostgreSQL.Store(false)
		withChannelGroupColumnLimit(t, legacyChannelGroupColumnWidth)
	} else {
		db = openBackend(t, dialect)
		if db == nil {
			t.Skipf("%s DSN not set, skipping the channel group relation matrix", dialect)
			return nil
		}
		require.NoError(t, db.Migrator().DropTable("abilities", "channel_groups", "channels"))
		t.Cleanup(func() {
			require.NoError(t, db.Migrator().DropTable("abilities", "channel_groups", "channels"))
		})
	}
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelGroup{}))
	require.NoError(t, RegisterChannelGroupHydration(db))
	DB = db
	return db
}

// storedChannelGroupRow reads what channels.group and group_truncated hold, the way an
// older binary would read the row. Parameters: t is the test handle and id the channel.
// Returns: the column value and the flag (nil when NULL).
func storedChannelGroupRow(t *testing.T, id int) (string, *bool) {
	t.Helper()
	var row struct {
		GroupValue     string
		GroupTruncated sql.NullBool
	}
	require.NoError(t, DB.Raw("SELECT "+quoteIdentifier(DB, "group")+" AS group_value, group_truncated FROM channels WHERE id = ?", id).
		Scan(&row).Error)
	if !row.GroupTruncated.Valid {
		return row.GroupValue, nil
	}
	return row.GroupValue, &row.GroupTruncated.Bool
}

// relationGroups returns the channel_groups names of a channel in position order.
// Parameters: t is the test handle and id the channel. Returns: the names.
func relationGroups(t *testing.T, id int) []string {
	t.Helper()
	lists, err := loadChannelGroupLists(DB, []int{id})
	require.NoError(t, err)
	return lists[id]
}

// abilityGroups returns the distinct groups the abilities table routes a channel in.
// Parameters: t is the test handle and id the channel. Returns: the sorted groups.
func abilityGroups(t *testing.T, id int) []string {
	t.Helper()
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", id).Find(&abilities).Error)
	groups := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		if !slices.Contains(groups, ability.Group) {
			groups = append(groups, ability.Group)
		}
	}
	slices.Sort(groups)
	return groups
}

// sortedCopy returns a sorted copy of names. Parameters: names is the list. Returns: the copy.
func sortedCopy(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}

// TestChannelGroupRelationMultiDB is the regression suite for channel group lists wider
// than the varchar(32) channels.group column. Before the relation existed, saving such a
// list failed on MySQL ("Data too long for column 'group'") and PostgreSQL ("value too long
// for type character varying(32)").
func TestChannelGroupRelationMultiDB(t *testing.T) {
	for _, dialect := range channelGroupDialects {
		t.Run(dialect, func(t *testing.T) {
			t.Run("wide list round-trips through every read path", func(t *testing.T) {
				openChannelGroupBackend(t, dialect)
				channel := &Channel{Name: "wide", Type: 1, Key: "sk-wide", Models: "gpt-4o,gpt-4o-mini",
					Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled}
				require.NoError(t, channel.Insert())
				require.Equal(t, groupname.JoinList(wideChannelGroups), channel.Group, "the caller keeps the whole list")

				column, truncated := storedChannelGroupRow(t, channel.Id)
				require.Equal(t, wideChannelGroupColumn, column, "older binaries read whole names that fit")
				require.NotNil(t, truncated)
				require.True(t, *truncated)
				require.Equal(t, wideChannelGroups, relationGroups(t, channel.Id))
				require.Equal(t, sortedCopy(wideChannelGroups), abilityGroups(t, channel.Id),
					"every group, including the ones past the column, is routable")

				whole := groupname.JoinList(wideChannelGroups)
				byID, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				require.Equal(t, whole, byID.Group)
				withoutKey, err := GetChannelById(channel.Id, false)
				require.NoError(t, err)
				require.Equal(t, whole, withoutKey.Group)
				for _, scope := range []string{"all", "limited"} {
					listed, err := GetAllChannels(0, 10, scope, "", "")
					require.NoError(t, err)
					require.Len(t, listed, 1)
					require.Equal(t, whole, listed[0].Group, "list scope %s", scope)
				}
				searched, err := SearchChannels("wide", "", "")
				require.NoError(t, err)
				require.Len(t, searched, 1)
				require.Equal(t, whole, searched[0].Group)
				enabled, err := GetAllEnabledChannels()
				require.NoError(t, err)
				require.Equal(t, whole, enabled[0].Group)

				var narrow Channel
				require.NoError(t, DB.Select("id", "group").First(&narrow, "id = ?", channel.Id).Error)
				require.Equal(t, whole, narrow.Group, "a narrow read without the flag is completed too")

				routed, err := GetRandomSatisfiedChannel("internal", "gpt-4o", false)
				require.NoError(t, err)
				require.Equal(t, channel.Id, routed.Id)
				require.Equal(t, wideChannelGroups, routed.GetGroupNames(),
					"billing sees every group, so the lowest group ratio applies")

				originalMemoryCache := config.MemoryCacheEnabled
				config.MemoryCacheEnabled = true
				t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })
				require.NoError(t, InitChannelCacheContext(context.Background()))
				cached, err := CacheGetRandomSatisfiedChannel("internal", "gpt-4o", false)
				require.NoError(t, err)
				require.Equal(t, channel.Id, cached.Id)
				require.Equal(t, whole, cached.Group)
			})

			t.Run("updates rewrite the column copy, the flag and the relation together", func(t *testing.T) {
				openChannelGroupBackend(t, dialect)
				channel := &Channel{Name: "update", Type: 1, Models: "gpt-4o", Group: "default,vip", Status: ChannelStatusEnabled}
				require.NoError(t, channel.Insert())
				column, truncated := storedChannelGroupRow(t, channel.Id)
				require.Equal(t, "default,vip", column)
				require.False(t, *truncated)

				channel.Group = groupname.JoinList(wideChannelGroups)
				require.NoError(t, channel.Update())
				require.Equal(t, groupname.JoinList(wideChannelGroups), channel.Group)
				column, truncated = storedChannelGroupRow(t, channel.Id)
				require.Equal(t, wideChannelGroupColumn, column)
				require.True(t, *truncated)
				require.Equal(t, sortedCopy(wideChannelGroups), abilityGroups(t, channel.Id))

				// An update that names no group keeps the stored groups.
				require.NoError(t, (&Channel{Id: channel.Id, Name: "renamed"}).Update())
				reloaded, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				require.Equal(t, "renamed", reloaded.Name)
				require.Equal(t, groupname.JoinList(wideChannelGroups), reloaded.Group)
				require.Equal(t, wideChannelGroups, relationGroups(t, channel.Id))

				// A case-only rename must not collide with the row it replaces (MySQL's
				// primary key compares group names case-insensitively).
				channel.Group = "default,ENTERPRISE"
				require.NoError(t, channel.Update())
				require.Equal(t, []string{"default", "ENTERPRISE"}, relationGroups(t, channel.Id))
				column, truncated = storedChannelGroupRow(t, channel.Id)
				require.Equal(t, "default,ENTERPRISE", column)
				require.False(t, *truncated)
				require.Equal(t, []string{"ENTERPRISE", "default"}, abilityGroups(t, channel.Id))
			})

			t.Run("batch insert and an empty list behave like the column default", func(t *testing.T) {
				openChannelGroupBackend(t, dialect)
				batch := []Channel{
					{Name: "batch-wide", Type: 1, Models: "gpt-4o", Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled},
					{Name: "batch-empty", Type: 1, Models: "gpt-4o", Group: " , ", Status: ChannelStatusEnabled},
				}
				require.NoError(t, BatchInsertChannels(batch))
				require.Equal(t, wideChannelGroups, relationGroups(t, batch[0].Id))
				require.Equal(t, []string{"default"}, relationGroups(t, batch[1].Id))
				require.Equal(t, []string{"default"}, abilityGroups(t, batch[1].Id))
				column, _ := storedChannelGroupRow(t, batch[1].Id)
				require.Equal(t, "default", column)
			})

			t.Run("deletes remove the relation rows", func(t *testing.T) {
				openChannelGroupBackend(t, dialect)
				single := &Channel{Name: "single", Type: 1, Models: "gpt-4o", Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled}
				require.NoError(t, single.Insert())
				require.NoError(t, single.Delete())
				require.Empty(t, relationGroups(t, single.Id))
				require.Empty(t, abilityGroups(t, single.Id))

				disabled := &Channel{Name: "disabled", Type: 1, Models: "gpt-4o", Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusAutoDisabled}
				kept := &Channel{Name: "kept", Type: 1, Models: "gpt-4o", Group: "default,vip", Status: ChannelStatusEnabled}
				require.NoError(t, disabled.Insert())
				require.NoError(t, kept.Insert())
				removed, err := DeleteDisabledChannel()
				require.NoError(t, err)
				require.EqualValues(t, 1, removed)
				require.Empty(t, relationGroups(t, disabled.Id))
				require.Empty(t, abilityGroups(t, disabled.Id), "bulk deletion also removes the channel's abilities")
				require.Equal(t, []string{"default", "vip"}, relationGroups(t, kept.Id))
				require.Equal(t, []string{"default", "vip"}, abilityGroups(t, kept.Id))

				selected := &Channel{Name: "selected", Type: 1, Models: "gpt-4o", Group: "default", Status: ChannelStatusManuallyDisabled}
				require.NoError(t, selected.Insert())
				deleted, err := DeleteSelectedDisabledChannel(context.Background(), selected.Id)
				require.NoError(t, err)
				require.True(t, deleted)
				require.Empty(t, relationGroups(t, selected.Id))
			})

			t.Run("rollback to an older binary and back", func(t *testing.T) {
				runChannelGroupRollbackCase(t, dialect)
			})

			t.Run("upgrade backfills a pre-relation deployment", func(t *testing.T) {
				runChannelGroupUpgradeCase(t, dialect)
			})
		})
	}
}

// countChannelGroupReads installs a query callback on db that counts statements reading
// channel_groups. Parameters: t is the test handle and db the handle. Returns: the counter.
func countChannelGroupReads(t *testing.T, db *gorm.DB) *atomic.Int64 {
	t.Helper()
	var reads atomic.Int64
	name := "test:count_channel_group_reads_" + t.Name()
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == (ChannelGroup{}).TableName() {
			reads.Add(1)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(name)) })
	return &reads
}

// TestChannelGroupHydrationStatementCost pins the read cost of the relation: a channel
// whose column holds its whole list (every list that fits 32 characters) costs no extra
// statement on any read path, including the per-request routing reads, and a list query
// completes all of its truncated channels with one batched statement.
func TestChannelGroupHydrationStatementCost(t *testing.T) {
	db := openChannelGroupBackend(t, "sqlite")
	plain := &Channel{Name: "plain", Type: 1, Models: "gpt-4o", Group: "default,vip", Status: ChannelStatusEnabled}
	require.NoError(t, plain.Insert())
	reads := countChannelGroupReads(t, db)

	_, err := GetChannelById(plain.Id, true)
	require.NoError(t, err)
	_, err = GetRandomSatisfiedChannel("vip", "gpt-4o", false)
	require.NoError(t, err)
	_, err = GetAllEnabledChannels()
	require.NoError(t, err)
	require.Zero(t, reads.Load(), "complete columns must not read channel_groups")

	for i := range 3 {
		wide := &Channel{Name: "wide-" + string(rune('a'+i)), Type: 1, Models: "gpt-4o",
			Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled}
		require.NoError(t, wide.Insert())
	}
	reads.Store(0)
	listed, err := GetAllChannels(0, 10, "all", "", "")
	require.NoError(t, err)
	require.Len(t, listed, 4)
	require.EqualValues(t, 1, reads.Load(), "one batched read completes every truncated channel in a list")
}
