package router

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// TestSecurityCSRFEmailBinding verifies even a valid attacker-owned email code
// cannot bind the victim account through navigation or a hostile POST, while
// the same-origin POST persists the requested binding.
func TestSecurityCSRFEmailBinding(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 971).Update("role", model.RoleCommonUser).Error)
	const email = "csrf-binding@example.test"
	common.RegisterVerificationCodeWithKey(email, "123456", common.EmailVerificationPurpose)
	t.Cleanup(func() { common.DeleteKey(email, common.EmailVerificationPurpose) })
	const path = "/api/oauth/email/bind?email=csrf-binding%40example.test&code=123456"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		csrfRequest(engine, saved, method, path, "", "https://evil.test", "cross-site")
		var user model.User
		require.NoError(t, db.First(&user, 971).Error)
		require.Empty(t, user.Email)
	}
	w := csrfRequest(engine, saved, http.MethodPost, path, "", "https://gateway.test", "same-origin")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, email, user.Email)
}

// TestSecurityCSRFWeChatBinding uses a local code verifier to prove rejected
// navigations and POST requests cannot dispatch or persist account bindings.
func TestSecurityCSRFWeChatBinding(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"success":true,"data":"fixture-wechat-id"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	oldEnabled, oldAddress := config.WeChatAuthEnabled, config.WeChatServerAddress
	config.WeChatAuthEnabled, config.WeChatServerAddress = true, upstream.URL
	t.Cleanup(func() { config.WeChatAuthEnabled, config.WeChatServerAddress = oldEnabled, oldAddress })
	const path = "/api/oauth/wechat/bind?code=123456"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		csrfRequest(engine, saved, method, path, "", "https://evil.test", "cross-site")
		var user model.User
		require.NoError(t, db.First(&user, 971).Error)
		require.Empty(t, user.WeChatId)
		require.Zero(t, calls.Load())
	}
	w := csrfRequest(engine, saved, http.MethodPost, path, "", "https://gateway.test", "same-origin")
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Equal(t, int32(1), calls.Load())
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "fixture-wechat-id", user.WeChatId)
}

// TestSecurityCSRFLogout verifies hostile POST requests cannot clear a signed
// session and a trusted POST still logs the caller out.
func TestSecurityCSRFLogout(t *testing.T) {
	_, engine, saved := csrfFixture(t)
	w := csrfRequest(engine, saved, http.MethodPost, "/api/user/logout", "", "https://evil.test", "cross-site")
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Empty(t, w.Result().Cookies())
	w = csrfRequest(engine, saved, http.MethodPost, "/api/user/logout", "", "https://gateway.test", "same-origin")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.NotEmpty(t, w.Result().Cookies())
	check := csrfRequest(engine, w.Result().Cookies()[0], http.MethodGet, "/api/user/self", "", "", "same-origin")
	require.Equal(t, http.StatusUnauthorized, check.Code)
}
