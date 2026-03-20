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
	"github.com/songquanpeng/one-api/model"
)

func setupPasswordResetTest(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "password-reset.db")
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

func createTestUser(t *testing.T, email, password string) *model.User {
	t.Helper()
	hashedPassword, err := common.Password2Hash(password)
	require.NoError(t, err)
	user := &model.User{
		Username:    "testuser",
		Password:    hashedPassword,
		Email:       email,
		DisplayName: "Test User",
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func postResetPassword(t *testing.T, router *gin.Engine, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/reset", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// TestResetPasswordWithUserProvidedPassword verifies that when the frontend
// sends a password, that password is used instead of a random one.
func TestResetPasswordWithUserProvidedPassword(t *testing.T) {
	setupPasswordResetTest(t)

	email := "user@example.com"
	oldPassword := "oldpassword123"
	createTestUser(t, email, oldPassword)

	// Register a reset token
	token := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, token, common.PasswordResetPurpose)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	newPassword := "mynewpassword123"
	w := postResetPassword(t, router, map[string]string{
		"email":    email,
		"token":    token,
		"password": newPassword,
	})

	resp := parseResponse(t, w)
	require.Equal(t, true, resp["success"], "expected success, got: %v", resp["message"])

	// Verify the returned password matches what was sent
	require.Equal(t, newPassword, resp["data"])

	// Verify user can authenticate with the new password
	var user model.User
	require.NoError(t, model.DB.Where("email = ?", email).First(&user).Error)
	require.True(t, common.ValidatePasswordAndHash(newPassword, user.Password),
		"user should be able to authenticate with the user-provided password")
	require.False(t, common.ValidatePasswordAndHash(oldPassword, user.Password),
		"old password should no longer work")
}

// TestResetPasswordWithoutPassword verifies backward compatibility: when no
// password is provided (legacy frontends), a random password is generated.
func TestResetPasswordWithoutPassword(t *testing.T) {
	setupPasswordResetTest(t)

	email := "legacy@example.com"
	createTestUser(t, email, "oldpassword")

	token := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, token, common.PasswordResetPurpose)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	// Legacy frontend sends only email and token, no password
	w := postResetPassword(t, router, map[string]string{
		"email": email,
		"token": token,
	})

	resp := parseResponse(t, w)
	require.Equal(t, true, resp["success"])

	// A random password should be returned
	generatedPassword, ok := resp["data"].(string)
	require.True(t, ok)
	require.Len(t, generatedPassword, 12, "generated password should be 12 chars")

	// Verify user can authenticate with the generated password
	var user model.User
	require.NoError(t, model.DB.Where("email = ?", email).First(&user).Error)
	require.True(t, common.ValidatePasswordAndHash(generatedPassword, user.Password))
}

// TestResetPasswordInvalidToken verifies that an invalid or expired token is rejected.
func TestResetPasswordInvalidToken(t *testing.T) {
	setupPasswordResetTest(t)

	email := "user@example.com"
	createTestUser(t, email, "password")

	// Register a valid token but use a different one
	validToken := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, validToken, common.PasswordResetPurpose)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	w := postResetPassword(t, router, map[string]string{
		"email":    email,
		"token":    "wrong-token",
		"password": "newpassword",
	})

	resp := parseResponse(t, w)
	require.Equal(t, false, resp["success"])
	require.Contains(t, resp["message"], "illegal or expired")
}

// TestResetPasswordMissingFields verifies that missing email or token returns an error.
func TestResetPasswordMissingFields(t *testing.T) {
	setupPasswordResetTest(t)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	cases := []struct {
		name    string
		payload map[string]string
	}{
		{"missing email", map[string]string{"token": "abc"}},
		{"missing token", map[string]string{"email": "a@b.com"}},
		{"both empty", map[string]string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postResetPassword(t, router, tc.payload)
			resp := parseResponse(t, w)
			require.Equal(t, false, resp["success"])
		})
	}
}

// TestResetPasswordTokenDeletedAfterUse verifies the token is consumed and
// cannot be reused.
func TestResetPasswordTokenDeletedAfterUse(t *testing.T) {
	setupPasswordResetTest(t)

	email := "reuse@example.com"
	createTestUser(t, email, "oldpw")

	token := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, token, common.PasswordResetPurpose)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	// First reset should succeed
	w := postResetPassword(t, router, map[string]string{
		"email":    email,
		"token":    token,
		"password": "firstnewpw12",
	})
	resp := parseResponse(t, w)
	require.Equal(t, true, resp["success"])

	// Second reset with same token should fail
	w = postResetPassword(t, router, map[string]string{
		"email":    email,
		"token":    token,
		"password": "secondnewpw",
	})
	resp = parseResponse(t, w)
	require.Equal(t, false, resp["success"])
	require.Contains(t, resp["message"], "illegal or expired")
}

// TestResetPasswordInvalidJSON verifies that malformed JSON body is handled.
func TestResetPasswordInvalidJSON(t *testing.T) {
	setupPasswordResetTest(t)

	router := gin.New()
	router.POST("/api/user/reset", ResetPassword)

	req := httptest.NewRequest(http.MethodPost, "/api/user/reset",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := parseResponse(t, w)
	require.Equal(t, false, resp["success"])
}
