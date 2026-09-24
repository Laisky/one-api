package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// channelResetTestRouter installs the production reset handlers with a request
// logger. Authentication is exercised separately in router contract tests.
func channelResetTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { gmwSetLoggerForUUIDContract(c) })
	router.POST("/api/channel/:id/reset_models", ResetChannelModels)
	router.POST("/api/channel/reset_models", ResetAllChannelModels)
	return router
}

// TestResetChannelModelsUsesServerCatalog verifies a UUID request replaces the
// stored list and cannot supply a different catalog or expose private fields.
func TestResetChannelModelsUsesServerCatalog(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := channelResetTestRouter()
	require.NoError(t, model.DB.Model(fixture.channel).Updates(map[string]any{
		"models": "retired-model", "key": "private-channel-key", "hidden_models": `["gpt-4o"]`,
	}).Error)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/channel/"+fixture.channel.UUID+"/reset_models", strings.NewReader(`{"models":["client-injected-model"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, fixture.channel.UUID, payload.Data["uuid"])
	require.NotContains(t, payload.Data, "id")
	require.NotContains(t, payload.Data, "key")
	require.NotContains(t, recorder.Body.String(), "private-channel-key")

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", fixture.channel.Id).Error)
	expected := (&model.Channel{Models: strings.Join(channelId2Models[channeltype.OpenAI], ",")}).GetSupportedModelNames()
	require.NotEmpty(t, expected)
	require.ElementsMatch(t, expected, stored.GetSupportedModelNames())
	require.Nil(t, stored.HiddenModels)
	require.Equal(t, "private-channel-key", stored.Key)
	require.NotContains(t, stored.Models, "client-injected-model")
	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", stored.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(expected))
}

// TestResetChannelModelsRefusesConflict verifies a conflict is a failed HTTP
// response with an actionable reason, not an apparently successful reset.
func TestResetChannelModelsRefusesConflict(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := channelResetTestRouter()
	require.NoError(t, model.DB.Model(fixture.channel).Update("model_mapping", `{"private-alias":"gpt-4o"}`).Error)
	var before model.Channel
	require.NoError(t, model.DB.First(&before, "id = ?", fixture.channel.Id).Error)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/"+fixture.channel.UUID+"/reset_models", nil))
	require.Equal(t, http.StatusConflict, recorder.Code)
	var payload struct {
		Success  bool                            `json:"success"`
		Conflict model.ChannelModelResetConflict `json:"conflict"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.False(t, payload.Success)
	require.Equal(t, "mapping_conflict", payload.Conflict.Code)
	require.Contains(t, payload.Conflict.Models, "private-alias")
	var after model.Channel
	require.NoError(t, model.DB.First(&after, "id = ?", fixture.channel.Id).Error)
	require.Equal(t, before, after)
}

// TestResetAllChannelModelsIncludesOffPageAndDisabled verifies global scope and
// per-channel isolation when a compatible channel is refused between valid rows.
func TestResetAllChannelModelsIncludesOffPageAndDisabled(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	compatible := &model.Channel{Type: channeltype.OpenAICompatible, Name: "custom", Models: "private-model", Group: "default"}
	disabled := &model.Channel{Type: channeltype.OpenAI, Name: "off-page", Models: "retired", Group: "default", Status: model.ChannelStatusManuallyDisabled}
	require.NoError(t, compatible.Insert())
	require.NoError(t, disabled.Insert())

	recorder := httptest.NewRecorder()
	channelResetTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/reset_models?p=0&size=1&keyword=uuid-contract", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool                     `json:"success"`
		Data    channelModelResetSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, 3, payload.Data.Total)
	require.Equal(t, 2, payload.Data.Reset)
	require.Equal(t, 1, payload.Data.Rejected)
	require.Zero(t, payload.Data.Failed)
	require.Len(t, payload.Data.Results, 3)
	require.Equal(t, fixture.channel.UUID, payload.Data.Results[0].UUID)
	require.Equal(t, "unsupported_channel", payload.Data.Results[1].Conflict.Code)
	require.Equal(t, disabled.UUID, payload.Data.Results[2].UUID)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", compatible.Id).Error)
	require.Equal(t, "private-model", stored.Models)
	stored = model.Channel{}
	require.NoError(t, model.DB.First(&stored, "id = ?", disabled.Id).Error)
	require.Equal(t, model.ChannelStatusManuallyDisabled, stored.Status)
	require.NotContains(t, stored.GetSupportedModelNames(), "retired")
	var enabled int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ? AND enabled = ?", disabled.Id, true).Count(&enabled).Error)
	require.Zero(t, enabled)
}

// TestResetChannelModelsDatabaseFailureIsNotSuccess verifies an ability failure
// rolls back the row and returns a generic operational error, not secret SQL data.
func TestResetChannelModelsDatabaseFailureIsNotSuccess(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER fail_reset_ability BEFORE INSERT ON abilities BEGIN SELECT RAISE(FAIL, 'private-db-detail'); END`).Error)
	recorder := httptest.NewRecorder()
	channelResetTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/"+fixture.channel.UUID+"/reset_models", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "private-db-detail")
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", fixture.channel.Id).Error)
	require.Equal(t, fixture.channel.Models, stored.Models)
}
