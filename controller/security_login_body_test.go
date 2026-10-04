package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
)

// TestSecurityLoginBodyAdmission verifies fixed-length and chunked login input is bounded before tracking.
func TestSecurityLoginBodyAdmission(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	for _, turnstile := range []bool{false, true} {
		config.TurnstileCheckEnabled = turnstile
		for _, chunked := range []bool{false, true} {
			username := "security-large"
			body := `{"username":"` + username + `","password":"wrong","padding":"` + strings.Repeat("x", 32<<10) + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(body))
			if chunked {
				req.ContentLength = -1
			}
			w := httptest.NewRecorder()
			newLoginRouter().ServeHTTP(w, req)
			require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
			require.False(t, middleware.HasLoginFailure(username))
			require.Empty(t, w.Header().Values("Set-Cookie"))
		}
	}
	config.TurnstileCheckEnabled = false
	for _, body := range []string{
		`{"username":"` + strings.Repeat("u", 255) + `","password":"wrong"}`,
		`{"username":"security-trailing","password":"wrong"}{"extra":true}`,
	} {
		w := httptest.NewRecorder()
		newLoginRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(body)))
		require.Equal(t, http.StatusOK, w.Code, "preserve the dashboard error-envelope status")
		var rejected map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rejected))
		require.Equal(t, false, rejected["success"])
		require.Contains(t, rejected["message"], "invalid parameter")
		require.Empty(t, w.Header().Values("Set-Cookie"))
	}
	require.False(t, middleware.HasLoginFailure(strings.Repeat("u", 255)))
	require.False(t, middleware.HasLoginFailure("security-trailing"))
	config.PasswordLoginEnabled = true
	createLoginUser(t, "security-valid", "safe-fixture-password", model.RoleCommonUser)
	w := postLogin(t, newLoginRouter(), "security-valid", "safe-fixture-password")
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, true, result["success"])
}
