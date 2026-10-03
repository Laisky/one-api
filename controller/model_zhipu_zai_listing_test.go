package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// listModelsForGroup drives the real HTTP handler and returns the decoded rows.
func listModelsForGroup(t *testing.T, groupName string) []struct {
	Id      string `json:"id"`
	OwnedBy string `json:"owned_by"`
	Created int    `json:"created"`
	Root    string `json:"root"`
} {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(ctxkey.UserObj, &model.User{
		Username: "listing-user",
		Group:    groupName,
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
	})

	ListModels(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			OwnedBy string `json:"owned_by"`
			Created int    `json:"created"`
			Root    string `json:"root"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "list", payload.Object)
	return payload.Data
}

// TestListModels_ZhipuAndZai_OneRowOwnedByPriorityWinner is the end-to-end guard
// for the reported symptom. Zhipu (open.bigmodel.cn) and Z.ai (api.z.ai) are two
// brands of one company and both serve glm-4.7, so both adaptors ship the id in
// the compiled-in catalog. /v1/models must publish exactly one row for it, owned
// by the channel this deployment would actually route to.
func TestListModels_ZhipuAndZai_OneRowOwnedByPriorityWinner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, tc := range map[string]struct {
		zhipuPriority int64
		zaiPriority   int64
		wantOwner     string
	}{
		"zai wins on priority":   {zhipuPriority: 0, zaiPriority: 10, wantOwner: "zai"},
		"zhipu wins on priority": {zhipuPriority: 10, zaiPriority: 0, wantOwner: "zhipu"},
		// Equal priority: the Zhipu channel below always carries the lower id.
		"equal priority falls back to lower channel id": {zhipuPriority: 5, zaiPriority: 5, wantOwner: "zhipu"},
	} {
		t.Run(name, func(t *testing.T) {
			cleanup := setupUserAvailableModelsTestEnvironment(t)
			t.Cleanup(cleanup)

			const sharedModel = "glm-4.7"
			// Unique per subtest: getGroupModelsV2Cache has a 10s in-process TTL.
			groupName := fmt.Sprintf("glm-group-%d", time.Now().UnixNano())

			for _, ch := range []struct {
				id       int
				chType   int
				priority int64
			}{
				{id: 5101, chType: channeltype.Zhipu, priority: tc.zhipuPriority},
				{id: 5102, chType: channeltype.Zai, priority: tc.zaiPriority},
			} {
				require.NoError(t, model.DB.Create(&model.Channel{
					Id:     ch.id,
					Name:   fmt.Sprintf("channel-%d", ch.id),
					Status: model.ChannelStatusEnabled,
					Type:   ch.chType,
					Models: sharedModel,
					Group:  groupName,
				}).Error)
				require.NoError(t, model.DB.Create(&model.Ability{
					Group:     groupName,
					Model:     sharedModel,
					ChannelId: ch.id,
					Enabled:   true,
					Priority:  ptrInt64(ch.priority),
				}).Error)
			}

			rows := listModelsForGroup(t, groupName)

			matches := make([]string, 0, 1)
			for _, m := range rows {
				if m.Id == sharedModel {
					matches = append(matches, m.OwnedBy)
				}
			}
			require.Len(t, matches, 1,
				"glm-4.7 must appear exactly once even though two channels serve it")
			require.Equal(t, tc.wantOwner, matches[0],
				"owned_by must name the channel routing would prefer")
		})
	}
}

// TestListModels_IsChannelDerivedNotCatalogDerived pins the architectural
// property: the response contains only what enabled channels serve, never the
// compiled-in adaptor catalog. Regressing to a catalog-seeded list would leak
// thousands of unroutable ids.
func TestListModels_IsChannelDerivedNotCatalogDerived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := fmt.Sprintf("single-model-group-%d", time.Now().UnixNano())
	const onlyModel = "glm-4.7"

	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 5201, Name: "only-zhipu", Status: model.ChannelStatusEnabled,
		Type: channeltype.Zhipu, Models: onlyModel, Group: groupName,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: groupName, Model: onlyModel, ChannelId: 5201,
		Enabled: true, Priority: ptrInt64(0),
	}).Error)

	rows := listModelsForGroup(t, groupName)

	require.Len(t, rows, 1, "only the one model this group's channel serves may be listed")
	require.Equal(t, onlyModel, rows[0].Id)
	require.Equal(t, onlyModel, rows[0].Root)
	require.Equal(t, "zhipu", rows[0].OwnedBy)
	require.Equal(t, modelCatalogCreated, rows[0].Created,
		"created must be the frozen constant so clients can diff the list")

	// gpt-4o is in the compiled-in catalog but served by no channel here.
	require.Contains(t, modelsMap, "gpt-4o",
		"precondition: the catalog must know gpt-4o, otherwise this proves nothing")
	for _, m := range rows {
		require.NotEqual(t, "gpt-4o", m.Id, "catalog-only models must not be advertised")
	}
}

// TestListModels_ZeroEnabledChannelsReturnsEmptyArray pins that a fresh
// deployment answers with [] rather than null (the frontend calls .map on it) and
// does not fall back to the compiled-in catalog.
func TestListModels_ZeroEnabledChannelsReturnsEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := fmt.Sprintf("empty-group-%d", time.Now().UnixNano())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(ctxkey.UserObj, &model.User{
		Username: "empty-user", Group: groupName,
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
	})

	ListModels(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`,
		"an empty catalog must serialize as [], never null")
}
