package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateOrderClauseUntrustedInput verifies that request strings can only
// select allowlisted columns and literal directions, including hostile inputs.
func TestValidateOrderClauseUntrustedInput(t *testing.T) {
	t.Parallel()
	allowed := map[string]string{"name": "name", "created_time": "created_at"}
	for _, tc := range []struct{ field, direction, want string }{
		{" Name ", " ASC ", "name asc"},
		{"created_time", "desc", "created_at desc"},
		{"name", "", "name desc"},
		{"name", "asc; DROP TABLE logs; --", "name desc"},
		{"name", "asc, (SELECT 1)", "name desc"},
		{"name", "asc\x00", "name desc"},
		{"name", "asc/**/", "name desc"},
		{"name", "DESC NULLS FIRST", "name desc"},
		{"name; DELETE FROM logs", "asc", "id desc"},
		{"name, id", "asc", "id desc"},
		{"(SELECT 1)", "desc", "id desc"},
		{"unknown", "asc", "id desc"},
	} {
		t.Run(tc.field+"/"+tc.direction, func(t *testing.T) {
			require.Equal(t, tc.want, ValidateOrderClause(tc.field, tc.direction, allowed, "id desc"))
		})
	}
	for field, column := range logSortFields {
		require.Equal(t, column+" asc", GetLogOrderClause(strings.ToUpper(field), "ASC"))
		require.Equal(t, column+" desc", GetLogOrderClause(field, "desc; DROP TABLE logs"))
	}
}

// FuzzValidateOrderClauseUntrustedInput requires every generated request to
// produce one of the trusted ORDER BY fragments, never caller-supplied SQL.
func FuzzValidateOrderClauseUntrustedInput(f *testing.F) {
	for _, seed := range [][2]string{{"created_time", "asc"}, {"quota", "DESC"}, {"id", "asc; DROP TABLE logs"}, {"id DESC; --", ""}, {"", ""}} {
		f.Add(seed[0], seed[1])
	}
	allowed := map[string]string{"id": "id", "created_time": "created_at", "quota": "quota"}
	valid := map[string]bool{"id desc": true, "id asc": true, "created_at asc": true, "created_at desc": true, "quota asc": true, "quota desc": true}
	f.Fuzz(func(t *testing.T, field, direction string) {
		clause := ValidateOrderClause(field, direction, allowed, "id desc")
		require.True(t, valid[clause], "unexpected SQL fragment %q", clause)
	})
}
