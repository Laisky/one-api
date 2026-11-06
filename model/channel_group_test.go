package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// withChannelGroupColumnLimit makes channels.group hold limit characters for the test,
// as MySQL and PostgreSQL do, even on SQLite. Parameters: t is the test handle and limit
// the capacity; 0 restores the backend's own limit.
func withChannelGroupColumnLimit(t *testing.T, limit int32) {
	t.Helper()
	previous := channelGroupColumnLimitOverride.Load()
	channelGroupColumnLimitOverride.Store(limit)
	t.Cleanup(func() { channelGroupColumnLimitOverride.Store(previous) })
}

// TestChannelGroupColumnValue pins the compatibility copy written to channels.group: the
// whole list when it fits, otherwise the longest leading run of whole names.
func TestChannelGroupColumnValue(t *testing.T) {
	withChannelGroupColumnLimit(t, legacyChannelGroupColumnWidth)
	cases := []struct {
		name      string
		groups    []string
		column    string
		truncated bool
	}{
		{name: "empty", groups: nil, column: "", truncated: false},
		{name: "fits", groups: []string{"default", "vip"}, column: "default,vip"},
		{name: "exactly 32", groups: []string{"default", "enterprise", "partners-team"}, column: "default,enterprise,partners-team"},
		{name: "one over", groups: []string{"default", "enterprise", "partners", "internal"},
			column: "default,enterprise,partners", truncated: true},
		{name: "stops at the first name that does not fit", groups: []string{"default", strings.Repeat("a", 30), "vip"},
			column: "default", truncated: true},
		{name: "a lone wide name is kept", groups: []string{strings.Repeat("b", 32), "vip"},
			column: strings.Repeat("b", 32), truncated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			column, truncated := channelGroupColumnValue(tc.groups)
			require.Equal(t, tc.column, column)
			require.Equal(t, tc.truncated, truncated)
			require.LessOrEqual(t, len([]rune(column)), legacyChannelGroupColumnWidth)
		})
	}
}

// TestChannelGroupColumnLimitPerBackend pins that SQLite, which never enforced the width,
// keeps the whole list in the column while MySQL and PostgreSQL get the 32-character copy.
func TestChannelGroupColumnLimitPerBackend(t *testing.T) {
	original := []bool{common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()}
	t.Cleanup(func() {
		common.UsingSQLite.Store(original[0])
		common.UsingMySQL.Store(original[1])
		common.UsingPostgreSQL.Store(original[2])
	})
	wide := []string{"default", "enterprise", "partners", "internal"}

	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	column, truncated := channelGroupColumnValue(wide)
	require.Equal(t, "default,enterprise,partners,internal", column)
	require.False(t, truncated)

	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(true)
	column, truncated = channelGroupColumnValue(wide)
	require.Equal(t, "default,enterprise,partners", column)
	require.True(t, truncated)
}

// TestResolveChannelGroupList pins when the relation is trusted over the column.
func TestResolveChannelGroupList(t *testing.T) {
	withChannelGroupColumnLimit(t, legacyChannelGroupColumnWidth)
	full := []string{"default", "enterprise", "partners", "internal"}

	groups, fromRelation := resolveChannelGroupList("default,enterprise,partners", full)
	require.True(t, fromRelation, "the column is exactly the copy of the relation")
	require.Equal(t, full, groups)

	groups, fromRelation = resolveChannelGroupList(" default, enterprise ,partners ", full)
	require.True(t, fromRelation, "formatting differences in the column do not count as a rewrite")
	require.Equal(t, full, groups)

	groups, fromRelation = resolveChannelGroupList("default", full)
	require.False(t, fromRelation, "an older binary rewrote the column; it is the newer intent")
	require.Equal(t, []string{"default"}, groups)

	groups, fromRelation = resolveChannelGroupList("default,vip", nil)
	require.False(t, fromRelation)
	require.Equal(t, []string{"default", "vip"}, groups)
}

// TestStatementReadsColumn pins how the hydration callback tells whether a query read the
// group_truncated flag.
func TestStatementReadsColumn(t *testing.T) {
	cases := []struct {
		name    string
		selects []string
		omits   []string
		want    bool
	}{
		{name: "all columns", want: true},
		{name: "omit another column", omits: []string{"key"}, want: true},
		{name: "omitted", omits: []string{"group_truncated"}, want: false},
		{name: "selected", selects: []string{"id", "group", "group_truncated"}, want: true},
		{name: "selected quoted and qualified", selects: []string{"`channels`.`group_truncated`"}, want: true},
		{name: "selected in one string", selects: []string{"id, group, GROUP_TRUNCATED"}, want: true},
		{name: "star", selects: []string{"*"}, want: true},
		{name: "not selected", selects: []string{"id", "group"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt := &gorm.Statement{Selects: tc.selects, Omits: tc.omits}
			require.Equal(t, tc.want, statementReadsColumn(stmt, channelGroupTruncatedColumn))
		})
	}
}

// TestRegisterChannelGroupHydrationIsIdempotent verifies a second registration neither
// fails nor installs a second callback.
func TestRegisterChannelGroupHydrationIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, RegisterChannelGroupHydration(db))
	require.NoError(t, RegisterChannelGroupHydration(db))
	require.NotNil(t, db.Callback().Query().Get(channelGroupHydrationCallback))
	require.Error(t, RegisterChannelGroupHydration(nil))
}
