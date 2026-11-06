package ratio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// withGroupRatioCatalog installs catalog for one test and restores the previous map afterwards.
func withGroupRatioCatalog(t *testing.T, catalog string) {
	t.Helper()
	previous := GroupRatio2JSONString()
	require.NoError(t, UpdateGroupRatioByJSONString(catalog))
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(previous))
	})
}

// TestUpdateGroupRatioKeepsCatalogOnInvalidJSON checks that a rejected update never wipes the live catalog.
func TestUpdateGroupRatioKeepsCatalogOnInvalidJSON(t *testing.T) {
	withGroupRatioCatalog(t, `{"default":1,"vip":0.5}`)

	for _, bad := range []string{`{"vip":`, `{"vip":"x"}`, `null`, `[]`, `"vip"`} {
		require.Error(t, UpdateGroupRatioByJSONString(bad), bad)
		require.InDelta(t, 0.5, GetGroupRatio("vip"), 1e-9, bad)
		require.Equal(t, []string{"default", "vip"}, GroupNames(), bad)
	}
}

// TestGroupNamesAndHasGroup checks the read helpers return a sorted copy and match exactly.
func TestGroupNamesAndHasGroup(t *testing.T) {
	withGroupRatioCatalog(t, `{"vip":1,"default":1,"VIP-2":0.8}`)

	names := GroupNames()
	require.Equal(t, []string{"VIP-2", "default", "vip"}, names)
	names[0] = "mutated"
	require.Equal(t, []string{"VIP-2", "default", "vip"}, GroupNames())

	require.True(t, HasGroup("vip"))
	require.False(t, HasGroup("VIP"))
	require.False(t, HasGroup(" vip"))
}

// TestValidateGroupRatioUpdate checks the write-path rule for new catalog entries.
func TestValidateGroupRatioUpdate(t *testing.T) {
	withGroupRatioCatalog(t, `{"default":1,"legacy group":1,"Team":1,"team":1}`)

	cases := []struct {
		name    string
		next    string
		wantErr string
	}{
		{name: "valid new key", next: `{"default":1,"vip_2":0.8}`},
		{name: "legacy invalid key kept", next: `{"default":1,"legacy group":0.9}`},
		{name: "legacy case pair kept", next: `{"default":1,"Team":1,"team":0.5}`},
		{name: "zero ratio", next: `{"default":0}`},
		{name: "empty catalog", next: `{}`},
		{name: "invalid json", next: `{"vip":`, wantErr: "invalid"},
		{name: "null", next: `null`, wantErr: "JSON object"},
		{name: "array", next: `[{"vip":1}]`, wantErr: "invalid"},
		{name: "string ratio", next: `{"vip":"1"}`, wantErr: "invalid"},
		{name: "negative ratio", next: `{"vip":-1}`, wantErr: "must be a number >= 0"},
		{name: "new key with space", next: `{"my group":1}`, wantErr: "may only contain"},
		{name: "new key with surrounding space", next: `{" vip":1}`, wantErr: "whitespace"},
		{name: "new key too long", next: `{"abcdefghijklmnopqrstuvwxyz0123456":1}`, wantErr: "cannot exceed"},
		{name: "new key non ascii", next: `{"企业":1}`, wantErr: "may only contain"},
		{name: "new key empty", next: `{"":1}`, wantErr: "cannot be empty"},
		{name: "new key collides with new key by case", next: `{"Gold":1,"gold":1}`, wantErr: "differ only by case"},
		{name: "new key collides with legacy key by case", next: `{"default":1,"DEFAULT":1}`, wantErr: "differ only by case"},
		{name: "duplicate key", next: `{"vip":1,"vip":2}`, wantErr: "more than once"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGroupRatioUpdate(tc.next)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
