package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// setupRedemptionTestEnvironment swaps in an isolated SQLite DB seeded with
// the Redemption table and returns a cleanup that restores the globals.
func setupRedemptionTestEnvironment(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}))

	originalDB := model.DB
	originalUsingSQLite := common.UsingSQLite.Load()
	model.DB = db
	common.UsingSQLite.Store(true)

	return func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
}

// TestGenerateRedemption_FirstInsertFailsEmitsKeysArray exercises the
// early-error path of AddRedemption: the redemption table is dropped before
// the handler runs so the very first Insert fails and the handler returns
// before appending any keys. The wire response must still carry "data":[]
// rather than "data":null — frontend code calls .map() on the field.
func TestGenerateRedemption_FirstInsertFailsEmitsKeysArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupRedemptionTestEnvironment(t)
	t.Cleanup(cleanup)

	// Drop the table so Insert fails on the first attempt; the handler
	// returns before any UUID is appended to keys.
	require.NoError(t, model.DB.Migrator().DropTable(&model.Redemption{}))

	body, err := json.Marshal(map[string]any{
		"name":  "test",
		"count": 3,
		"quota": 100,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/redemption", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)

	AddRedemption(c)

	require.Equal(t, http.StatusOK, w.Code)

	raw := w.Body.String()
	require.Contains(t, raw, `"data":[]`,
		"early-error redemption response must serialize keys as [] not null")
	require.NotContains(t, raw, `"data":null`,
		"a nil slice marshals to null and crashes the admin UI .map() call")

	var payload struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.False(t, payload.Success, "insert failure must report success=false")
	require.Len(t, payload.Data, 0)
}

// TestGenerateRedemption_PopulatedReturnsKeys is the populated companion
// path: a successful run must persist the requested number of keys and
// return them as a non-empty array.
func TestGenerateRedemption_PopulatedReturnsKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupRedemptionTestEnvironment(t)
	t.Cleanup(cleanup)

	body, err := json.Marshal(map[string]any{
		"name":  "test",
		"count": 2,
		"quota": 100,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/redemption", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)

	AddRedemption(c)

	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 2)
}
