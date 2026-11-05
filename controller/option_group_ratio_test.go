package controller

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// useGroupRatioCatalog replaces the global GroupRatio catalog with catalogJSON
// for the duration of the test and restores the previous catalog on cleanup.
// Parameters: t is the active test and catalogJSON is a JSON object of ratios.
func useGroupRatioCatalog(t *testing.T, catalogJSON string) {
	t.Helper()

	prev := ratio.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio.UpdateGroupRatioByJSONString(prev))
	})
	require.NoError(t, ratio.UpdateGroupRatioByJSONString(catalogJSON))
}

// TestUpdateOption_GroupRatioRejectsInvalidNewKey verifies that a GroupRatio
// save introducing a group whose name breaks the naming rule, or a malformed
// value, is rejected before anything is persisted, leaving both the stored
// option row and the in-memory catalog untouched.
func TestUpdateOption_GroupRatioRejectsInvalidNewKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupOptionTestEnvironment(t)
	t.Cleanup(cleanup)

	const original = `{"default":1,"vip":1}`
	useGroupRatioCatalog(t, original)
	require.NoError(t, model.UpdateOption("GroupRatio", original))

	cases := []struct {
		name    string
		value   string
		message string
	}{
		{name: "whitespace in new key", value: `{"default":1,"vip":1,"my group":1}`, message: `"my group"`},
		{name: "comma in new key", value: `{"default":1,"a,b":1}`, message: `"a,b"`},
		{name: "case-only duplicate of existing key", value: `{"default":1,"vip":1,"VIP":1}`, message: "differ only by case"},
		{name: "negative ratio", value: `{"default":1,"vip":-1}`, message: ">= 0"},
		{name: "malformed JSON", value: `{"default":`, message: "invalid GroupRatio JSON"},
		{name: "JSON null", value: `null`, message: "JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, resp := callUpdateOption(t, "GroupRatio", tc.value)
			require.NotNil(t, resp)
			require.Equal(t, false, resp["success"], "invalid GroupRatio must be rejected: %v", resp)
			require.Contains(t, resp["message"], tc.message)

			stored := loadStoredOption(t, "GroupRatio")
			require.NotNil(t, stored)
			require.Equal(t, original, stored.Value, "rejected GroupRatio must not be persisted")
			require.Equal(t, []string{"default", "vip"}, ratio.GroupNames(), "rejected GroupRatio must not change the live catalog")
		})
	}
}

// TestUpdateOption_GroupRatioKeepsLegacyInvalidKey verifies that a GroupRatio
// key stored before the naming rule existed is grandfathered: saving the
// catalog again with that key (and a new valid key) succeeds.
func TestUpdateOption_GroupRatioKeepsLegacyInvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupOptionTestEnvironment(t)
	t.Cleanup(cleanup)

	useGroupRatioCatalog(t, `{"default":1,"Old Team":1}`)

	const next = `{"default":1,"Old Team":2,"vip":0.8}`
	_, resp := callUpdateOption(t, "GroupRatio", next)
	require.NotNil(t, resp)
	require.Equal(t, true, resp["success"], "legacy GroupRatio keys must stay editable: %v", resp)

	stored := loadStoredOption(t, "GroupRatio")
	require.NotNil(t, stored)
	require.Equal(t, next, stored.Value)
	require.Equal(t, []string{"Old Team", "default", "vip"}, ratio.GroupNames())
	require.InDelta(t, 2.0, ratio.GetGroupRatio("Old Team"), 1e-9)
}
