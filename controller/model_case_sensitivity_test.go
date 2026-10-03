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
	"github.com/Laisky/one-api/middleware"
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
	// The nvidia adaptor registers the same logical model in lowercase; that
	// collision is the issue #352 trigger.
	const catalogAlias = "deepseek-ai/deepseek-v4-flash"
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

	// Precondition: the compiled-in catalog really does register this model under
	// a different casing, so the collision under test exists. Fail loudly if the
	// adaptor catalog ever changes and the reproduction goes stale.
	require.NotEqual(t, configuredModel, catalogAlias)
	catalogHasAlias := false
	for _, m := range allModels {
		if m.Id == catalogAlias {
			catalogHasAlias = true
			break
		}
	}
	require.True(t, catalogHasAlias,
		"precondition: compiled-in catalog must advertise the lowercase alias %q", catalogAlias)

	// ListModels builds every entry from the ability itself, so the catalog alias
	// can no longer leak into the response regardless of what the snapshot holds.

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
		"/v1/models must advertise the model under its configured casing %q, not the catalog alias %q",
		configuredModel, catalogAlias)
	require.NotContains(t, listed, catalogAlias,
		"/v1/models must not advertise the non-routable lowercase alias %q", catalogAlias)

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
	// The compiled-in nvidia adaptor registers this model in lowercase while the
	// group's ability is configured in mixed case (as SiliconFlow advertises it).
	abilities := []dto.EnabledAbility{
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 7, ChannelType: channeltype.SiliconFlow},
	}

	got := resolveUserAvailableModels(abilities, map[int]*model.Channel{}, nil)

	require.Len(t, got, 1)
	require.Equal(t, "deepseek-ai/DeepSeek-V4-Flash", got[0].Id,
		"listed id must equal the ability's actual routing key, not the snapshot's lowercase alias")
	require.Equal(t, "deepseek-ai/DeepSeek-V4-Flash", got[0].Root,
		"root must also carry the routable casing")
	// The model is served here by a SiliconFlow channel and only collides with the
	// nvidia adaptor's lowercase catalog id. Reporting "nvidia" would attribute it
	// to a provider this deployment may not even have configured.
	require.Equal(t, "siliconflow", got[0].OwnedBy,
		"owner must come from the channel that actually serves the model")
}

// TestResolveUserAvailableModels_KeepsDistinctCasingsAsDistinctModels asserts
// that two abilities differing only in case remain two separately-routable
// entries (they are distinct keys in the case-sensitive abilities table), while
// identical names collapse to one.
func TestResolveUserAvailableModels_KeepsDistinctCasingsAsDistinctModels(t *testing.T) {
	abilities := []dto.EnabledAbility{
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 1, ChannelType: channeltype.SiliconFlow},
		{Model: "deepseek-ai/deepseek-v4-flash", ChannelId: 2, ChannelType: channeltype.NVIDIA},
		{Model: "deepseek-ai/DeepSeek-V4-Flash", ChannelId: 3, ChannelType: channeltype.SiliconFlow}, // duplicate casing -> collapses
	}

	got := resolveUserAvailableModels(abilities, map[int]*model.Channel{}, nil)

	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.Id)
	}
	require.ElementsMatch(t, []string{"deepseek-ai/DeepSeek-V4-Flash", "deepseek-ai/deepseek-v4-flash"}, ids,
		"both distinct casings must be listed exactly once each")
}

// TestMatchVisibleAbilityByModelIDPrefersExactAndStableFallback verifies exact
// routing IDs take precedence and an ambiguous case-folded request is resolved
// deterministically, independently of database row order.
func TestMatchVisibleAbilityByModelIDPrefersExactAndStableFallback(t *testing.T) {
	abilities := []dto.EnabledAbility{
		{Model: "foo", ChannelId: 2, ChannelType: channeltype.NVIDIA},
		{Model: "Foo", ChannelId: 1, ChannelType: channeltype.SiliconFlow},
	}

	for _, tc := range []struct {
		name      string
		requested string
		wantModel string
		wantID    int
	}{
		{name: "exact mixed case", requested: "Foo", wantModel: "Foo", wantID: 1},
		{name: "exact lowercase", requested: "foo", wantModel: "foo", wantID: 2},
		{name: "stable folded fallback", requested: "FOO", wantModel: "Foo", wantID: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchVisibleAbilityByModelID(abilities, tc.requested)
			require.True(t, ok)
			require.Equal(t, tc.wantModel, got.Model)
			require.Equal(t, tc.wantID, got.ChannelId)

			reversed := []dto.EnabledAbility{abilities[1], abilities[0]}
			gotReversed, ok := matchVisibleAbilityByModelID(reversed, tc.requested)
			require.True(t, ok)
			require.Equal(t, got, gotReversed)
		})
	}
}

// TestIntersectTokenModelIDsUsesExactRoutingCasing verifies token availability
// never case-folds or echoes a token spelling that cannot route.
func TestIntersectTokenModelIDsUsesExactRoutingCasing(t *testing.T) {
	abilities := []dto.EnabledAbility{
		{Model: "Foo", ChannelId: 1},
		{Model: "foo", ChannelId: 2},
	}

	// Casing is a routing key, so FOO matches nothing while foo and Foo each match
	// their own ability; the duplicate collapses and CSV order is preserved.
	got := intersectTokenModelIDs("FOO,foo,Foo,foo", abilities)
	require.Equal(t, []string{"foo", "Foo"}, got)

	// Entries are matched raw, exactly as middleware.IsModelInList does at the
	// relay. A stored " Foo" is not callable, so it must not be advertised either.
	require.False(t, middleware.IsModelInList("Foo", " Foo"),
		"precondition: the relay refuses an untrimmed allow-list entry")
	require.Empty(t, intersectTokenModelIDs(" Foo, foo", abilities),
		"untrimmed entries must not be advertised, since the relay would refuse them")
}

// TestRetrieveModelPrefersExactAbilityCasing verifies the HTTP handler returns
// the exact routing ID when both case variants exist and a stable canonical ID
// for a case-folded fallback request.
func TestRetrieveModelPrefersExactAbilityCasing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := fmt.Sprintf("retrieve-case-group-%d", time.Now().UnixNano())
	for _, channel := range []model.Channel{
		{Id: 4401, Name: "mixed-case", Status: model.ChannelStatusEnabled, Type: channeltype.SiliconFlow, Models: "Foo", Group: groupName},
		{Id: 4402, Name: "lower-case", Status: model.ChannelStatusEnabled, Type: channeltype.NVIDIA, Models: "foo", Group: groupName},
	} {
		require.NoError(t, model.DB.Create(&channel).Error)
		require.NoError(t, channel.AddAbilities())
	}

	user := &model.User{Username: "retrieve-case-user", Group: groupName, Role: model.RoleCommonUser, Status: model.UserStatusEnabled}
	for _, tc := range []struct {
		requested string
		wantID    string
	}{
		{requested: "Foo", wantID: "Foo"},
		{requested: "foo", wantID: "foo"},
		{requested: "FOO", wantID: "Foo"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+tc.requested, nil)
		c.Params = gin.Params{{Key: "model", Value: tc.requested}}
		c.Set(ctxkey.UserObj, user)

		RetrieveModel(c)
		require.Equal(t, http.StatusOK, w.Code)

		var payload struct {
			Id string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		require.Equal(t, tc.wantID, payload.Id)
	}
}

// TestGetAvailableModelsByTokenUsesExactAbilityCasing verifies the token-model
// endpoint omits wrong-cased restrictions and preserves two distinct routable
// case variants.
func TestGetAvailableModelsByTokenUsesExactAbilityCasing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))

	groupName := fmt.Sprintf("token-case-group-%d", time.Now().UnixNano())
	for _, channel := range []model.Channel{
		{Id: 4501, Name: "mixed-case", Status: model.ChannelStatusEnabled, Type: channeltype.SiliconFlow, Models: "Foo", Group: groupName},
		{Id: 4502, Name: "lower-case", Status: model.ChannelStatusEnabled, Type: channeltype.NVIDIA, Models: "foo", Group: groupName},
	} {
		require.NoError(t, model.DB.Create(&channel).Error)
		require.NoError(t, channel.AddAbilities())
	}

	const userID = 91
	// No incidental whitespace: entries are matched raw (see intersectTokenModelIDs),
	// so " foo" would be a different, uncallable entry rather than a spelling of foo.
	tokenModels := "FOO,foo,Foo,foo"
	token := &model.Token{UserId: userID, Name: "case-token", Status: model.TokenStatusEnabled, Models: &tokenModels}
	require.NoError(t, model.DB.Create(token).Error)
	user := &model.User{Id: userID, Username: "token-case-user", Group: groupName, Role: model.RoleCommonUser, Status: model.UserStatusEnabled}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/available_models", nil)
	c.Set(ctxkey.Id, userID)
	c.Set(ctxkey.TokenId, token.Id)
	c.Set(ctxkey.AvailableModels, tokenModels)
	c.Set(ctxkey.UserObj, user)

	GetAvailableModelsByToken(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Available []string `json:"available"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, []string{"foo", "Foo"}, payload.Data.Available)

	// Whatever this endpoint advertises must be callable: /api/available_models and
	// the relay's own 403 now share one predicate, so they cannot contradict.
	for _, advertised := range payload.Data.Available {
		require.Truef(t, middleware.IsModelInList(advertised, tokenModels),
			"advertised model %q must pass the relay's allow-list check", advertised)
	}
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

	// Precondition: the compiled-in catalog holds a case-insensitive VARIANT of the
	// configured id but not the id itself. RetrieveModel no longer consults that
	// catalog at all -- it renders the matched ability -- so this fixture now guards
	// against a regression that reintroduces a catalog lookup, which would echo the
	// non-routable variant back to the client (issue #352).
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
