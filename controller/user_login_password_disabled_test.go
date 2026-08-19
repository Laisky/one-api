package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

// setupPasswordLoginDisabledTest spins up a sqlite-backed test environment
// suitable for exercising controller.Login end-to-end (session middleware
// included so the success path can save its session).
func setupPasswordLoginDisabledTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "password-login-disabled.db")
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

	// Snapshot and restore feature toggles so tests don't bleed into each other.
	originalPasswordLogin := config.PasswordLoginEnabled
	originalTurnstile := config.TurnstileCheckEnabled
	t.Cleanup(func() {
		config.PasswordLoginEnabled = originalPasswordLogin
		config.TurnstileCheckEnabled = originalTurnstile
	})
	config.TurnstileCheckEnabled = false
}

func newLoginRouter() *gin.Engine {
	router := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("test-session", store))
	router.POST("/api/user/login", Login)
	return router
}

func createLoginUser(t *testing.T, username, password string, role int) *model.User {
	t.Helper()
	hashed, err := common.Password2Hash(password)
	require.NoError(t, err)
	u := &model.User{
		Username:    username,
		Password:    hashed,
		Email:       username + "@example.com",
		DisplayName: username,
		Role:        role,
		Status:      model.UserStatusEnabled,
		Group:       "default",
		AccessToken: "tok-" + username,
		AffCode:     "AFF-" + username,
	}
	require.NoError(t, model.DB.Create(u).Error)
	return u
}

func postLogin(t *testing.T, router *gin.Engine, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestLoginPasswordDisabled_BlocksRegularUser verifies that when an admin
// disables PasswordLoginEnabled, a regular user with otherwise valid
// credentials is rejected with the dedicated message instead of being logged
// in. This is the regression guard for the original bug report.
func TestLoginPasswordDisabled_BlocksRegularUser(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "regularuser", "correctpw1", model.RoleCommonUser)
	defer middleware.ClearLoginFailure("regularuser")

	config.PasswordLoginEnabled = false

	w := postLogin(t, newLoginRouter(), "regularuser", "correctpw1")

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["success"])
	require.Contains(t, resp["message"], "disabled password login")
}

// TestLoginPasswordDisabled_BlocksAdminUser confirms the rule applies to
// non-root admins too — only the root account may bypass the toggle.
func TestLoginPasswordDisabled_BlocksAdminUser(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "adminuser", "adminpw123", model.RoleAdminUser)
	defer middleware.ClearLoginFailure("adminuser")

	config.PasswordLoginEnabled = false

	w := postLogin(t, newLoginRouter(), "adminuser", "adminpw123")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["success"])
	require.Contains(t, resp["message"], "disabled password login")
}

// TestLoginPasswordDisabled_AllowsRootUser confirms the recovery path: the
// root user can still log in via password even when the toggle is off, so a
// site operator can recover access if SSO breaks.
func TestLoginPasswordDisabled_AllowsRootUser(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "rootuser", "rootpw1234", model.RoleRootUser)
	defer middleware.ClearLoginFailure("rootuser")

	config.PasswordLoginEnabled = false

	w := postLogin(t, newLoginRouter(), "rootuser", "rootpw1234")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "root login must succeed; got: %v", resp["message"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected user payload, got %v", resp["data"])
	require.Equal(t, "rootuser", data["username"])
	require.EqualValues(t, model.RoleRootUser, data["role"])
}

// TestLoginPasswordEnabled_AllowsRegularUser is the backward-compatibility
// guard: with the default toggle, a regular user keeps logging in normally.
func TestLoginPasswordEnabled_AllowsRegularUser(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "happyuser", "happypw123", model.RoleCommonUser)
	defer middleware.ClearLoginFailure("happyuser")

	config.PasswordLoginEnabled = true

	w := postLogin(t, newLoginRouter(), "happyuser", "happypw123")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])
}

// TestLoginPasswordDisabled_WrongPasswordStillGenericError verifies that the
// new check runs *after* credential validation, so a caller with a wrong
// password still sees the generic "wrong password" message — we don't leak
// the toggle state (or account existence) to unauthenticated probers.
func TestLoginPasswordDisabled_WrongPasswordStillGenericError(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "victim", "realpassword1", model.RoleCommonUser)
	defer middleware.ClearLoginFailure("victim")

	config.PasswordLoginEnabled = false

	w := postLogin(t, newLoginRouter(), "victim", "wrongpassword")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["success"])
	require.NotContains(t, resp["message"], "disabled password login",
		"must not reveal toggle state to callers without valid credentials")
}
