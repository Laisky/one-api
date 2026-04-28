package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

func TestGetSelfByTokenReturnsDetailedInfo(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	t.Cleanup(cleanup)

	models := "gpt-4o-mini,claude-3"
	subnet := "10.0.0.0/8"
	token := &model.Token{
		UserId:         1,
		Name:           "integration",
		Status:         model.TokenStatusEnabled,
		RemainQuota:    1200,
		UsedQuota:      33,
		UnlimitedQuota: false,
		ExpiredTime:    -1,
		CreatedTime:    111,
		AccessedTime:   222,
		CreatedAt:      333,
		UpdatedAt:      444,
		Models:         &models,
		Subnet:         &subnet,
	}
	require.NoError(t, model.DB.Create(token).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user/get-by-token", nil)
	c.Set(ctxkey.Id, token.UserId)
	c.Set(ctxkey.TokenId, token.Id)
	c.Set(ctxkey.AvailableModels, "gpt-4o-mini,claude-3")

	GetSelfByToken(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp["success"].(bool))
	assert.EqualValues(t, token.UserId, resp["uid"])
	assert.EqualValues(t, token.Id, resp["token_id"])
	assert.Equal(t, token.Name, resp["token_name"])
	assert.EqualValues(t, token.RemainQuota, resp["token_remain_quota"])
	assert.EqualValues(t, token.UsedQuota, resp["token_used_quota"])

	data := resp["data"].(map[string]any)
	tokenData := data["token"].(map[string]any)
	assert.Equal(t, token.Name, tokenData["name"])
	assert.EqualValues(t, token.AccessedTime, tokenData["accessed_time"])
	assert.Equal(t, models, tokenData["models"])
	assert.Equal(t, "gpt-4o-mini,claude-3", tokenData["available_models"])
	assert.Equal(t, subnet, tokenData["subnet"])

	userData := data["user"].(map[string]any)
	assert.Equal(t, "testuser", userData["username"])
	assert.EqualValues(t, 1, userData["id"])
}

func TestGetSelfByTokenMissingContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user/get-by-token", nil)

	GetSelfByToken(c)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.False(t, resp["success"].(bool))
	assert.Equal(t, "missing token context", resp["message"])
}

func TestGetSelfByTokenMissingToken(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	t.Cleanup(cleanup)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/user/get-by-token", nil)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 999)

	GetSelfByToken(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.False(t, resp["success"].(bool))
	assert.Contains(t, resp["message"].(string), "failed to get token")
}
