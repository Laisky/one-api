package model

// Split-topology regression test for channel-name resolution
// (docs/proposals/20260905_observability-data-tiering.md, section 1: "New usage
// queries ... must use the usage-owning handle and its dialect").
//
// `logs` lives on LOG_DB; `channels` lives on the primary handle. Resolving one
// from the other's database is what broke the log list entirely whenever
// LOG_SQL_DSN pointed somewhere else.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// TestFillLogChannelNamesWorksWithSeparateLogDatabase verifies the log list
// still resolves channel names when LOG_DB is a different database that has no
// channels table.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestFillLogChannelNamesWorksWithSeparateLogDatabase(t *testing.T) {
	dir := t.TempDir()

	// Primary handle: owns channels, and deliberately has no logs table.
	primary, err := gorm.Open(sqlite.Open(filepath.Join(dir, "primary.db")), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, primary.AutoMigrate(&Channel{}))

	// Log handle: owns logs, and deliberately has no channels table, exactly as
	// migrateLOGDB provisions it.
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(dir, "logs.db")), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	require.False(t, logDB.Migrator().HasTable(&Channel{}),
		"the fixture must reproduce a log database with no channels table")

	prevDB, prevLogDB := DB, LOG_DB
	DB, LOG_DB = primary, logDB
	t.Cleanup(func() { DB, LOG_DB = prevDB, prevLogDB })

	channel := &Channel{Name: "OpenAI Primary", Type: 1, Key: "test-key-split"}
	require.NoError(t, primary.Create(channel).Error)

	logs := []*Log{{ChannelId: channel.Id}, {ChannelId: channel.Id}, {ChannelId: 0}}

	require.NoError(t, fillLogChannelNames(logs),
		"channel names must resolve from the primary handle, not the log handle")
	require.Equal(t, "OpenAI Primary", logs[0].ChannelName)
	require.Equal(t, "OpenAI Primary", logs[1].ChannelName)
	require.Empty(t, logs[2].ChannelName, "a log with no channel keeps an empty name")
}
