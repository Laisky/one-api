package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
)

// TestSecurityCSRFExistingSessionLogin verifies public login endpoints cannot
// replace a signed account session or its challenge using hostile provenance.
// Successful password login and passkey challenge creation remain available.
func TestSecurityCSRFExistingSessionLogin(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	hash, err := common.Password2Hash("fixture-login-password")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 971).Update("password", hash).Error)
	const body = `{"username":"csrf-fixture","password":"fixture-login-password"}`
	for _, path := range []string{"/api/user/login", "/api/user/passkey/login/begin", "/api/user/passkey/login/finish"} {
		w := csrfRequest(engine, saved, http.MethodPost, path, body, "https://evil.test", "cross-site")
		require.Equal(t, http.StatusForbidden, w.Code, path)
		require.Empty(t, w.Result().Cookies(), path)
	}
	for _, cookie := range []*http.Cookie{saved, nil} {
		// Anonymous login intentionally retains its existing contract; an absent
		// signed account cookie does not become a CSRF-protected dashboard action.
		w := csrfRequest(engine, cookie, http.MethodPost, "/api/user/login", body, "https://gateway.test", "same-origin")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"success":true`)
		require.NotEmpty(t, w.Result().Cookies())
		w = csrfRequest(engine, cookie, http.MethodPost, "/api/user/passkey/login/begin", "", "https://gateway.test", "same-origin")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"success":true`)
		require.NotEmpty(t, w.Result().Cookies())
		w = csrfRequest(engine, cookie, http.MethodPost, "/api/user/passkey/login/finish", "{}", "https://gateway.test", "same-origin")
		require.NotEqual(t, http.StatusForbidden, w.Code)
		require.Contains(t, w.Body.String(), "session")
	}
}
