package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

// TestGetGroupModelsV2ReturnsChannelPriority pins that the ability join actually
// projects the priority column. Without it every row decodes as priority 0 and
// the model-listing owner ranking silently collapses to "lowest channel id",
// which no other test would catch because that is a valid tie-break result.
func TestGetGroupModelsV2ReturnsChannelPriority(t *testing.T) {
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	t.Cleanup(func() { DB = originalDB })

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalUsingSQLite) })

	const group = "priority-projection-group"
	const modelName = "glm-4.7"

	for _, tc := range []struct {
		channelID int
		priority  int64
	}{{channelID: 601, priority: 3}, {channelID: 602, priority: 7}} {
		require.NoError(t, DB.Create(&Channel{
			Id: tc.channelID, Name: "c", Status: ChannelStatusEnabled,
			Type: 1, Models: modelName, Group: group,
		}).Error)
		require.NoError(t, DB.Create(&Ability{
			Group: group, Model: modelName, ChannelId: tc.channelID,
			Enabled: true, Priority: &tc.priority,
		}).Error)
	}

	// Bypass the 10s process-local cache that GetGroupModelsV2 consults first.
	getGroupModelsV2Cache.Delete(group)

	abilities, err := GetGroupModelsV2(context.Background(), group)
	require.NoError(t, err)
	require.Len(t, abilities, 2)

	byChannel := map[int]int64{}
	for _, a := range abilities {
		byChannel[a.ChannelId] = a.Priority
	}
	require.Equal(t, int64(3), byChannel[601])
	require.Equal(t, int64(7), byChannel[602])

	// And the ranking rule agrees with the projected values.
	var best = abilities[0]
	for _, a := range abilities[1:] {
		if a.Beats(best) {
			best = a
		}
	}
	require.Equal(t, 602, best.ChannelId, "the higher-priority channel must win")
}

// TestGetGroupModelsV2NullPriorityScansAsZero pins the COALESCE in the SELECT.
// Ability.Priority is *int64 while the DTO field is int64: SQLite silently scans
// NULL as 0, but PostgreSQL and MySQL raise "converting NULL to int64 is
// unsupported", which would 500 the entire model listing for the group. This test
// therefore proves the query shape, not the driver behavior -- the real coverage
// comes from running it against the live MySQL/Postgres suites.
func TestGetGroupModelsV2NullPriorityScansAsZero(t *testing.T) {
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	t.Cleanup(func() { DB = originalDB })

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalUsingSQLite) })

	const group = "null-priority-group"

	require.NoError(t, DB.Create(&Channel{
		Id: 611, Name: "c", Status: ChannelStatusEnabled,
		Type: 1, Models: "glm-4.7", Group: group,
	}).Error)
	// Raw insert so priority really is NULL rather than the column default.
	require.NoError(t, DB.Exec(
		"INSERT INTO abilities (`group`, model, channel_id, enabled, priority) VALUES (?, ?, ?, ?, NULL)",
		group, "glm-4.7", 611, true).Error)

	getGroupModelsV2Cache.Delete(group)

	abilities, err := GetGroupModelsV2(context.Background(), group)
	require.NoError(t, err, "a NULL priority must not error the listing")
	require.Len(t, abilities, 1)
	require.Equal(t, int64(0), abilities[0].Priority)
}
