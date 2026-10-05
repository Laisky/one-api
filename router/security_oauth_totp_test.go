package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
)

// oauthTotpCase describes one OAuth login provider and how its callback is driven.
type oauthTotpCase struct {
	provider string
	user     model.User
	code     string
	identity string
}

// oauthLoginCallback drives the production login callback for tc in browser.
// GitHub, OIDC and Lark are GET callbacks validated by a fresh OAuth state;
// WeChat is the same-origin POST used by the dashboard login modal.
func oauthLoginCallback(t *testing.T, browser *oauthBrowser, provider, code string) *testHTTPResult {
	t.Helper()
	if provider == "wechat" {
		w := browser.do(http.MethodPost, "/api/oauth/wechat?code="+code, "", "same-origin", nil)
		return &testHTTPResult{status: w.Code, body: w.Body.String()}
	}
	state := browser.oauthState(t)
	w := browser.do(http.MethodGet, "/api/oauth/"+provider+"?code="+code+"&state="+state, "", "same-origin", nil)
	return &testHTTPResult{status: w.Code, body: w.Body.String()}
}

// testHTTPResult captures the status and body of one fixture request.
type testHTTPResult struct {
	status int
	body   string
}

// oauthTotpCases returns one TOTP-enabled account per OAuth login provider.
// Every call allocates fresh account ids, so repeated runs in one process never
// collide with codes already recorded by the TOTP replay cache.
func oauthTotpCases() []oauthTotpCase {
	wechat := oauthFixtureUser("totp-wechat-user")
	wechat.WeChatId = "totp-wechat"
	oidc := oauthFixtureUser("totp-oidc-user")
	oidc.OidcId = "totp-oidc"
	github := oauthFixtureUser("totp-github-user")
	github.GitHubId = "totp-github"
	lark := oauthFixtureUser("totp-lark-user")
	lark.LarkId = "totp-lark"
	return []oauthTotpCase{
		{provider: "wechat", user: wechat, code: "code-wechat", identity: "totp-wechat"},
		{provider: "oidc", user: oidc, code: "code-oidc", identity: "totp-oidc"},
		{provider: "github", user: github, code: "code-github", identity: "totp-github"},
		{provider: "lark", user: lark, code: "code-lark", identity: "totp-lark"},
	}
}

// TestSecurityOAuthLoginRequiresTotp proves every OAuth login provider enforces
// the account's TOTP second factor: the callback answers the password-login
// totp_required contract without an authenticated session, the pending login
// rejects hostile provenance, wrong codes and a replayed code, and only a
// valid code completes the login through POST /api/oauth/totp.
func TestSecurityOAuthLoginRequiresTotp(t *testing.T) {
	for _, tc := range oauthTotpCases() {
		t.Run(tc.provider, func(t *testing.T) {
			db, engine, _ := csrfFixture(t)
			require.NoError(t, db.Create(&tc.user).Error)
			installOAuthIdPStub(t, map[string]string{tc.code: tc.identity})
			browser := &oauthBrowser{engine: engine}

			callback := oauthLoginCallback(t, browser, tc.provider, tc.code)
			require.Equal(t, http.StatusOK, callback.status, callback.body)
			require.NotContains(t, callback.body, `"success":true`, "OAuth login bypassed TOTP")
			require.Contains(t, callback.body, `"totp_required":true`)
			require.NotContains(t, callback.body, tc.user.Username, "challenge leaked account data")
			require.Equal(t, http.StatusUnauthorized, browser.selfStatus(), "challenge created an authenticated session")
			pending := browser.cookie
			require.NotNil(t, pending, "challenge must persist the pending login marker")

			code := fixtureTotpCode(t)
			hostile := browser.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+code+`"}`, "cross-site", map[string]string{"Origin": "https://evil.test"})
			require.Equal(t, http.StatusForbidden, hostile.Code, hostile.Body.String())
			require.Equal(t, http.StatusUnauthorized, browser.selfStatus())

			wrong := browser.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+wrongTotpCode(code)+`"}`, "same-origin", nil)
			require.NotContains(t, wrong.Body.String(), `"success":true`)
			require.Contains(t, wrong.Body.String(), "Invalid TOTP code")
			require.Equal(t, http.StatusUnauthorized, browser.selfStatus())

			ok := browser.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+code+`"}`, "same-origin", nil)
			require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
			require.Contains(t, ok.Body.String(), `"success":true`)
			require.Contains(t, ok.Body.String(), tc.user.Username)
			self := browser.do(http.MethodGet, "/api/user/self", "", "same-origin", nil)
			require.Equal(t, http.StatusOK, self.Code)
			require.Contains(t, self.Body.String(), tc.user.Username)

			// A copy of the pending cookie cannot reuse the accepted code.
			replay := &oauthBrowser{engine: engine, cookie: pending}
			replayed := replay.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+code+`"}`, "same-origin", nil)
			require.NotContains(t, replayed.Body.String(), `"success":true`)
			require.Equal(t, http.StatusUnauthorized, replay.selfStatus())
		})
	}
}

// TestSecurityOAuthLoginWithoutTotpUnchanged is the positive control: accounts
// without TOTP still sign in directly from every OAuth provider.
func TestSecurityOAuthLoginWithoutTotpUnchanged(t *testing.T) {
	for _, tc := range oauthTotpCases() {
		t.Run(tc.provider, func(t *testing.T) {
			db, engine, _ := csrfFixture(t)
			tc.user.TotpSecret = ""
			require.NoError(t, db.Create(&tc.user).Error)
			installOAuthIdPStub(t, map[string]string{tc.code: tc.identity})
			browser := &oauthBrowser{engine: engine}

			callback := oauthLoginCallback(t, browser, tc.provider, tc.code)
			require.Equal(t, http.StatusOK, callback.status, callback.body)
			require.Contains(t, callback.body, `"success":true`)
			require.Contains(t, callback.body, tc.user.Username)
			require.Equal(t, http.StatusOK, browser.selfStatus())
		})
	}
}

// TestSecurityOAuthTotpWithoutPendingLogin verifies the completion endpoint
// never authenticates a browser that has no pending OAuth login, even with a
// valid code for a TOTP-enabled account, and that password login keeps its
// TOTP contract (positive control for the shared verifier).
func TestSecurityOAuthTotpWithoutPendingLogin(t *testing.T) {
	db, engine, _ := csrfFixture(t)
	user := oauthFixtureUser("totp-password-user")
	hash, err := common.Password2Hash("fixture-login-password")
	require.NoError(t, err)
	user.Password = hash
	require.NoError(t, db.Create(&user).Error)
	browser := &oauthBrowser{engine: engine}

	orphan := browser.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+fixtureTotpCode(t)+`"}`, "same-origin", nil)
	require.NotContains(t, orphan.Body.String(), `"success":true`)
	require.Equal(t, http.StatusUnauthorized, browser.selfStatus())

	login := browser.do(http.MethodPost, "/api/user/login", `{"username":"`+user.Username+`","password":"fixture-login-password"}`, "same-origin", nil)
	require.Contains(t, login.Body.String(), `"totp_required":true`)
	require.Equal(t, http.StatusUnauthorized, browser.selfStatus())
	login = browser.do(http.MethodPost, "/api/user/login", `{"username":"`+user.Username+`","password":"fixture-login-password","totp_code":"`+fixtureTotpCode(t)+`"}`, "same-origin", nil)
	require.Contains(t, login.Body.String(), `"success":true`)
	require.Equal(t, http.StatusOK, browser.selfStatus())
}
