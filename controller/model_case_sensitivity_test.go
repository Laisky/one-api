package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestListModels_PreservesConfiguredModelCasing_Issue352 reproduces issue #352.
//
// A channel is configured with a mixed-case model name
// (deepseek-ai/DeepSeek-V4-Flash, as SiliconFlow advertises it). Another adaptor
// (nvidia) registers the same logical model in lowercase
// (deepseek-ai/deepseek-v4-flash), so the global supported-models snapshot carries
// the lowercase casing. /v1/models (ListModels) must still advertise the model
// under the channel's configured casing, because channel routing matches the
// abilities.model column case-sensitively. Advertising the lowercase alias yields
// an id the client cannot call ("No available channels for Model ...").
func TestListModels_PreservesConfiguredModelCasing_Issue352(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	const configuredModel = "deepseek-ai/DeepSeek-V4-Flash"
	groupName := fmt.Sprintf("case-group-%d", time.Now().UnixNano())

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     4201,
		Name:   "siliconflow-mixedcase",
		Status: model.ChannelStatusEnabled,
		Type:   channeltype.SiliconFlow,
		Models: configuredModel,
		Group:  groupName,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     groupName,
		Model:     configuredModel,
		ChannelId: 4201,
		Enabled:   true,
		Priority:  ptrInt64(0),
	}).Error)

	// Precondition: the global snapshot advertises the same model under a
	// DIFFERENT casing. Mirror ListModels' own last-write-wins resolution (a
	// map keyed by lowercased id) rather than scanning the slice, because that
	// is the casing ListModels actually substitutes. Without this collision
	// there is nothing to reproduce, so fail loudly if the catalog ever changes.
	snapshot, err := getSupportedModelsSnapshot()
	require.NoError(t, err)
	snapshotByID := make(map[string]OpenAIModels, len(snapshot))
	for _, m := range snapshot {
		snapshotByID[strings.ToLower(m.Id)] = m
	}
	snapshotCasing := snapshotByID[strings.ToLower(configuredModel)].Id
	require.NotEmpty(t, snapshotCasing,
		"precondition: supported-models snapshot must advertise %q", configuredModel)
	require.NotEqual(t, configuredModel, snapshotCasing,
		"precondition: snapshot casing %q must differ from configured casing %q (issue #352 trigger)",
		snapshotCasing, configuredModel)

	user := &model.User{
		Username: "case-user",
		Group:    groupName,
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(ctxkey.UserObj, user)

	ListModels(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Data []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	listed := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		listed = append(listed, m.Id)
	}

	require.Contains(t, listed, configuredModel,
		"/v1/models must advertise the model under its configured casing %q, not the snapshot alias %q",
		configuredModel, snapshotCasing)
	require.NotContains(t, listed, snapshotCasing,
		"/v1/models must not advertise the non-routable lowercase alias %q", snapshotCasing)

	// Behavioral contract: every advertised id must be routable. Channel selection
	// matches the ability model column case-sensitively (verified for both the DB
	// and the memory-cache paths), so a wrong-cased id fails exactly like the
	// reported "No available channels" error.
	for _, id := range listed {
		_, err := model.GetRandomSatisfiedChannel(groupName, id, false)
		require.NoErrorf(t, err,
			"advertised model %q must be routable via case-sensitive ability lookup", id)
	}
}

// TestResolveUserAvailableModels_PreservesAbilityCasing_Issue352 is a
// deterministic, catalog-independent unit test for the resolver that backs
// /v1/models. It hand-builds the snapshot so the reproduction does not depend on
// which adaptors happen to be compiled in.
func TestResolveUserAvailableModels_PreservesAbilityCasing_Issue352(t *testing.T) {
	// Snapshot advertises the model in lowercase (as the nvidia adaptor does),
	// while the group's ability is configured in mixed case (as SiliconFlow uses).
	snapshot := []OpenAIModels{
		{Id: "deepseek-ai/deepseek-v4-flash", Object: "model", OwnedBy: "nvidia", Root: "deepseek-ai/deepseek-v4-flash"},
		{Id: "some-other-model", Object: "model", OwnedBy: "openai", Root: "some-other-model"},
	}
	abilities := []dto.EnabledAbility{
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 7, ChannelType: channeltype.SiliconFlow},
	}

	got := resolveUserAvailableModels(abilities, snapshot, 123, map[int]*model.Channel{}, nil)

	require.Len(t, got, 1)
	require.Equal(t, "deepseek-ai/DeepSeek-V4-Flash", got[0].Id,
		"listed id must equal the ability's actual routing key, not the snapshot's lowercase alias")
	require.Equal(t, "deepseek-ai/DeepSeek-V4-Flash", got[0].Root,
		"root must also carry the routable casing")
	// Display metadata is still inherited from the snapshot entry.
	require.Equal(t, "nvidia", got[0].OwnedBy,
		"owner metadata should still be inherited from the case-insensitive snapshot match")
}

// TestResolveUserAvailableModels_KeepsDistinctCasingsAsDistinctModels asserts
// that two abilities differing only in case remain two separately-routable
// entries (they are distinct keys in the case-sensitive abilities table), while
// identical names collapse to one.
func TestResolveUserAvailableModels_KeepsDistinctCasingsAsDistinctModels(t *testing.T) {
	snapshot := []OpenAIModels{
		{Id: "deepseek-ai/deepseek-v4-flash", Object: "model", OwnedBy: "nvidia", Root: "deepseek-ai/deepseek-v4-flash"},
	}
	abilities := []dto.EnabledAbility{
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 1, ChannelType: channeltype.SiliconFlow},
		{Model: "deepseek-ai/deepseek-v4-flash", ChannelId: 2, ChannelType: channeltype.NVIDIA},
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 3, ChannelType: channeltype.SiliconFlow}, // duplicate casing -> collapses
	}

	got := resolveUserAvailableModels(abilities, snapshot, 0, map[int]*model.Channel{}, nil)

	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.Id)
	}
	require.ElementsMatch(t, []string{"deepseek-ai/DeepSeek-V4-Flash", "deepseek-ai/deepseek-v4-flash"}, ids,
		"both distinct casings must be listed exactly once each")
}

// TestRetrieveModel_ReturnsConfiguredCasing_Issue352 covers the single-model
// retrieve endpoint (GET /v1/models/:model), which shared the same defect:
// resolving a request case-insensitively but echoing back the catalog's
// (non-routable) casing.
//
// The channel configures an ALL-CAPS casing that no adaptor registers, so the
// exact-match branch (modelsMap[modelId]) misses and the handler must fall
// through to the case-insensitive catalog/snapshot lookup — the branch that
// previously leaked a non-routable casing.
func TestRetrieveModel_ReturnsConfiguredCasing_Issue352(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	const configuredModel = "deepseek-ai/DEEPSEEK-V4-FLASH"
	groupName := fmt.Sprintf("retrieve-group-%d", time.Now().UnixNano())

	// Precondition: the configured casing is NOT an exact catalog entry, but a
	// case-insensitive variant IS. That is exactly what forces the buggy branch.
	require.NotContains(t, modelsMap, configuredModel,
		"precondition: configured casing must be absent from the exact catalog map")
	var variantFound bool
	for key := range modelsMap {
		if strings.EqualFold(key, configuredModel) {
			variantFound = true
			break
		}
	}
	require.True(t, variantFound,
		"precondition: a case-insensitive catalog variant of %q must exist", configuredModel)

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     4301,
		Name:   "siliconflow-retrieve",
		Status: model.ChannelStatusEnabled,
		Type:   channeltype.SiliconFlow,
		Models: configuredModel,
		Group:  groupName,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     groupName,
		Model:     configuredModel,
		ChannelId: 4301,
		Enabled:   true,
		Priority:  ptrInt64(0),
	}).Error)

	user := &model.User{
		Username: "retrieve-user",
		Group:    groupName,
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
	}

	// A client following /v1/models would request the id it was given. Even if it
	// requests the lowercase alias, the retrieved id must be the routable casing.
	for _, requested := range []string{configuredModel, strings.ToLower(configuredModel)} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+requested, nil)
		c.Params = gin.Params{{Key: "model", Value: requested}}
		c.Set(ctxkey.UserObj, user)

		RetrieveModel(c)
		require.Equal(t, http.StatusOK, w.Code)

		var entry struct {
			Id    string `json:"id"`
			Root  string `json:"root"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
		require.Nilf(t, entry.Error, "retrieve %q must not error", requested)
		require.Equalf(t, configuredModel, entry.Id,
			"retrieving %q must echo the routable casing %q", requested, configuredModel)

		_, err := model.GetRandomSatisfiedChannel(groupName, entry.Id, false)
		require.NoErrorf(t, err, "retrieved id %q must be routable", entry.Id)
	}
}
