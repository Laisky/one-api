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

// TestChannelStrictOutResponses verifies channel strict-out responses and strict-in request bodies.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestChannelStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/channel/", GetAllChannels)
	router.GET("/api/channel/search", SearchChannels)
	router.GET("/api/debug/channel/:id/migration-status", GetChannelMigrationStatus)
	router.GET("/api/channel/:id", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		GetChannel(c)
	})
	router.POST("/api/channel/:id/duplicate", DuplicateChannel)
	router.PUT("/api/channel/", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		UpdateChannel(c)
	})
	router.POST("/api/channel/", AddChannel)

	for _, path := range []string{"/api/channel/?p=0&size=10", "/api/channel/search?keyword=uuid-contract-channel"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Equal(t, true, payload["success"])
		rows, ok := payload["data"].([]any)
		require.True(t, ok, "channel data must be an array: %s", recorder.Body.String())
		require.NotEmpty(t, rows)
		row, ok := rows[0].(map[string]any)
		require.True(t, ok, "channel row must be an object")
		require.Equal(t, fixture.channel.UUID, row["uuid"])
		require.Equal(t, fixture.channel.Name, row["name"])
		require.NotContains(t, row, "id")
	}

	detailRecorder := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, "/api/channel/"+fixture.channel.UUID, nil)
	router.ServeHTTP(detailRecorder, detailRequest)
	detailData := decodeUUIDContractData(t, detailRecorder)
	require.Equal(t, fixture.channel.UUID, detailData["uuid"])
	require.Equal(t, fixture.channel.Name, detailData["name"])
	require.NotContains(t, detailData, "id")

	statusRecorder := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/debug/channel/"+fixture.channel.UUID+"/migration-status", nil)
	router.ServeHTTP(statusRecorder, statusRequest)
	statusData := decodeUUIDContractData(t, statusRecorder)
	require.Equal(t, fixture.channel.UUID, statusData["channel_uuid"])
	require.NotContains(t, statusData, "channel_id")

	duplicateRecorder := httptest.NewRecorder()
	duplicateRequest := httptest.NewRequest(http.MethodPost, "/api/channel/"+fixture.channel.UUID+"/duplicate", nil)
	router.ServeHTTP(duplicateRecorder, duplicateRequest)
	duplicateData := decodeUUIDContractData(t, duplicateRecorder)
	require.NotEmpty(t, duplicateData["uuid"])
	require.NotEqual(t, fixture.channel.UUID, duplicateData["uuid"])
	require.NotContains(t, duplicateData, "id")

	updateRecorder := httptest.NewRecorder()
	updateBody := `{"uuid":"` + fixture.channel.UUID + `","name":"uuid-contract-channel-updated","type":1,"models":"gpt-4o","config":"{}"}`
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(updateBody))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	updateData := decodeUUIDContractData(t, updateRecorder)
	require.Equal(t, fixture.channel.UUID, updateData["uuid"])
	require.Equal(t, "uuid-contract-channel-updated", updateData["name"])
	require.NotContains(t, updateData, "id")

	legacyUpdateRecorder := httptest.NewRecorder()
	legacyUpdateRequest := httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(`{"id":1,"name":"legacy-int-channel","type":1,"models":"gpt-4o","config":"{}"}`))
	legacyUpdateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(legacyUpdateRecorder, legacyUpdateRequest)
	var legacyUpdatePayload map[string]any
	require.NoError(t, json.Unmarshal(legacyUpdateRecorder.Body.Bytes(), &legacyUpdatePayload))
	require.Equal(t, false, legacyUpdatePayload["success"])

	createRecorder := httptest.NewRecorder()
	createBody := `{"name":"uuid-contract-created-channel","type":1,"models":"gpt-4o","config":"{}","uuid":"018f0000-0000-7000-8000-00000000feed"}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/channel/", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRecorder, createRequest)
	require.Equal(t, http.StatusOK, createRecorder.Code)
	var createPayload map[string]any
	require.NoError(t, json.Unmarshal(createRecorder.Body.Bytes(), &createPayload))
	require.Equal(t, true, createPayload["success"])

	var created model.Channel
	require.NoError(t, model.DB.Where("name = ?", "uuid-contract-created-channel").First(&created).Error)
	require.NotEmpty(t, created.UUID)
	require.NotEqual(t, "018f0000-0000-7000-8000-00000000feed", created.UUID)
}
