package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// migratedActionPaths lists every former GET action that now requires POST.
var migratedActionPaths = []string{
	"/api/user/token", "/api/user/aff", "/api/user/totp/setup", "/api/user/logout",
	"/api/oauth/email/bind?email=attacker%40example.test&code=123456",
	"/api/oauth/wechat/bind?code=123456",
	"/api/oauth/wechat?code=123456",
	"/api/channel/test", "/api/channel/test/018f0000-0000-7000-8000-000000000972",
	"/api/channel/update_balance", "/api/channel/update_balance/018f0000-0000-7000-8000-000000000972",
}

// TestSecurityCSRFProxiedSameOriginMutation verifies that a browser-asserted
// same-origin dashboard mutation still works when a reverse proxy or a frontend
// dev proxy rewrites Host and no public ServerAddress is configured, while the
// same request with cross-site Fetch Metadata stays rejected.
func TestSecurityCSRFProxiedSameOriginMutation(t *testing.T) {
	for _, tc := range []struct {
		name, site, displayName string
		want                    int
	}{
		{"same_origin_behind_proxy", "same-origin", "proxied", http.StatusOK},
		{"cross_site_behind_proxy", "cross-site", "original", http.StatusForbidden},
		{"same_site_behind_proxy", "same-site", "original", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, engine, saved := csrfFixture(t)
			// The unconfigured placeholder is never a trust grant, and the proxy
			// forwards to a loopback Host that differs from the browser Origin.
			config.ServerAddress = "http://localhost:3000"
			request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:3000/api/user/self", strings.NewReader(`{"display_name":"proxied"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "https://dashboard.public.test")
			request.Header.Set("Sec-Fetch-Site", tc.site)
			request.AddCookie(saved)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			require.Equal(t, tc.want, w.Code)
			var user model.User
			require.NoError(t, db.First(&user, 971).Error)
			require.Equal(t, tc.displayName, user.DisplayName)
		})
	}
}

// TestSecurityCSRFWeChatLoginNavigation verifies a hostile navigation cannot
// replace a signed dashboard session with an attacker-owned WeChat account, and
// that cross-site POST is rejected before the code verifier is contacted.
// Trusted same-origin and anonymous logins still complete.
func TestSecurityCSRFWeChatLoginNavigation(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	require.NoError(t, db.Create(&model.User{Id: 973, UUID: "018f0000-0000-7000-8000-000000000973", Username: "csrf-attacker", Password: "unused-test-password", WeChatId: "attacker-wechat", AffCode: "atk1", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Group: "default"}).Error)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"success":true,"data":"attacker-wechat"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	oldEnabled, oldAddress := config.WeChatAuthEnabled, config.WeChatServerAddress
	config.WeChatAuthEnabled, config.WeChatServerAddress = true, upstream.URL
	t.Cleanup(func() { config.WeChatAuthEnabled, config.WeChatServerAddress = oldEnabled, oldAddress })
	const path = "/api/oauth/wechat?code=attacker-code"

	navigation := csrfRequest(engine, saved, http.MethodGet, path, "", "", "cross-site")
	require.Empty(t, navigation.Result().Cookies(), "navigation replaced the signed session")
	require.Zero(t, calls.Load(), "navigation contacted the WeChat code verifier")
	require.NotContains(t, navigation.Body.String(), `"success":true`)

	hostile := csrfRequest(engine, saved, http.MethodPost, path, "", "https://evil.test", "cross-site")
	require.Equal(t, http.StatusForbidden, hostile.Code)
	require.Empty(t, hostile.Result().Cookies())
	require.Zero(t, calls.Load())

	check := csrfRequest(engine, saved, http.MethodGet, "/api/user/self", "", "", "same-origin")
	require.Equal(t, http.StatusOK, check.Code)
	require.Contains(t, check.Body.String(), "csrf-fixture")

	for i, cookie := range []*http.Cookie{saved, nil} {
		request := httptest.NewRequest(http.MethodPost, "https://gateway.test"+path, nil)
		request.Header.Set("Origin", "https://gateway.test")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, request)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"success":true`)
		require.Contains(t, w.Body.String(), "csrf-attacker")
		require.NotEmpty(t, w.Result().Cookies())
		require.Equal(t, int32(i+1), calls.Load())
	}
}

// TestSecurityCSRFLegacyGetMethodNotAllowed verifies that every migrated action
// answers a legacy GET with 405 and an Allow header instead of falling through
// to an unrelated parameterized route, without touching account state.
func TestSecurityCSRFLegacyGetMethodNotAllowed(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	for _, path := range migratedActionPaths {
		for _, bearer := range []bool{false, true} {
			cookie := saved
			if bearer {
				cookie = nil
			}
			w := csrfRequest(engine, cookie, http.MethodGet, path, "", "", "same-origin")
			require.Equal(t, http.StatusMethodNotAllowed, w.Code, path)
			require.Equal(t, http.MethodPost, w.Header().Get("Allow"), path)
			require.Contains(t, w.Body.String(), `"success":false`, path)
			require.Empty(t, w.Result().Cookies(), path)
		}
	}
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "fixture-access-token", user.AccessToken)
	require.Empty(t, user.AffCode)
	require.Empty(t, user.Email)
	require.Empty(t, user.WeChatId)
}
