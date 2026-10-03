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
)

// channelStatusRequest executes the real status-only handler with the supplied
// JSON body and returns the HTTP result. Authentication is tested on the router.
func channelStatusRequest(body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { gmwSetLoggerForUUIDContract(c) })
	router.PUT("/api/channel/", UpdateChannel)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/channel/?status_only=1", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestChannelStatusOnlyPreservesConfiguration checks minimal UUID status requests
// update routing without accepting unrelated configuration changes from the body.
func TestChannelStatusOnlyPreservesConfiguration(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	ability := &model.Ability{ChannelId: fixture.channel.Id, Model: "gpt-4o", Group: "default", Enabled: true}
	require.NoError(t, model.DB.Create(ability).Error)
	for _, status := range []int{2, 1, 1} {
		body, err := json.Marshal(map[string]any{"uuid": fixture.channel.UUID, "status": status, "models": "must-not-overwrite", "key": "private-key"})
		require.NoError(t, err)
		recorder := channelStatusRequest(string(body))
		require.Equal(t, http.StatusOK, recorder.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Equal(t, true, payload["success"])
		require.NotContains(t, recorder.Body.String(), "private-key")
		var stored model.Channel
		require.NoError(t, model.DB.First(&stored, fixture.channel.Id).Error)
		require.Equal(t, status, stored.Status)
		require.Equal(t, fixture.channel.Models, stored.Models)
		require.Equal(t, fixture.channel.Key, stored.Key)
		var storedAbility model.Ability
		require.NoError(t, model.DB.First(&storedAbility, "channel_id = ?", fixture.channel.Id).Error)
		require.Equal(t, status == model.ChannelStatusEnabled, storedAbility.Enabled)
	}
}

// TestChannelStatusOnlyReportsPersistenceFailure proves an ability-write failure
// rolls back the channel and yields a sanitized failure instead of HTTP success.
func TestChannelStatusOnlyReportsPersistenceFailure(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: fixture.channel.Id, Model: "gpt-4o", Group: "default", Enabled: true}).Error)
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER fail_status BEFORE UPDATE OF enabled ON abilities BEGIN SELECT RAISE(FAIL, 'private-db-detail'); END`).Error)
	recorder := channelStatusRequest(`{"uuid":"` + fixture.channel.UUID + `","status":2}`)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "private-db-detail")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, false, payload["success"])
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, fixture.channel.Id).Error)
	require.Equal(t, fixture.channel.Status, stored.Status)
	var storedAbility model.Ability
	require.NoError(t, model.DB.First(&storedAbility, "channel_id = ?", fixture.channel.Id).Error)
	require.True(t, storedAbility.Enabled)
}

// TestChannelStatusOnlyRejectsMissingStatus verifies a malformed status-only
// request cannot silently assign the invalid zero status to a live channel.
func TestChannelStatusOnlyRejectsMissingStatus(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	recorder := channelStatusRequest(`{"uuid":"` + fixture.channel.UUID + `"}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, fixture.channel.Id).Error)
	require.Equal(t, fixture.channel.Status, stored.Status)
}
