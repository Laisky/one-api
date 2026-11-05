package model

import (
	"reflect"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/groupname"
	"github.com/Laisky/one-api/common/logger"
)

// channelGroupHydrationCallback names the query callback that completes Channel.Group.
const channelGroupHydrationCallback = "one_api:channel_groups_hydrate"

// channelGroupTruncatedColumn is the channels column behind Channel.GroupTruncated.
const channelGroupTruncatedColumn = "group_truncated"

// channelTableName is the table GORM maps Channel to.
const channelTableName = "channels"

// RegisterChannelGroupHydration installs, on db, the query callback that gives every
// Channel loaded through GORM its complete group list. Every channel read therefore
// sees the same groups, whichever query loaded it: routing, billing, abilities, the
// cache and the admin API all read Channel.Group.
//
// Register it once on a handle before the handle issues statements, because GORM
// mutates its callback chain without locking. Registering twice is a no-op.
//
// Parameters:
//   - db: the primary database handle.
//
// Return values:
//   - error: a wrapped registration failure.
func RegisterChannelGroupHydration(db *gorm.DB) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	if db.Callback().Query().Get(channelGroupHydrationCallback) != nil {
		return nil
	}
	if err := db.Callback().Query().After("gorm:after_query").
		Register(channelGroupHydrationCallback, hydrateChannelGroups); err != nil {
		return errors.Wrap(err, "register channel group hydration callback")
	}
	return nil
}

// hydrateChannelGroups replaces the stored channels.group copy of every loaded channel
// whose copy may be incomplete with the list from channel_groups, in one batched read.
// A row needs it when GroupTruncated is true, or when the query read group but not
// group_truncated so completeness is unknown. A row whose flag is false or NULL holds
// its complete list in the column (NULL rows were written before the relation existed,
// or by an older binary), so the common case costs no extra statement.
//
// Statements run with SkipHooks read the raw stored row and are left untouched; the
// reconciliation migration relies on that.
//
// Parameters:
//   - db: the statement GORM just ran.
func hydrateChannelGroups(db *gorm.DB) {
	stmt := db.Statement
	if db.Error != nil || db.RowsAffected == 0 || stmt.SkipHooks ||
		stmt.Schema == nil || stmt.Schema.Table != channelTableName {
		return
	}
	// Without the flag in the result, completeness is unknown (and the destination may
	// still hold a stale flag from an earlier read), so every such row is checked.
	flagRead := statementReadsColumn(stmt, channelGroupTruncatedColumn)
	pending := make(map[int][]*Channel)
	visitLoadedChannels(stmt.ReflectValue, func(channel *Channel) {
		if channel.Id == 0 || strings.TrimSpace(channel.Group) == "" {
			return
		}
		if flagRead && (channel.GroupTruncated == nil || !*channel.GroupTruncated) {
			return
		}
		pending[channel.Id] = append(pending[channel.Id], channel)
	})
	if len(pending) == 0 {
		return
	}

	ids := make([]int, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	relations, err := loadChannelGroupLists(db.Session(&gorm.Session{NewDB: true}), ids)
	if err != nil {
		db.AddError(errors.Wrap(err, "complete channel group lists"))
		return
	}

	lg := logger.FromContext(stmt.Context)
	for _, id := range ids {
		relation := relations[id]
		for _, channel := range pending[id] {
			groups, fromRelation := resolveChannelGroupList(channel.Group, relation)
			if fromRelation {
				channel.Group = groupname.JoinList(groups)
				continue
			}
			// The column wins. That is expected for a row read without its flag that
			// predates the relation; for a row marked truncated it means the column was
			// rewritten behind the relation's back. Startup reconciliation repairs that and
			// reports it at WARN once, so this per-read note stays at DEBUG on hot paths.
			if flagRead {
				lg.Debug("channel group relation disagrees with channels.group; using the column",
					append(channel.Ref().Zap(),
						zap.Int("relation_groups", len(relation)),
						zap.Int("column_groups", len(groups)))...)
			}
		}
	}
}

// statementReadsColumn reports whether stmt selected column: every column when it names
// no Select list, otherwise only the listed ones. A column named in Omit is not read.
//
// Parameters:
//   - stmt: the executed statement.
//   - column: the unqualified column name.
//
// Return values:
//   - bool: true when the loaded rows carry column.
func statementReadsColumn(stmt *gorm.Statement, column string) bool {
	if slices.ContainsFunc(stmt.Omits, func(omitted string) bool { return sameColumnName(omitted, column) }) {
		return false
	}
	if len(stmt.Selects) == 0 {
		return true
	}
	for _, selected := range stmt.Selects {
		for part := range strings.SplitSeq(selected, ",") {
			if strings.TrimSpace(part) == "*" || sameColumnName(part, column) {
				return true
			}
		}
	}
	return false
}

// sameColumnName reports whether a Select or Omit entry names column, ignoring quoting,
// a table qualifier and case.
//
// Parameters:
//   - entry: the Select or Omit entry.
//   - column: the unqualified column name.
//
// Return values:
//   - bool: true when entry names column.
func sameColumnName(entry, column string) bool {
	name := strings.TrimSpace(entry)
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	name = strings.Trim(name, "`\"")
	return strings.EqualFold(name, column)
}

// visitLoadedChannels calls visit for every addressable Channel in a query destination:
// a Channel, or a slice or array of Channel or *Channel.
//
// Parameters:
//   - value: the statement's destination value.
//   - visit: called with each loaded channel.
func visitLoadedChannels(value reflect.Value, visit func(*Channel)) {
	visitAddressable := func(element reflect.Value) {
		if element.Kind() == reflect.Struct && element.CanAddr() {
			if channel, ok := element.Addr().Interface().(*Channel); ok {
				visit(channel)
			}
		}
	}
	if kind := value.Kind(); kind != reflect.Slice && kind != reflect.Array {
		visitAddressable(value)
		return
	}
	for i := range value.Len() {
		element := value.Index(i)
		if element.Kind() == reflect.Pointer {
			if element.IsNil() {
				continue
			}
			element = element.Elem()
		}
		visitAddressable(element)
	}
}
