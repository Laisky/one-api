package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
)

// setupChannelStatusTestEnvironment swaps in a clean SQLite DB seeded with
// the Channel table and returns a cleanup that restores globals.
func setupChannelStatusTestEnvironment(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	originalDB := model.DB
	originalUsingSQLite := common.UsingSQLite.Load()
	model.DB = db
	common.UsingSQLite.Store(true)

	return func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
}

// TestGetChannelStatus_NoChannelsEmitsDataArray verifies the channel status
// endpoint returns "data":[] (not null) when no channels are configured.
// The frontend invokes .map() on the array and crashes on null.
func TestGetChannelStatus_NoChannelsEmitsDataArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupChannelStatusTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/status/channel", GetChannelStatus)

	request, _ := http.NewRequest(http.MethodGet, "/api/status/channel", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	raw := response.Body.String()
	require.Contains(t, raw, `"data":[]`,
		"empty channel status must serialize as [] so frontend .map() does not crash")
	require.NotContains(t, raw, `"data":null`,
		"a nil slice marshals to null and breaks the admin UI")

	var payload struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Total   int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 0)
	require.Equal(t, int64(0), payload.Total)
}

// TestGetChannelStatus_PopulatedReturnsChannels confirms the populated path
// still surfaces channels in the data array.
func TestGetChannelStatus_PopulatedReturnsChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupChannelStatusTestEnvironment(t)
	t.Cleanup(cleanup)

	require.NoError(t, model.DB.Create(&model.Channel{
		Name:   "test-channel",
		Status: 1,
		Type:   1,
	}).Error)

	router := gin.New()
	router.GET("/api/status/channel", GetChannelStatus)

	request, _ := http.NewRequest(http.MethodGet, "/api/status/channel", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	var payload struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
		Total   int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 1)
	require.Equal(t, "test-channel", payload.Data[0]["name"])
	require.Equal(t, int64(1), payload.Total)
}
