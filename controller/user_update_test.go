package controller

import (
	"bytes"
	"encoding/json"
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

func setupUserControllerTest(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "user-controller.db")
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

func TestUpdateUserQuotaToZero(t *testing.T) {
	setupUserControllerTest(t)

	user := &model.User{
		Username:    "quota-user",
		Password:    "hashed-password",
		DisplayName: "Original",
		Quota:       100,
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":    user.Id,
		"quota": 0,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, resp.Message)

	updated, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(0), updated.Quota)
	require.Equal(t, "Original", updated.DisplayName)
}

func TestUpdateUserClearEmail(t *testing.T) {
	setupUserControllerTest(t)

	user := &model.User{
		Username:    "email-user",
		Password:    "hashed-password",
		DisplayName: "Email User",
		Email:       "user@example.com",
		Quota:       50,
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":    user.Id,
		"email": "",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, resp.Message)

	updated, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, "", updated.Email)
	require.Equal(t, int64(50), updated.Quota)
}

func TestUpdateUserEmailNullSkipsChange(t *testing.T) {
	setupUserControllerTest(t)

	user := &model.User{
		Username:    "null-email-user",
		Password:    "hashed-password",
		DisplayName: "Null Email User",
		Email:       "existing@example.com",
		Quota:       42,
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":    user.Id,
		"email": nil,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, resp.Message)

	updated, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, "existing@example.com", updated.Email)
	require.Equal(t, int64(42), updated.Quota)
}

// createLockTargetUser creates a regular common user fixture for the password
// lock tests. The hashed password is preserved so individual cases can verify
// it has (or has not) been mutated by the controller.
func createLockTargetUser(t *testing.T, username string, locked bool) *model.User {
	t.Helper()
	hashed, err := common.Password2Hash("oldpassword")
	require.NoError(t, err)
	user := &model.User{
		Username:    username,
		Password:    hashed,
		DisplayName: "Lock Target",
		Email:       username + "@example.com",
		Role:        model.RoleCommonUser,
		Group:       "default",
		Status:      model.UserStatusEnabled,
		Metadata:    model.UserMetadata{PasswordLocked: locked},
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

// TestUpdateUserMetadataPasswordLockByRoot ensures a root admin can flip the
// metadata.password_locked flag and the change is persisted.
func TestUpdateUserMetadataPasswordLockByRoot(t *testing.T) {
	setupUserControllerTest(t)

	target := createLockTargetUser(t, "lock-by-root", false)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		c.Set(ctxkey.Id, 1)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":       target.Id,
		"metadata": map[string]any{"password_locked": true},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, resp.Message)

	updated, err := model.GetUserById(target.Id, true)
	require.NoError(t, err)
	require.True(t, updated.Metadata.PasswordLocked)
}

// TestUpdateUserMetadataPasswordLockByAdminRejected ensures only the root
// admin can change the password lock flag; a regular admin must be rejected
// and the database record must be untouched.
func TestUpdateUserMetadataPasswordLockByAdminRejected(t *testing.T) {
	setupUserControllerTest(t)

	target := createLockTargetUser(t, "lock-by-admin", false)

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleAdminUser)
		c.Set(ctxkey.Id, 1)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":       target.Id,
		"metadata": map[string]any{"password_locked": true},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "Only root admin can change password lock")

	updated, err := model.GetUserById(target.Id, true)
	require.NoError(t, err)
	require.False(t, updated.Metadata.PasswordLocked)
}

// TestUpdateUserPasswordRejectedWhenLocked ensures admins (including root)
// cannot change the password while metadata.password_locked is true.
func TestUpdateUserPasswordRejectedWhenLocked(t *testing.T) {
	setupUserControllerTest(t)

	target := createLockTargetUser(t, "locked-target", true)
	originalHash := target.Password

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		c.Set(ctxkey.Id, 1)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":       target.Id,
		"password": "newpass99",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "Password is locked for this user")

	var stored model.User
	require.NoError(t, model.DB.First(&stored, target.Id).Error)
	require.Equal(t, originalHash, stored.Password)
	require.True(t, stored.Metadata.PasswordLocked)
}

// TestUpdateUserRootCanUnlockAndChangePasswordInOneRequest ensures a root
// admin can clear the password lock and change the password atomically in
// the same PUT request.
func TestUpdateUserRootCanUnlockAndChangePasswordInOneRequest(t *testing.T) {
	setupUserControllerTest(t)

	target := createLockTargetUser(t, "unlock-and-change", true)
	originalHash := target.Password

	router := gin.New()
	router.PUT("/api/user/", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		c.Set(ctxkey.Id, 1)
		UpdateUser(c)
	})

	payload := map[string]any{
		"id":       target.Id,
		"metadata": map[string]any{"password_locked": false},
		"password": "newpass99",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/user/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success, resp.Message)

	var stored model.User
	require.NoError(t, model.DB.First(&stored, target.Id).Error)
	require.False(t, stored.Metadata.PasswordLocked)
	require.NotEqual(t, originalHash, stored.Password)
	require.True(t, common.ValidatePasswordAndHash("newpass99", stored.Password))
}
