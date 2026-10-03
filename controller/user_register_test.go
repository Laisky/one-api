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

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// setupRegisterTest initializes an isolated database and registration config for user registration tests.
func setupRegisterTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "register.db")
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

	originalRegisterEnabled := config.RegisterEnabled
	originalPasswordRegisterEnabled := config.PasswordRegisterEnabled
	originalEmailVerificationEnabled := config.EmailVerificationEnabled
	originalTurnstile := config.TurnstileCheckEnabled
	t.Cleanup(func() {
		config.RegisterEnabled = originalRegisterEnabled
		config.PasswordRegisterEnabled = originalPasswordRegisterEnabled
		config.EmailVerificationEnabled = originalEmailVerificationEnabled
		config.TurnstileCheckEnabled = originalTurnstile
	})

	config.RegisterEnabled = true
	config.PasswordRegisterEnabled = true
	config.EmailVerificationEnabled = false
	config.TurnstileCheckEnabled = false
}

// newRegisterRouter returns a router with the registration endpoint wired directly for behavior tests.
func newRegisterRouter() *gin.Engine {
	router := gin.New()
	router.POST("/api/user/register", Register)
	return router
}

// TestRegisterDuplicateUsernameReturnsPublicFailure verifies duplicate registrations do not expose raw database errors.
func TestRegisterDuplicateUsernameReturnsPublicFailure(t *testing.T) {
	setupRegisterTest(t)
	existing := &model.User{
		Username:    "existinguser",
		Password:    "already-hashed",
		DisplayName: "existinguser",
		AccessToken: "existing-access-token",
		AffCode:     "EXST",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(existing).Error)

	payload := map[string]string{
		"username": "existinguser",
		"password": "password123",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newRegisterRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["success"])
	require.Equal(t, "Username already exists", resp["message"])
	require.NotContains(t, w.Body.String(), "duplicate key")
	require.NotContains(t, w.Body.String(), "unique constraint")

	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "existinguser").Count(&count).Error)
	require.Equal(t, int64(1), count)
}
