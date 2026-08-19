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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// setupUserAvailableModelsTestEnvironment swaps in an isolated SQLite DB
// seeded with Ability + Channel tables and returns a cleanup. Redis must be
// disabled so CacheGetGroupModelsV2 falls through to the DB path.
func setupUserAvailableModelsTestEnvironment(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Ability{}, &model.Channel{}))

	originalDB := model.DB
	originalUsingSQLite := common.UsingSQLite.Load()
	originalRedis := common.IsRedisEnabled()
	model.DB = db
	common.UsingSQLite.Store(true)
	common.SetRedisEnabled(false)

	return func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)
		common.SetRedisEnabled(originalRedis)
	}
}

// TestGetUserAvailableModels_NoAccessibleModelsEmitsDataArray sets up a user
// whose group has zero enabled abilities. The handler must serialize the
// model list as [], not null — the frontend invokes .map() on it.
func TestGetUserAvailableModels_NoAccessibleModelsEmitsDataArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	// Use a unique group name so the in-process getGroupModelsV2Cache (which
	// has a 10s TTL) cannot serve a stale entry from another test.
	emptyGroup := fmt.Sprintf("empty-group-%d", time.Now().UnixNano())
	user := &model.User{
		Username: "no-models-user",
		Password: "password",
		Group:    emptyGroup,
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/available_models", nil)
	c.Set(ctxkey.UserObj, user)

	GetUserAvailableModels(c)

	require.Equal(t, http.StatusOK, w.Code)

	raw := w.Body.String()
	require.Contains(t, raw, `"data":[]`,
		"available models must serialize as [] so frontend .map() does not crash")
	require.NotContains(t, raw, `"data":null`,
		"a nil slice marshals to null and breaks the admin UI")

	var payload struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 0)
}

// TestGetUserAvailableModels_PopulatedReturnsModels is the populated
// companion: an enabled ability for the user's group must surface in the
// returned data array.
func TestGetUserAvailableModels_PopulatedReturnsModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := fmt.Sprintf("populated-group-%d", time.Now().UnixNano())
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     101,
		Name:   "test-channel",
		Status: 1,
		Type:   1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     groupName,
		Model:     "test-model",
		ChannelId: 101,
		Enabled:   true,
		Priority:  ptrInt64(0),
	}).Error)

	user := &model.User{
		Username: "with-models-user",
		Password: "password",
		Group:    groupName,
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/available_models", nil)
	c.Set(ctxkey.UserObj, user)

	GetUserAvailableModels(c)

	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "test-model")
}

func ptrInt64(v int64) *int64 { return &v }
