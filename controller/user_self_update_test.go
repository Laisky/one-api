package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

func setupSelfUpdateTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "self-update.db")
	t.Cleanup(func() {
		common.SQLitePath = originalSQLitePath
	})

	model.InitDB()
	model.InitLogDB()

	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.CloseDB())
			model.DB = nil
			model.LOG_DB = nil
		}
	})
}

func createSelfUpdateUser(t *testing.T) *model.User {
	t.Helper()
	hashedPw, err := common.Password2Hash("oldpassword")
	require.NoError(t, err)
	user := &model.User{
		Username:    "selfuser",
		Password:    hashedPw,
		Email:       "self@example.com",
		DisplayName: "Self User",
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

// TestUpdateSelfPasswordOnly verifies that sending only a password
// (without username/display_name) succeeds by falling back to current values.
func TestUpdateSelfPasswordOnly(t *testing.T) {
	setupSelfUpdateTest(t)
	user := createSelfUpdateUser(t)

	router := gin.New()
	router.PUT("/api/user/self", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		UpdateSelf(c)
	})

	// Send only password, no username or display_name
	payload := map[string]string{"password": "newpassword123"}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])

	// Verify password was changed
	var updated model.User
	require.NoError(t, model.DB.First(&updated, user.Id).Error)
	require.True(t, common.ValidatePasswordAndHash("newpassword123", updated.Password))

	// Verify username and display_name were preserved
	require.Equal(t, "selfuser", updated.Username)
	require.Equal(t, "Self User", updated.DisplayName)
}

// TestUpdateSelfFullPayload verifies normal full update still works.
func TestUpdateSelfFullPayload(t *testing.T) {
	setupSelfUpdateTest(t)
	user := createSelfUpdateUser(t)

	router := gin.New()
	router.PUT("/api/user/self", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		UpdateSelf(c)
	})

	payload := map[string]string{
		"username":     "newname",
		"display_name": "New Name",
		"password":     "newpass456",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"])

	var updated model.User
	require.NoError(t, model.DB.First(&updated, user.Id).Error)
	require.Equal(t, "newname", updated.Username)
	require.Equal(t, "New Name", updated.DisplayName)
	require.True(t, common.ValidatePasswordAndHash("newpass456", updated.Password))
}

// TestUpdateSelfWithoutPassword verifies that omitting password keeps the old one.
func TestUpdateSelfWithoutPassword(t *testing.T) {
	setupSelfUpdateTest(t)
	user := createSelfUpdateUser(t)

	router := gin.New()
	router.PUT("/api/user/self", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		UpdateSelf(c)
	})

	payload := map[string]string{
		"username":     "selfuser",
		"display_name": "Updated Display",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"])

	var updated model.User
	require.NoError(t, model.DB.First(&updated, user.Id).Error)
	require.Equal(t, "Updated Display", updated.DisplayName)
	// Old password should still work
	require.True(t, common.ValidatePasswordAndHash("oldpassword", updated.Password))
}

// TestCreateUserWithAllFields verifies that admin create user honors
// email, quota, and group fields.
func TestCreateUserWithAllFields(t *testing.T) {
	setupSelfUpdateTest(t)

	router := gin.New()
	router.POST("/api/user/", func(c *gin.Context) {
		c.Set("role", model.RoleRootUser)
		CreateUser(c)
	})

	payload := map[string]any{
		"username":     "newadminuser",
		"password":     "testpass123",
		"display_name": "Admin Created",
		"email":        "admin-created@example.com",
		"quota":        500000,
		"group":        "vip",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])

	var created model.User
	require.NoError(t, model.DB.Where("username = ?", "newadminuser").First(&created).Error)
	require.Equal(t, "Admin Created", created.DisplayName)
	require.Equal(t, "admin-created@example.com", created.Email)
	require.Equal(t, int64(500000), created.Quota)
	require.Equal(t, "vip", created.Group)
}

// TestCreateUserMinimalFields verifies backward compatibility with minimal fields.
func TestCreateUserMinimalFields(t *testing.T) {
	setupSelfUpdateTest(t)

	router := gin.New()
	router.POST("/api/user/", func(c *gin.Context) {
		c.Set("role", model.RoleRootUser)
		CreateUser(c)
	})

	payload := map[string]string{
		"username": "minimaluser",
		"password": "testpass123",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])

	var created model.User
	require.NoError(t, model.DB.Where("username = ?", "minimaluser").First(&created).Error)
	require.Equal(t, "minimaluser", created.DisplayName) // defaults to username
}

// TestUpdateUserMcpToolBlacklistWithoutGroup verifies that mcp_tool_blacklist
// can be updated independently from group (the scoping bug fix).
func TestUpdateUserMcpToolBlacklistWithoutGroup(t *testing.T) {
	setupSelfUpdateTest(t)

	user := &model.User{
		Username:    "mcpuser",
		Password:    "hashed",
		DisplayName: "MCP User",
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		c.Set(ctxkey.Id, 1) // admin user ID
		UpdateUser(c)
	})

	// Send only id and mcp_tool_blacklist, NO group field
	payloadJSON := fmt.Sprintf(`{"id": %d, "mcp_tool_blacklist": ["tool1", "tool2"]}`, user.Id)
	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader([]byte(payloadJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])

	// Verify mcp_tool_blacklist was updated
	var updated model.User
	require.NoError(t, model.DB.First(&updated, user.Id).Error)
	require.NotNil(t, updated.MCPToolBlacklist)
	require.Contains(t, updated.MCPToolBlacklist, "tool1")
	require.Contains(t, updated.MCPToolBlacklist, "tool2")
	// Group should remain unchanged
	require.Equal(t, "default", updated.Group)
}
