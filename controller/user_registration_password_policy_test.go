package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
)

// signupPolicyFixture describes a shared frontend/backend password-policy case.
type signupPolicyFixture struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Valid    bool   `json:"valid"`
}

// signupPolicyResponse holds the public response envelope inspected by these tests.
type signupPolicyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// serveSignupPolicyRequest runs the supplied JSON payload through a real handler
// using the existing isolated fixture and returns its HTTP response.
func serveSignupPolicyRequest(t *testing.T, f *behaviorDiffFixture, path string, handler gin.HandlerFunc, payload any, withSession bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return behaviorDiffServe(t, f, behaviorDiffRequest{
		method:   http.MethodPost,
		pattern:  path,
		target:   func(_ *behaviorDiffFixture) string { return path },
		body:     func(_ *behaviorDiffFixture) string { return string(body) },
		handler:  handler,
		sessions: withSession,
	})
}

// TestRegistrationPasswordPolicyAPI exercises the real registration handler
// against the shared fixtures, including callers that bypass the Modern UI.
func TestRegistrationPasswordPolicyAPI(t *testing.T) {
	f := behaviorDiffSetup(t)
	data, err := os.ReadFile("../web/modern/src/lib/testdata/registration-passwords.json")
	require.NoError(t, err)
	var cases []signupPolicyFixture
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)
	for i, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			username := fmt.Sprintf("signup-policy-%d", i)
			var beforeUsers, beforeTokens int64
			require.NoError(t, model.DB.Model(&model.User{}).Count(&beforeUsers).Error)
			require.NoError(t, model.DB.Model(&model.Token{}).Count(&beforeTokens).Error)
			recorder := serveSignupPolicyRequest(t, f, "/api/user/register", Register,
				map[string]string{"username": username, "password": tc.Password}, false)
			var response signupPolicyResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, tc.Valid, response.Success)
			if !tc.Valid {
				// Management errors keep HTTP 200; rejection is carried by success:false.
				require.Equal(t, http.StatusOK, recorder.Code)
				require.NotEmpty(t, response.Message)
				var afterUsers, afterTokens int64
				require.NoError(t, model.DB.Model(&model.User{}).Count(&afterUsers).Error)
				require.NoError(t, model.DB.Model(&model.Token{}).Count(&afterTokens).Error)
				require.Equal(t, beforeUsers, afterUsers)
				require.Equal(t, beforeTokens, afterTokens)
				return
			}
			require.Equal(t, http.StatusOK, recorder.Code)
			var stored model.User
			require.NoError(t, model.DB.Where("username = ?", username).First(&stored).Error)
			require.True(t, common.ValidatePasswordAndHash(tc.Password, stored.Password))
			if len(tc.Password) > 20 {
				require.False(t, common.ValidatePasswordAndHash(tc.Password[:20], stored.Password))
			}
			login := serveSignupPolicyRequest(t, f, "/api/user/login", Login,
				LoginRequest{Username: username, Password: tc.Password}, true)
			require.Equal(t, http.StatusOK, login.Code)
			response = signupPolicyResponse{}
			require.NoError(t, json.Unmarshal(login.Body.Bytes(), &response))
			require.True(t, response.Success)
			require.NotEmpty(t, login.Result().Cookies())
		})
	}
}

// TestRegistrationPasswordPolicyRecovery verifies invalid password feedback
// precedes email verification and allows correction using the original code.
func TestRegistrationPasswordPolicyRecovery(t *testing.T) {
	f := behaviorDiffSetup(t)
	config.EmailVerificationEnabled = true
	email, code := "signup-policy@example.test", "Ab1234"
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
	t.Cleanup(func() { common.DeleteKey(email, common.EmailVerificationPurpose) })
	payload := map[string]string{
		"username": "signup-recovery", "email": email,
		"verification_code": code, "password": "Abcdefgh12 ",
	}
	recorder := serveSignupPolicyRequest(t, f, "/api/user/register", Register, payload, false)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response signupPolicyResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Equal(t, dto.RegistrationPasswordError, response.Message)
	require.True(t, common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose))
	payload["password"] = "Abcdefgh12!"
	recorder = serveSignupPolicyRequest(t, f, "/api/user/register", Register, payload, false)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
}

// TestRegistrationPasswordPolicyPreservesLegacyLogin proves signup restrictions
// do not invalidate existing short, spaced or non-ASCII password hashes.
func TestRegistrationPasswordPolicyPreservesLegacyLogin(t *testing.T) {
	f := behaviorDiffSetup(t)
	for _, password := range []string{"Abc12345", " old pass12 ", "AncienMoté12"} {
		hash, err := common.Password2Hash(password)
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", f.user.Id).Update("password", hash).Error)
		recorder := serveSignupPolicyRequest(t, f, "/api/user/login", Login,
			LoginRequest{Username: f.user.Username, Password: password}, true)
		var response signupPolicyResponse
		require.Equal(t, http.StatusOK, recorder.Code)
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		var stored model.User
		require.NoError(t, model.DB.First(&stored, f.user.Id).Error)
		require.Equal(t, hash, stored.Password)
		if strings.TrimSpace(password) != password {
			require.False(t, common.ValidatePasswordAndHash(strings.TrimSpace(password), stored.Password))
		}
	}
}
