package model

import (
	"slices"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/groupname"
)

// ChannelGroup is one row of the channel-to-group relation: channel ChannelId is
// assigned group GroupName, at Position in the channel's ordered group list.
//
// The relation is the complete record of every channel's groups. channels.group
// predates it and is kept as a compatibility copy, because older binaries read and
// write that column and an operator must be able to roll back to them. The column is
// varchar(32) on MySQL and PostgreSQL, so it holds the whole comma-separated list
// only when the list fits; otherwise it holds the longest leading run of whole names
// that fits and Channel.GroupTruncated is set (see channelGroupColumnValue).
//
// The primary key (channel_id, group_name) inherits the database's default collation,
// which is case-insensitive on MySQL, the same as abilities.group. Writers therefore
// store a list already deduplicated by groupname.ParseList, which keeps one spelling
// of names that differ only by case.
type ChannelGroup struct {
	ChannelId int    `gorm:"primaryKey;autoIncrement:false"`
	GroupName string `gorm:"column:group_name;type:varchar(32);primaryKey"`
	Position  int    `gorm:"not null"`
}

// TableName returns the channel_groups table name.
//
// Return values:
//   - string: the table name.
func (ChannelGroup) TableName() string {
	return "channel_groups"
}

// legacyChannelGroupColumnWidth is the declared width of channels.group, in characters.
// The width must never change: older binaries declare the same varchar(32), and their
// AutoMigrate would alter a wider column back, failing or truncating the stored lists.
const legacyChannelGroupColumnWidth = 32

// channelGroupColumnLimitOverride, when positive, replaces the backend's column limit.
// Tests set it to exercise truncation on SQLite, which does not enforce varchar widths.
var channelGroupColumnLimitOverride atomic.Int32

// channelGroupColumnLimit returns how many characters channels.group can hold on the
// current backend. SQLite does not enforce varchar widths, so it keeps storing the
// whole list there, exactly as every earlier release did; 0 means unlimited.
//
// Return values:
//   - int: the column's capacity in characters, or 0 when it is unlimited.
func channelGroupColumnLimit() int {
	if limit := channelGroupColumnLimitOverride.Load(); limit > 0 {
		return int(limit)
	}
	if common.UsingSQLite.Load() {
		return 0
	}
	return legacyChannelGroupColumnWidth
}

// channelGroupColumnValue returns the channels.group value that stands for groups:
// the whole comma-separated list when it fits the column, otherwise the longest
// leading run of whole names that fits. An older binary reading the column therefore
// sees a subset of the channel's groups, never a cut name. A subset can only make an
// older binary route the channel to fewer groups and bill it at an equal or higher
// group ratio (the channel ratio is the lowest ratio among its groups).
//
// Parameters:
//   - groups: the channel's complete, normalized group list.
//
// Return values:
//   - string: the value to store in channels.group.
//   - bool: true when names were left out, so channel_groups alone holds the full list.
func channelGroupColumnValue(groups []string) (string, bool) {
	whole := groupname.JoinList(groups)
	limit := channelGroupColumnLimit()
	if limit <= 0 || utf8.RuneCountInString(whole) <= limit {
		return whole, false
	}
	// The first name is always kept, so the column is never empty: on MySQL and
	// PostgreSQL every name already fits, because names were stored in this column or
	// validated against the same 32-character limit.
	kept, width := 1, utf8.RuneCountInString(groups[0])
	for kept < len(groups) {
		next := width + 1 + utf8.RuneCountInString(groups[kept])
		if next > limit {
			break
		}
		kept, width = kept+1, next
	}
	return groupname.JoinList(groups[:kept]), true
}

// resolveChannelGroupList decides which list a stored channel row stands for, given its
// channels.group value and its channel_groups rows. The relation is trusted only when
// the column still holds exactly what a relation-aware writer stores for that relation;
// otherwise something else (an older binary, or manual SQL) rewrote the column after
// the relation was written, and the column is the newer intent.
//
// Parameters:
//   - column: the stored channels.group value.
//   - relation: the channel's channel_groups names in position order.
//
// Return values:
//   - []string: the effective group list.
//   - bool: true when the relation was used because it agrees with the column.
func resolveChannelGroupList(column string, relation []string) ([]string, bool) {
	stored := groupname.ParseList(column)
	if len(relation) == 0 {
		return stored, false
	}
	expected, _ := channelGroupColumnValue(relation)
	if !slices.Equal(groupname.ParseList(expected), stored) {
		return stored, false
	}
	return relation, true
}

// assignedChannelGroups returns the normalized list a channel write assigns. An empty
// submission resolves to groupname.Default, the value the channels.group column default
// has always given such a channel, so the relation and the abilities agree with it.
//
// Parameters:
//   - channel: the channel being created.
//
// Return values:
//   - []string: the groups to persist; never empty.
func assignedChannelGroups(channel *Channel) []string {
	if groups := channel.GetGroupNames(); len(groups) > 0 {
		return groups
	}
	return []string{groupname.Default}
}

// stageChannelGroupColumn points channel at the values its channels row stores for
// groups, so a following GORM write persists the column copy instead of the whole list.
// The returned function restores channel.Group to the whole list; call it as soon as
// the write returns, whether or not it failed.
//
// Parameters:
//   - channel: the channel about to be written.
//   - groups: the channel's complete, normalized group list.
//
// Return values:
//   - func(): restores channel.Group to the comma-separated whole list.
func stageChannelGroupColumn(channel *Channel, groups []string) func() {
	column, truncated := channelGroupColumnValue(groups)
	channel.Group = column
	channel.GroupTruncated = &truncated
	whole := groupname.JoinList(groups)
	return func() {
		channel.Group = whole
	}
}

// loadChannelGroupLists reads the channel_groups names of every channel in channelIDs
// through db, so a caller inside a transaction sees its own uncommitted writes.
//
// Parameters:
//   - db: the handle or transaction to read through.
//   - channelIDs: the channels whose groups are read.
//
// Return values:
//   - map[int][]string: each channel's names in position order; channels without rows are absent.
//   - error: a wrapped read failure.
func loadChannelGroupLists(db *gorm.DB, channelIDs []int) (map[int][]string, error) {
	lists := make(map[int][]string, len(channelIDs))
	for batch := range slices.Chunk(channelIDs, abilitySyncBatchSize) {
		var rows []ChannelGroup
		if err := db.Where("channel_id IN ?", batch).
			Order("channel_id").Order("position").
			Find(&rows).Error; err != nil {
			return nil, errors.Wrapf(err, "read channel_groups of %d channels", len(batch))
		}
		for _, row := range rows {
			lists[row.ChannelId] = append(lists[row.ChannelId], row.GroupName)
		}
	}
	return lists, nil
}

// replaceChannelGroupsWithDB makes groups the channel's channel_groups rows inside the
// caller's transaction. An unchanged list writes nothing. A changed one is deleted
// before it is inserted, so a case-only rename cannot collide with the row it replaces
// on MySQL's case-insensitive primary key.
//
// Parameters:
//   - tx: the transaction that writes the channel row.
//   - channelID: the channel whose groups are replaced.
//   - groups: the complete, normalized group list, in order.
//
// Return values:
//   - error: a wrapped read or write failure; the caller rolls the transaction back.
func replaceChannelGroupsWithDB(tx *gorm.DB, channelID int, groups []string) error {
	current, err := loadChannelGroupLists(tx, []int{channelID})
	if err != nil {
		return errors.Wrapf(err, "read groups of channel %d", channelID)
	}
	if slices.Equal(current[channelID], groups) {
		return nil
	}
	if err := deleteChannelGroupsWithDB(tx, channelID); err != nil {
		return errors.Wrapf(err, "clear groups of channel %d", channelID)
	}
	if len(groups) == 0 {
		return nil
	}
	rows := make([]ChannelGroup, 0, len(groups))
	for position, name := range groups {
		rows = append(rows, ChannelGroup{ChannelId: channelID, GroupName: name, Position: position})
	}
	if err := tx.CreateInBatches(&rows, abilitySyncBatchSize).Error; err != nil {
		return errors.Wrapf(err, "write %d groups of channel %d", len(rows), channelID)
	}
	return nil
}

// insertNewChannelGroupsWithDB writes the channel_groups rows of channels the caller's
// transaction has just created, with one delete and batched inserts for the whole set
// instead of a read and a rewrite per channel. The delete clears any rows left under a
// reused id (MySQL before 8.0 can reuse auto-increment ids after a restart).
//
// Parameters:
//   - tx: the transaction that created the channels.
//   - channelIDs: the new channels' ids.
//   - groupLists: each channel's complete, normalized group list, aligned with channelIDs.
//
// Return values:
//   - error: a wrapped write failure; the caller rolls the transaction back.
func insertNewChannelGroupsWithDB(tx *gorm.DB, channelIDs []int, groupLists [][]string) error {
	if len(channelIDs) != len(groupLists) {
		return errors.Errorf("got %d channel ids but %d group lists", len(channelIDs), len(groupLists))
	}
	if err := deleteChannelGroupsWithDB(tx, channelIDs...); err != nil {
		return errors.Wrap(err, "clear groups of new channels")
	}
	var rows []ChannelGroup
	for i, channelID := range channelIDs {
		for position, name := range groupLists[i] {
			rows = append(rows, ChannelGroup{ChannelId: channelID, GroupName: name, Position: position})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := tx.CreateInBatches(&rows, abilitySyncBatchSize).Error; err != nil {
		return errors.Wrapf(err, "write %d groups of %d new channels", len(rows), len(channelIDs))
	}
	return nil
}

// deleteChannelGroupsWithDB removes the channel_groups rows of the given channels inside
// the caller's transaction.
//
// Parameters:
//   - tx: the transaction that deletes the channels.
//   - channelIDs: the channels whose rows are removed.
//
// Return values:
//   - error: a wrapped write failure.
func deleteChannelGroupsWithDB(tx *gorm.DB, channelIDs ...int) error {
	for batch := range slices.Chunk(channelIDs, abilitySyncBatchSize) {
		if err := tx.Where("channel_id IN ?", batch).Delete(&ChannelGroup{}).Error; err != nil {
			return errors.Wrapf(err, "delete channel_groups of %d channels", len(batch))
		}
	}
	return nil
}
