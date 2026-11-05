package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/groupname"
)

// legacyChannelRow is the channels row as a binary released before the channel-to-group
// relation declares it: no group_truncated column and the same varchar(32) group column.
// Its AutoMigrate and writes stand in for an older binary after a rollback.
type legacyChannelRow struct {
	Id     int    `gorm:"primaryKey"`
	UUID   string `gorm:"type:char(36);column:uuid"`
	Type   int    `gorm:"default:0"`
	Key    string `gorm:"type:text"`
	Status int    `gorm:"default:1"`
	Name   string `gorm:"index"`
	Models string
	Group  string `gorm:"type:varchar(32);default:'default'"`
}

// TableName maps legacyChannelRow to the channels table. Returns: the table name.
func (legacyChannelRow) TableName() string {
	return "channels"
}

// rawChannelGroupWrite rewrites channels.group without touching the relation or the flag,
// as an older binary's save does. Parameters: t is the test handle, id the channel and
// value the new column value.
func rawChannelGroupWrite(t *testing.T, id int, value string) {
	t.Helper()
	require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).Model(&legacyChannelRow{}).
		Where("id = ?", id).UpdateColumn("group", value).Error)
}

// runChannelGroupRollbackCase verifies an older binary can run against the new schema and
// that its writes win once the new binary is back. Parameters: t is the test handle and
// dialect the backend name.
func runChannelGroupRollbackCase(t *testing.T, dialect string) {
	openChannelGroupBackend(t, dialect)
	edited := &Channel{Name: "edited", Type: 1, Models: "gpt-4o", Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled}
	resaved := &Channel{Name: "resaved", Type: 1, Models: "gpt-4o", Group: groupname.JoinList(wideChannelGroups), Status: ChannelStatusEnabled}
	require.NoError(t, edited.Insert())
	require.NoError(t, resaved.Insert())

	// The older binary's AutoMigrate leaves the schema alone: it declares the group
	// column exactly as before and never drops the columns or tables it does not know.
	require.NoError(t, DB.AutoMigrate(&legacyChannelRow{}))
	require.True(t, DB.Migrator().HasColumn(&Channel{}, "group_truncated"))
	require.True(t, DB.Migrator().HasTable(&ChannelGroup{}))

	// What the older binary reads is a valid list of whole, configured names.
	var legacy legacyChannelRow
	require.NoError(t, DB.First(&legacy, edited.Id).Error)
	require.Equal(t, wideChannelGroupColumn, legacy.Group)
	for _, name := range groupname.ParseList(legacy.Group) {
		require.True(t, groupname.IsValid(name), "older binaries never see a cut name: %q", name)
	}

	// The older binary edits one channel's groups, re-saves the other unchanged, and
	// creates a new channel. Each save also rebuilds the channel's abilities from the
	// column alone, which drops the routes of the groups past the column.
	rawChannelGroupWrite(t, edited.Id, "default,vip")
	rawChannelGroupWrite(t, resaved.Id, wideChannelGroupColumn)
	for id, column := range map[int]string{edited.Id: "default,vip", resaved.Id: wideChannelGroupColumn} {
		oldView := Channel{Id: id, Models: "gpt-4o", Group: column, Status: ChannelStatusEnabled}
		require.NoError(t, syncAbilitiesWithDB(DB, &oldView, false))
	}
	require.Equal(t, []string{"default", "enterprise", "partners"}, abilityGroups(t, resaved.Id))
	created := legacyChannelRow{Name: "created-by-old-binary", Type: 1, Models: "gpt-4o", Group: "default,svip", Status: ChannelStatusEnabled}
	require.NoError(t, DB.Create(&created).Error)

	// Back on the new binary, before startup reconciliation runs, reads already follow
	// the older binary's intent and keep the groups it could not see.
	got, err := GetChannelById(edited.Id, true)
	require.NoError(t, err)
	require.Equal(t, "default,vip", got.Group, "the column rewritten by the older binary wins")
	got, err = GetChannelById(resaved.Id, true)
	require.NoError(t, err)
	require.Equal(t, groupname.JoinList(wideChannelGroups), got.Group, "an unchanged copy keeps the full relation")
	got, err = GetChannelById(created.Id, true)
	require.NoError(t, err)
	require.Equal(t, "default,svip", got.Group)
	require.Nil(t, got.GroupTruncated)

	result, err := ReconcileChannelGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, ChannelGroupReconcileResult{Backfilled: 1, Repaired: 1, AbilitiesRealigned: 1}, result)
	require.Equal(t, []string{"default", "vip"}, relationGroups(t, edited.Id))
	require.Equal(t, []string{"default", "vip"}, abilityGroups(t, edited.Id))
	require.Equal(t, wideChannelGroups, relationGroups(t, resaved.Id))
	require.Equal(t, sortedCopy(wideChannelGroups), abilityGroups(t, resaved.Id),
		"the routes the older binary dropped are restored")
	require.Equal(t, []string{"default", "svip"}, relationGroups(t, created.Id))
	for _, id := range []int{edited.Id, created.Id} {
		_, truncated := storedChannelGroupRow(t, id)
		require.NotNil(t, truncated)
		require.False(t, *truncated)
	}

	again, err := ReconcileChannelGroups(context.Background())
	require.NoError(t, err)
	require.Zero(t, again, "a converged relation is not rewritten")
}

// runChannelGroupUpgradeCase verifies the first start after an upgrade: AutoMigrate adds a
// NULL flag to every existing row and the reconciliation backfills the relation without
// changing what any channel serves. Parameters: t is the test handle and dialect the
// backend name.
func runChannelGroupUpgradeCase(t *testing.T, dialect string) {
	db := openChannelGroupBackend(t, dialect)
	require.NoError(t, db.Migrator().DropTable("abilities", "channel_groups", "channels"))
	require.NoError(t, db.AutoMigrate(&legacyChannelRow{}))
	rows := []legacyChannelRow{
		{Name: "plain", Type: 1, Models: "gpt-4o", Group: "default", Status: ChannelStatusEnabled},
		{Name: "spaced", Type: 1, Models: "gpt-4o", Group: " default, VIP,vip ,", Status: ChannelStatusEnabled},
		{Name: "full-width", Type: 1, Models: "gpt-4o", Group: "default,enterprise,partners-team", Status: ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&rows).Error)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelGroup{}))

	for _, row := range rows {
		column, truncated := storedChannelGroupRow(t, row.Id)
		require.Equal(t, row.Group, column, "the upgrade never rewrites the stored column")
		require.Nil(t, truncated, "the new flag has no default, so existing rows read back NULL")
		got, err := GetChannelById(row.Id, true)
		require.NoError(t, err)
		require.Equal(t, row.Group, got.Group, "unreconciled rows read their complete column")
	}

	result, err := ReconcileChannelGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, ChannelGroupReconcileResult{Backfilled: len(rows)}, result)
	for _, row := range rows {
		require.Equal(t, groupname.ParseList(row.Group), relationGroups(t, row.Id))
		column, truncated := storedChannelGroupRow(t, row.Id)
		require.Equal(t, row.Group, column)
		require.NotNil(t, truncated)
		require.False(t, *truncated)
		got, err := GetChannelById(row.Id, true)
		require.NoError(t, err)
		require.Equal(t, groupname.ParseList(row.Group), got.GetGroupNames())
	}

	again, err := ReconcileChannelGroups(context.Background())
	require.NoError(t, err)
	require.Zero(t, again)
}
